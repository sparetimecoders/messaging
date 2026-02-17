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

package amqp

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/sparetimecoders/gomessaging/spec"
)

type (
	// TypeMapper resolves a routing key to a concrete Go type (as reflect.Type).
	// Return false if no type is registered for the key.
	TypeMapper func(routingKey string) (reflect.Type, bool)
)

// TypeMappingHandler wraps an EventHandler[any] and uses the TypeMapper to unmarshal each
// message payload into the correct concrete type before passing it to the handler.
// This is useful when a single consumer handles multiple event types on a wildcard routing key.
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

// EventStreamConsumer sets up a durable, persistent event stream consumer on the default
// "events" topic exchange. For a transient queue, use TransientEventStreamConsumer instead.
func EventStreamConsumer[T any](routingKey string, handler spec.EventHandler[T], opts ...ConsumerOptions) Setup {
	return StreamConsumer(defaultEventExchangeName, routingKey, handler, opts...)
}

// ServiceResponseConsumer sets up a durable, persistent consumer for responses from targetService.
// It binds to the target service's response exchange using header-based routing.
func ServiceResponseConsumer[T any](targetService, routingKey string, handler spec.EventHandler[T], opts ...ConsumerOptions) Setup {
	return func(c *Connection) error {
		opts = append(opts, func(config *consumerConfig) error {
			config.queueBindingHeaders[headerService] = c.serviceName
			return nil
		})

		config, err := newConsumerConfig(routingKey,
			serviceResponseExchangeName(targetService),
			serviceResponseQueueName(targetService, c.serviceName),
			amqp.ExchangeHeaders,
			newWrappedHandler(handler),
			opts...)
		if err != nil {
			return err
		}

		if err := c.messageHandlerBindQueueToExchange(config); err != nil {
			return err
		}
		c.addEndpoint(spec.Endpoint{
			Direction:    spec.DirectionConsume,
			Pattern:      spec.PatternServiceResponse,
			ExchangeName: config.exchangeName,
			ExchangeKind: spec.ExchangeHeaders,
			QueueName:    config.queueName,
			RoutingKey:   routingKey,
			MessageType:  reflect.TypeFor[T]().String(),
		})
		return nil
	}
}

// ServiceRequestConsumer sets up a durable, persistent consumer for incoming service requests.
// It binds to the connection's own service request exchange using direct routing.
func ServiceRequestConsumer[T any](routingKey string, handler spec.EventHandler[T], opts ...ConsumerOptions) Setup {
	return func(c *Connection) error {
		resExchangeName := serviceResponseExchangeName(c.serviceName)
		if err := exchangeDeclare(c.setupChannel, resExchangeName, amqp.ExchangeHeaders); err != nil {
			return fmt.Errorf("failed to create exchange %s, %w", resExchangeName, err)
		}

		config, err := newConsumerConfig(routingKey,
			serviceRequestExchangeName(c.serviceName),
			serviceRequestQueueName(c.serviceName),
			amqp.ExchangeDirect,
			newWrappedHandler(handler),
			opts...)
		if err != nil {
			return err
		}
		if err := c.messageHandlerBindQueueToExchange(config); err != nil {
			return err
		}
		c.addEndpoint(spec.Endpoint{
			Direction:    spec.DirectionConsume,
			Pattern:      spec.PatternServiceRequest,
			ExchangeName: config.exchangeName,
			ExchangeKind: spec.ExchangeDirect,
			QueueName:    config.queueName,
			RoutingKey:   routingKey,
			MessageType:  reflect.TypeFor[T]().String(),
		})
		return nil
	}
}

// StreamConsumer sets up a durable, persistent event stream consumer on a named topic exchange.
func StreamConsumer[T any](exchange, routingKey string, handler spec.EventHandler[T], opts ...ConsumerOptions) Setup {
	exchangeName := topicExchangeName(exchange)
	return func(c *Connection) error {
		config, err := newConsumerConfig(routingKey,
			exchangeName,
			serviceEventQueueName(exchangeName, c.serviceName),
			amqp.ExchangeTopic,
			newWrappedHandler(handler),
			opts...)
		if err != nil {
			return err
		}

		if err := c.messageHandlerBindQueueToExchange(config); err != nil {
			return err
		}
		pattern := spec.PatternEventStream
		if exchange != defaultEventExchangeName {
			pattern = spec.PatternCustomStream
		}
		c.addEndpoint(spec.Endpoint{
			Direction:    spec.DirectionConsume,
			Pattern:      pattern,
			ExchangeName: config.exchangeName,
			ExchangeKind: spec.ExchangeTopic,
			QueueName:    config.queueName,
			RoutingKey:   routingKey,
			MessageType:  reflect.TypeFor[T]().String(),
		})
		return nil
	}
}

// TransientEventStreamConsumer sets up an event stream consumer that will clean up resources when the
// connection is closed.
// For a durable queue, use the EventStreamConsumer function instead.
func TransientEventStreamConsumer[T any](routingKey string, handler spec.EventHandler[T], opts ...ConsumerOptions) Setup {
	return TransientStreamConsumer(defaultEventExchangeName, routingKey, handler, opts...)
}

// TransientStreamConsumer sets up an event stream consumer that will clean up resources when the
// connection is closed.
// For a durable queue, use the StreamConsumer function instead.
func TransientStreamConsumer[T any](exchange, routingKey string, handler spec.EventHandler[T], opts ...ConsumerOptions) Setup {
	exchangeName := topicExchangeName(exchange)
	return func(c *Connection) error {
		queueName := serviceEventRandomQueueName(exchangeName, c.serviceName)
		opts = append(opts, func(config *consumerConfig) error {
			config.queueHeaders[amqp.QueueTTLArg] = 1000
			return nil
		})
		config, err := newConsumerConfig(routingKey,
			exchangeName,
			queueName,
			amqp.ExchangeTopic,
			newWrappedHandler(handler),
			opts...)
		if err != nil {
			return err
		}

		if err := c.messageHandlerBindQueueToExchange(config); err != nil {
			return err
		}
		pattern := spec.PatternEventStream
		if exchange != defaultEventExchangeName {
			pattern = spec.PatternCustomStream
		}
		c.addEndpoint(spec.Endpoint{
			Direction:    spec.DirectionConsume,
			Pattern:      pattern,
			ExchangeName: config.exchangeName,
			ExchangeKind: spec.ExchangeTopic,
			QueueName:    config.queueName,
			RoutingKey:   routingKey,
			MessageType:  reflect.TypeFor[T]().String(),
			Ephemeral:    true,
		})
		return nil
	}
}
