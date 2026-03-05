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
	"strings"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/sparetimecoders/messaging/specification/spec"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// Publisher is used to send messages
type Publisher struct {
	channel        amqpChannel
	exchange       string
	serviceName    string
	defaultHeaders []Header
	tracer         trace.Tracer
	propagator     propagation.TextMapPropagator
	spanNameFn     func(exchange, routingKey string) string
	confirmCh      chan amqp.Confirmation // nil = no confirms
	noConfirm      bool                  // true = skip confirm setup
}

// PublisherOption is a functional option for NewPublisher.
type PublisherOption func(*Publisher)

// WithConfirm enables publisher confirms on this publisher's own AMQP channel.
// Confirmations are delivered to the provided channel.
func WithConfirm(ch chan amqp.Confirmation) PublisherOption {
	return func(p *Publisher) {
		p.confirmCh = ch
	}
}

// WithoutPublisherConfirms disables publisher confirms for this publisher.
// By default, Publish() waits for broker confirmation and returns error on nack.
// Use this opt-out for high-throughput scenarios where occasional message loss
// is acceptable.
func WithoutPublisherConfirms() PublisherOption {
	return func(p *Publisher) {
		p.noConfirm = true
	}
}

var (
	// ErrNoMessageTypeForRouteKey is returned when a TypeMapper has no type registered for the routing key.
	ErrNoMessageTypeForRouteKey = fmt.Errorf("no message type for routing key configured")
)

// NewPublisher creates an uninitialized Publisher. Pass it to a publisher Setup function
// (e.g. EventStreamPublisher, ServicePublisher) which wires it to the connection's channel
// and exchange. After Start completes, call Publish to send messages.
func NewPublisher(opts ...PublisherOption) *Publisher {
	p := &Publisher{}
	for _, o := range opts {
		o(p)
	}
	return p
}

// Publish sends msg as a JSON-encoded AMQP message with the given routing key.
// Additional headers can be provided to attach metadata to the message.
// By default, Publish waits for broker confirmation and returns an error if the
// broker nacks. Use WithoutPublisherConfirms to disable this behavior.
func (p *Publisher) Publish(ctx context.Context, routingKey string, msg any, headers ...Header) error {
	table := amqp.Table{}
	for _, v := range p.defaultHeaders {
		table[v.Key] = v.Value
	}
	for _, h := range headers {
		if err := h.validateKey(); err != nil {
			return err
		}
		table[h.Key] = h.Value
	}

	err := publishMessage(ctx, p.tracer, p.propagator, p.spanNameFn, p.channel, msg, routingKey, p.exchange, p.serviceName, table)
	if err != nil {
		return err
	}
	if p.confirmCh != nil {
		select {
		case confirm := <-p.confirmCh:
			if !confirm.Ack {
				return fmt.Errorf("broker nacked publish to %s/%s", p.exchange, routingKey)
			}
		case <-ctx.Done():
			return fmt.Errorf("context cancelled waiting for publish confirm: %w", ctx.Err())
		}
	}
	return nil
}

// EventStreamPublisher sets up an event stream publisher
func EventStreamPublisher(publisher *Publisher) Setup {
	return StreamPublisher(defaultEventExchangeName, publisher)
}

// StreamPublisher sets up an event stream publisher
func StreamPublisher(exchange string, publisher *Publisher) Setup {
	exchangeName := topicExchangeName(exchange)
	return func(c *Connection) error {
		if err := exchangeDeclare(c.setupChannel, exchangeName, amqp.ExchangeTopic); err != nil {
			return fmt.Errorf("failed to declare exchange %s, %w", exchangeName, err)
		}
		ch, err := c.connection.channel()
		if err != nil {
			return fmt.Errorf("failed to create publisher channel: %w", err)
		}
		c.logger.Info("configured publisher", "exchange", exchangeName, "kind", "topic")
		pattern := spec.PatternEventStream
		if exchange != defaultEventExchangeName {
			pattern = spec.PatternCustomStream
		}
		c.addEndpoint(spec.Endpoint{
			Direction:    spec.DirectionPublish,
			Pattern:      pattern,
			ExchangeName: exchangeName,
			ExchangeKind: spec.ExchangeTopic,
		})
		return publisher.setup(ch, c.serviceName, exchangeName, c.tracer(), c.propagator, c.publishSpanNameFn)
	}
}

// QueuePublisher sets up a publisher that will send events to a specific queue instead of using the exchange,
// so called Sender-Selected distribution
// https://www.rabbitmq.com/sender-selected.html#:~:text=The%20RabbitMQ%20broker%20treats%20the,key%20if%20they%20are%20present.
func QueuePublisher(publisher *Publisher, destinationQueueName string) Setup {
	return func(c *Connection) error {
		ch, err := c.connection.channel()
		if err != nil {
			return fmt.Errorf("failed to create publisher channel: %w", err)
		}
		c.addEndpoint(spec.Endpoint{
			Direction:    spec.DirectionPublish,
			Pattern:      spec.PatternQueuePublish,
			QueueName:    destinationQueueName,
			ExchangeName: "(default)",
		})
		return publisher.setup(ch, c.serviceName, "", c.tracer(), c.propagator, c.publishSpanNameFn, Header{Key: "CC", Value: []any{destinationQueueName}})
	}
}

