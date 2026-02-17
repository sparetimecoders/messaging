// MIT License
//
// Copyright (c) 2026 sparetimecoders
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	natsgo "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/sparetimecoders/gomessaging/spec"
)

// TypeMapper resolves a routing key to a concrete Go type.
type TypeMapper func(routingKey string) (reflect.Type, bool)

// TypeMappingHandler wraps an EventHandler[any] and uses the TypeMapper to unmarshal
// each message payload into the correct concrete type before passing it to the handler.
func TypeMappingHandler(handler spec.EventHandler[any], routingKeyToType TypeMapper) spec.EventHandler[any] {
	return func(ctx context.Context, event spec.ConsumableEvent[any]) error {
		if routingKeyToType == nil {
			return fmt.Errorf("TypeMapper is nil")
		}

		typ, exists := routingKeyToType(event.DeliveryInfo.Key)
		if !exists {
			return ErrNoMessageTypeForRouteKey
		}
		payload := reflect.New(typ.Elem()).Interface()
		if err := json.Unmarshal(event.Payload.(json.RawMessage), &payload); err != nil {
			return fmt.Errorf("%v: %w", err, spec.ErrParseJSON)
		}
		event.Payload = payload
		return handler(ctx, event)
	}
}

// EventStreamConsumer sets up a durable consumer on the default "events" stream.
func EventStreamConsumer[T any](routingKey string, handler spec.EventHandler[T], opts ...ConsumerOptions) Setup {
	return StreamConsumer[T](defaultEventStreamName, routingKey, handler, opts...)
}

// StreamConsumer sets up a durable consumer on a named JetStream stream.
func StreamConsumer[T any](stream, routingKey string, handler spec.EventHandler[T], opts ...ConsumerOptions) Setup {
	name := streamName(stream)
	return func(c *Connection) error {
		cfg, err := newConsumerConfig(name, routingKey, false, newWrappedHandler(handler), c.serviceName, opts...)
		if err != nil {
			return err
		}

		if !c.collectMode {
			if err := c.startJSConsumer(context.Background(), cfg); err != nil {
				return err
			}
		}

		pattern := spec.PatternEventStream
		if stream != defaultEventStreamName {
			pattern = spec.PatternCustomStream
		}
		c.addEndpoint(spec.Endpoint{
			Direction:    spec.DirectionConsume,
			Pattern:      pattern,
			ExchangeName: name,
			ExchangeKind: spec.ExchangeTopic,
			QueueName:    cfg.consumerName,
			RoutingKey:   routingKey,
			MessageType:  reflect.TypeFor[T]().String(),
		})
		return nil
	}
}

// TransientEventStreamConsumer sets up an ephemeral consumer on the default "events" stream.
func TransientEventStreamConsumer[T any](routingKey string, handler spec.EventHandler[T], opts ...ConsumerOptions) Setup {
	return TransientStreamConsumer[T](defaultEventStreamName, routingKey, handler, opts...)
}

// TransientStreamConsumer sets up an ephemeral consumer on a named JetStream stream.
func TransientStreamConsumer[T any](stream, routingKey string, handler spec.EventHandler[T], opts ...ConsumerOptions) Setup {
	name := streamName(stream)
	return func(c *Connection) error {
		cfg, err := newConsumerConfig(name, routingKey, true, newWrappedHandler(handler), c.serviceName, opts...)
		if err != nil {
			return err
		}

		if !c.collectMode {
			if err := c.startJSConsumer(context.Background(), cfg); err != nil {
				return err
			}
		}

		pattern := spec.PatternEventStream
		if stream != defaultEventStreamName {
			pattern = spec.PatternCustomStream
		}
		c.addEndpoint(spec.Endpoint{
			Direction:    spec.DirectionConsume,
			Pattern:      pattern,
			ExchangeName: name,
			ExchangeKind: spec.ExchangeTopic,
			QueueName:    cfg.consumerName,
			RoutingKey:   routingKey,
			MessageType:  reflect.TypeFor[T]().String(),
			Ephemeral:    true,
		})
		return nil
	}
}

// ServiceRequestConsumer sets up a consumer for incoming service requests
// using NATS Core subscriptions.
func ServiceRequestConsumer[T any](routingKey string, handler spec.EventHandler[T], opts ...ConsumerOptions) Setup {
	return func(c *Connection) error {
		if !c.collectMode {
			subject := serviceRequestSubject(c.serviceName, routingKey)
			if err := c.startCoreConsumer(subject, routingKey, c.serviceName, newWrappedHandler(handler)); err != nil {
				return err
			}
		}
		c.addEndpoint(spec.Endpoint{
			Direction:    spec.DirectionConsume,
			Pattern:      spec.PatternServiceRequest,
			ExchangeName: c.serviceName,
			ExchangeKind: spec.ExchangeDirect,
			QueueName:    c.serviceName,
			RoutingKey:   routingKey,
			MessageType:  reflect.TypeFor[T]().String(),
		})
		return nil
	}
}