// ServicePublisher sets up a publisher that sends messages to the targetService's request exchange.
func ServicePublisher(targetService string, publisher *Publisher) Setup {
	exchangeName := serviceRequestExchangeName(targetService)
	return func(c *Connection) error {
		if err := exchangeDeclare(c.setupChannel, exchangeName, amqp.ExchangeDirect); err != nil {
			return err
		}
		ch, err := c.connection.channel()
		if err != nil {
			return fmt.Errorf("failed to create publisher channel: %w", err)
		}
		c.logger.Info("configured service publisher", "exchange", exchangeName, "targetService", targetService)
		c.addEndpoint(spec.Endpoint{
			Direction:    spec.DirectionPublish,
			Pattern:      spec.PatternServiceRequest,
			ExchangeName: exchangeName,
			ExchangeKind: spec.ExchangeDirect,
		})
		return publisher.setup(ch, c.serviceName, exchangeName, c.tracer(), c.propagator, c.publishSpanNameFn)
	}
}

func (p *Publisher) setup(channel amqpChannel, serviceName, exchange string, tracer trace.Tracer, prop propagation.TextMapPropagator, spanNameFn func(string, string) string, headers ...Header) error {
	for _, h := range headers {
		if err := h.validateKey(); err != nil {
			return err
		}
	}
	p.defaultHeaders = append(headers, Header{Key: headerService, Value: serviceName})
	p.channel = channel
	p.exchange = exchange
	p.serviceName = serviceName
	p.tracer = tracer
	p.propagator = prop
	p.spanNameFn = spanNameFn
	if !p.noConfirm {
		if p.confirmCh == nil {
			p.confirmCh = make(chan amqp.Confirmation, 1)
		}
		channel.NotifyPublish(p.confirmCh)
		if err := channel.Confirm(false); err != nil {
			return fmt.Errorf("failed to enable confirm mode: %w", err)
		}
	}
	return nil
}

func publishMessage(ctx context.Context, tracer trace.Tracer, prop propagation.TextMapPropagator, spanNameFn func(string, string) string, channel amqpChannel, msg any, routingKey, exchangeName, source string, headers amqp.Table) error {
	if tracer == nil {
		tracer = tracerFromProvider(nil)
	}
	if spanNameFn == nil {
		spanNameFn = defaultPublishSpanName
	}
	spanAttrs := publishSpanAttributes(exchangeName, routingKey, source)
	ctx, span := tracer.Start(ctx, spanNameFn(exchangeName, routingKey),
		trace.WithSpanKind(trace.SpanKindProducer),
		trace.WithAttributes(spanAttrs...),
	)
	defer span.End()

	jsonBytes, err := json.Marshal(msg)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	if headers == nil {
		headers = amqp.Table{}
	}

	// Normalize user-supplied ce-* headers to cloudEvents:* for AMQP wire format.
	// This ensures backward compat: users passing spec.CEID ("ce-id") as a custom
	// header will have it correctly mapped to "cloudEvents:id" on the wire.
	//
	// NOTE: Deleting and inserting during range is safe per Go spec. The new
	// "cloudEvents:*" keys won't match the "ce-" prefix guard, so no double-processing.
	for k, v := range headers {
		if strings.HasPrefix(k, "ce-") {
			amqpKey := "cloudEvents:" + strings.TrimPrefix(k, "ce-")
			if _, exists := headers[amqpKey]; !exists {
				headers[amqpKey] = v
			}
			delete(headers, k)
		}
	}

	setDefault := func(key, value string) {
		if _, exists := headers[key]; !exists {
			headers[key] = value
		}
	}
	setDefault(spec.AMQPCEHeaderKey(spec.CEAttrSpecVersion), spec.CESpecVersionValue)
	setDefault(spec.AMQPCEHeaderKey(spec.CEAttrType), routingKey)
	setDefault(spec.AMQPCEHeaderKey(spec.CEAttrSource), source)
	setDefault(spec.AMQPCEHeaderKey(spec.CEAttrDataContentType), contentType)
	setDefault(spec.AMQPCEHeaderKey(spec.CEAttrTime), time.Now().UTC().Format(time.RFC3339))

	messageID := uuid.New().String()
	amqpIDKey := spec.AMQPCEHeaderKey(spec.CEAttrID)
	if existing, ok := headers[amqpIDKey].(string); ok && existing != "" {
		messageID = existing
	}
	headers[amqpIDKey] = messageID

	span.SetAttributes(
		attrMessagingMessageID.String(messageID),
		attrMessagingBodySize.Int(len(jsonBytes)),
	)

	publishing := amqp.Publishing{
		Body:         jsonBytes,
		ContentType:  contentType,
		DeliveryMode: 2,
		Headers:      injectToHeaders(ctx, headers, prop),
	}
	startTime := time.Now()
	err = channel.PublishWithContext(ctx, exchangeName,
		routingKey,
		false,
		false,
		publishing,
	)
	elapsed := time.Since(startTime).Milliseconds()
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		eventPublishFailed(exchangeName, routingKey, elapsed)
		return err
	}
	span.SetStatus(codes.Ok, "")
	eventPublishSucceed(exchangeName, routingKey, elapsed)
	return nil
}