// ServiceResponseConsumer sets up a consumer for responses from targetService.
// In the NATS implementation, this is handled automatically by the request-reply
// mechanism. This setup exists for topology registration.
func ServiceResponseConsumer[T any](targetService, routingKey string, handler spec.EventHandler[T], opts ...ConsumerOptions) Setup {
	return func(c *Connection) error {
		c.addEndpoint(spec.Endpoint{
			Direction:    spec.DirectionConsume,
			Pattern:      spec.PatternServiceResponse,
			ExchangeName: targetService,
			ExchangeKind: spec.ExchangeHeaders,
			QueueName:    c.serviceName,
			RoutingKey:   routingKey,
			MessageType:  reflect.TypeFor[T]().String(),
		})
		return nil
	}
}

// startJSConsumer collects a JetStream consumer config for deferred startup.
// Actual consumer creation happens in startPendingJSConsumers after all setup
// functions have run, so that registrations sharing the same stream+durable
// are grouped into a single NATS consumer with multiple filter subjects.
func (c *Connection) startJSConsumer(_ context.Context, cfg *consumerConfig) error {
	c.pendingJSConsumers = append(c.pendingJSConsumers, cfg)
	return nil
}

// consumerGroupKey returns a key for grouping JetStream consumer configs.
func consumerGroupKey(cfg *consumerConfig) string {
	return cfg.stream + ":" + cfg.consumerName
}

// startPendingJSConsumers groups collected configs by stream+durable and creates
// one NATS consumer per group with all filter subjects. This matches the AMQP
// pattern of one queue with multiple routing key bindings.
func (c *Connection) startPendingJSConsumers(ctx context.Context) error {
	// Group configs by stream+durable.
	type group struct {
		configs []*consumerConfig
	}
	groups := make(map[string]*group)
	var order []string
	for _, cfg := range c.pendingJSConsumers {
		key := consumerGroupKey(cfg)
		g, exists := groups[key]
		if !exists {
			g = &group{}
			groups[key] = g
			order = append(order, key)
		}
		g.configs = append(g.configs, cfg)
	}

	for _, key := range order {
		g := groups[key]
		first := g.configs[0]

		stream, err := c.ensureStream(ctx, first.stream)
		if err != nil {
			return fmt.Errorf("failed to ensure stream %s: %w", first.stream, err)
		}

		// Collect filter subjects and handlers from all configs in the group.
		var filterSubjects []string
		consumer := &jsConsumer{
			name:           first.consumerName,
			stream:         first.stream,
			serviceName:    c.serviceName,
			handlers:       make(routingKeyHandler),
			notificationCh: c.notificationCh,
			errorCh:        c.errorCh,
			spanNameFn:     c.spanNameFn,
			logger:         c.log().With("consumer", first.consumerName, "stream", first.stream),
			tracer:         c.tracer(),
			propagator:     c.propagator,
		}

		var routingKeys []string
		for _, cfg := range g.configs {
			filterSubjects = append(filterSubjects, filterSubject(cfg.stream, cfg.routingKey))
			consumer.handlers.add(cfg.routingKey, cfg.handler)
			routingKeys = append(routingKeys, cfg.routingKey)
		}

		jsCfg := jetstream.ConsumerConfig{}
		if !first.ephemeral {
			jsCfg.Durable = first.consumerName
		}
		if len(filterSubjects) == 1 {
			jsCfg.FilterSubject = filterSubjects[0]
		} else {
			jsCfg.FilterSubjects = filterSubjects
		}

		cons, err := stream.CreateOrUpdateConsumer(ctx, jsCfg)
		if err != nil {
			return fmt.Errorf("failed to create consumer on stream %s: %w", first.stream, err)
		}

		consCtx, err := cons.Consume(consumer.handleMessage)
		if err != nil {
			return fmt.Errorf("failed to start consuming on stream %s: %w", first.stream, err)
		}

		c.consumers = append(c.consumers, &jsConsumerHandle{ctx: consCtx})
		c.log().Info("started JetStream consumer",
			"consumer", first.consumerName,
			"stream", first.stream,
			"routingKeys", routingKeys,
		)
	}

	c.pendingJSConsumers = nil
	return nil
}

// startCoreConsumer subscribes to a NATS Core subject.
func (c *Connection) startCoreConsumer(subject, routingKey, consName string, handler wrappedHandler) error {
	consumer := &jsConsumer{
		name:           consName,
		stream:         "",
		serviceName:    c.serviceName,
		handlers:       make(routingKeyHandler),
		notificationCh: c.notificationCh,
		errorCh:        c.errorCh,
		spanNameFn:     c.spanNameFn,
		logger:         c.log().With("subject", subject),
		tracer:         c.tracer(),
		propagator:     c.propagator,
	}
	consumer.handlers.add(routingKey, handler)

	sub, err := c.nc.Subscribe(subject, func(msg *natsgo.Msg) {
		consumer.handleCoreMessage(msg, routingKey)
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe to %s: %w", subject, err)
	}

	c.subscriptions = append(c.subscriptions, sub)
	c.log().Info("started Core subscription", "subject", subject)
	return nil
}
