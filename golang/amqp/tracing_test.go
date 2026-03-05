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
	"errors"
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/sparetimecoders/messaging/specification/spec"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func newTestTracerProvider() (*tracetest.InMemoryExporter, trace.TracerProvider) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	return exporter, tp
}

func spanAttr(span tracetest.SpanStub, key string) attribute.Value {
	for _, a := range span.Attributes {
		if string(a.Key) == key {
			return a.Value
		}
	}
	return attribute.Value{}
}

func Test_PublishSpanAttributes(t *testing.T) {
	exporter, tp := newTestTracerProvider()
	channel := NewMockAmqpChannel()

	tracer := tp.Tracer(tracerName)
	err := publishMessage(context.Background(), tracer, nil, nil, channel, Message{true}, "order.created", "events.topic.exchange", "orders-svc", nil)
	require.NoError(t, err)

	spans := exporter.GetSpans()
	require.Len(t, spans, 1)

	s := spans[0]
	require.Equal(t, "publish order.created", s.Name)
	require.Equal(t, trace.SpanKindProducer, s.SpanKind)

	require.Equal(t, "rabbitmq", spanAttr(s, "messaging.system").AsString())
	require.Equal(t, "publish", spanAttr(s, "messaging.operation").AsString())
	require.Equal(t, "events.topic.exchange", spanAttr(s, "messaging.destination.name").AsString())
	require.Equal(t, "order.created", spanAttr(s, "messaging.rabbitmq.routing_key").AsString())
	require.Equal(t, "orders-svc", spanAttr(s, "messaging.client_id").AsString())

	// message.id and body.size are set after marshal via SetAttributes
	require.NotEmpty(t, spanAttr(s, "messaging.message.id").AsString())
	body, _ := json.Marshal(Message{true})
	require.Equal(t, int64(len(body)), spanAttr(s, "messaging.message.body.size").AsInt64())

	require.Equal(t, codes.Ok, s.Status.Code)
}

func Test_PublishSpanError(t *testing.T) {
	exporter, tp := newTestTracerProvider()
	channel := NewMockAmqpChannel()
	channel.publishFn = func(ctx context.Context, exchange, key string, mandatory, immediate bool, msg amqp.Publishing) error {
		return errors.New("connection lost")
	}

	tracer := tp.Tracer(tracerName)
	err := publishMessage(context.Background(), tracer, nil, nil, channel, Message{true}, "key", "exchange", "svc", nil)
	require.Error(t, err)

	spans := exporter.GetSpans()
	require.Len(t, spans, 1)

	s := spans[0]
	require.Equal(t, codes.Error, s.Status.Code)
	require.Equal(t, "connection lost", s.Status.Description)
	require.Len(t, s.Events, 1)
	require.Equal(t, "exception", s.Events[0].Name)
}

func Test_PublishSpanMarshalError(t *testing.T) {
	exporter, tp := newTestTracerProvider()
	channel := NewMockAmqpChannel()

	tracer := tp.Tracer(tracerName)
	// channel with unsupported value to trigger marshal error
	err := publishMessage(context.Background(), tracer, nil, nil, channel, func() {}, "key", "exchange", "svc", nil)
	require.Error(t, err)

	spans := exporter.GetSpans()
	require.Len(t, spans, 1)
	require.Equal(t, codes.Error, spans[0].Status.Code)
}

func Test_ConsumerSpanAttributes(t *testing.T) {
	exporter, tp := newTestTracerProvider()
	tracer := tp.Tracer(tracerName)

	acker := MockAcknowledger{
		Acks:    make(chan Ack, 1),
		Nacks:   make(chan Nack, 1),
		Rejects: make(chan Reject, 1),
	}

	handler := newWrappedHandler(func(ctx context.Context, event spec.ConsumableEvent[Message]) error {
		return nil
	})

	body, _ := json.Marshal(Message{true})
	consumer := queueConsumer{
		queue:       "events.topic.exchange.queue.test-svc",
		serviceName: "test-svc",
		handlers:    routingKeyHandler{},
		spanNameFn: func(info spec.DeliveryInfo) string {
			return "test-span"
		},
		tracer: tracer,
	}
	consumer.handlers.add("order.created", handler)

	d := amqp.Delivery{
		Body:         body,
		RoutingKey:   "order.created",
		Exchange:     "events.topic.exchange",
		Acknowledger: &acker,
		Headers: amqp.Table{
			spec.CEID: "msg-123",
		},
	}

	deliveries := make(chan amqp.Delivery, 1)
	deliveries <- d
	close(deliveries)
	consumer.loop(deliveries)

	spans := exporter.GetSpans()
	require.Len(t, spans, 1)

	s := spans[0]
	require.Equal(t, "test-span", s.Name)
	require.Equal(t, trace.SpanKindConsumer, s.SpanKind)

	require.Equal(t, "rabbitmq", spanAttr(s, "messaging.system").AsString())
	require.Equal(t, "receive", spanAttr(s, "messaging.operation").AsString())
	require.Equal(t, "events.topic.exchange", spanAttr(s, "messaging.destination.name").AsString())
	require.Equal(t, "order.created", spanAttr(s, "messaging.rabbitmq.routing_key").AsString())
	require.Equal(t, "events.topic.exchange.queue.test-svc", spanAttr(s, "messaging.destination.queue").AsString())
	require.Equal(t, "msg-123", spanAttr(s, "messaging.message.id").AsString())
	require.Equal(t, int64(len(body)), spanAttr(s, "messaging.message.body.size").AsInt64())
	require.Equal(t, "test-svc", spanAttr(s, "messaging.client_id").AsString())

	require.Equal(t, codes.Ok, s.Status.Code)
}

func Test_ConsumerSpanError(t *testing.T) {
	exporter, tp := newTestTracerProvider()
	tracer := tp.Tracer(tracerName)

	acker := MockAcknowledger{
		Acks:    make(chan Ack, 1),
		Nacks:   make(chan Nack, 1),
		Rejects: make(chan Reject, 1),
	}

	handler := newWrappedHandler(func(ctx context.Context, event spec.ConsumableEvent[Message]) error {
		return errors.New("handler failed")
	})

	body, _ := json.Marshal(Message{true})
	errorCh := make(chan spec.ErrorNotification, 1)
	consumer := queueConsumer{
		queue:       "q",
		serviceName: "svc",
		handlers:    routingKeyHandler{},
		errorCh:     errorCh,
		spanNameFn: func(info spec.DeliveryInfo) string {
			return "span"
		},
		tracer: tracer,
	}
	consumer.handlers.add("key", handler)

	deliveries := make(chan amqp.Delivery, 1)
	deliveries <- amqp.Delivery{
		Body:         body,
		RoutingKey:   "key",
		Acknowledger: &acker,
	}
	close(deliveries)
	consumer.loop(deliveries)

	spans := exporter.GetSpans()
	require.Len(t, spans, 1)
	require.Equal(t, codes.Error, spans[0].Status.Code)
	require.Equal(t, "handler failed", spans[0].Status.Description)
}

func Test_ContextPropagation(t *testing.T) {
	exporter, tp := newTestTracerProvider()
	tracer := tp.Tracer(tracerName)
	prop := propagation.TraceContext{}

	// Publish with a parent span so trace context is injected into headers
	ctx, parentSpan := tracer.Start(context.Background(), "parent")

	channel := NewMockAmqpChannel()
	err := publishMessage(ctx, tracer, prop, nil, channel, Message{true}, "key", "exchange", "svc", nil)
	require.NoError(t, err)

	published := <-channel.Published

	// Consume — extract context from published headers
	acker := MockAcknowledger{
		Acks:    make(chan Ack, 1),
		Nacks:   make(chan Nack, 1),
		Rejects: make(chan Reject, 1),
	}
	handler := newWrappedHandler(func(ctx context.Context, event spec.ConsumableEvent[Message]) error {
		return nil
	})
	consumer := queueConsumer{
		queue:       "q",
		serviceName: "svc",
		handlers:    routingKeyHandler{},
		spanNameFn:  func(info spec.DeliveryInfo) string { return "consume" },
		tracer:      tracer,
		propagator:  prop,
	}
	consumer.handlers.add("key", handler)

	body, _ := json.Marshal(Message{true})
	deliveries := make(chan amqp.Delivery, 1)
	deliveries <- amqp.Delivery{
		Body:         body,
		RoutingKey:   "key",
		Exchange:     "exchange",
		Headers:      published.msg.Headers,
		Acknowledger: &acker,
	}
	close(deliveries)
	consumer.loop(deliveries)
	parentSpan.End()

	spans := exporter.GetSpans()
	// Should have: publish span, consume span, parent span
	require.Len(t, spans, 3)

	// All spans share the same trace ID
	traceID := spans[0].SpanContext.TraceID()
	for _, s := range spans {
		require.Equal(t, traceID, s.SpanContext.TraceID())
	}
}

func Test_CustomPropagator(t *testing.T) {
	exporter, tp := newTestTracerProvider()
	tracer := tp.Tracer(tracerName)

	// Use a custom propagator that injects a custom header
	customProp := &testPropagator{key: "x-custom-trace"}

	ctx, span := tracer.Start(context.Background(), "parent")
	defer span.End()

	channel := NewMockAmqpChannel()
	err := publishMessage(ctx, tracer, customProp, nil, channel, Message{true}, "key", "exchange", "svc", nil)
	require.NoError(t, err)

	published := <-channel.Published
	// The custom propagator should have injected its header
	require.NotEmpty(t, published.msg.Headers["x-custom-trace"])

	spans := exporter.GetSpans()
	require.True(t, len(spans) >= 1)
	_ = exporter
}

func Test_CustomPublishSpanNameFn(t *testing.T) {
	exporter, tp := newTestTracerProvider()
	tracer := tp.Tracer(tracerName)
	channel := NewMockAmqpChannel()

	customFn := func(exchange, routingKey string) string {
		return exchange + "/" + routingKey
	}
	err := publishMessage(context.Background(), tracer, nil, customFn, channel, Message{true}, "order.created", "events.topic.exchange", "svc", nil)
	require.NoError(t, err)

	spans := exporter.GetSpans()
	require.Len(t, spans, 1)
	require.Equal(t, "events.topic.exchange/order.created", spans[0].Name)
}

func Test_DefaultPublishSpanName(t *testing.T) {
	require.Equal(t, "publish order.created", defaultPublishSpanName("events.topic.exchange", "order.created"))
}

func Test_PropagatorOrGlobal_Nil(t *testing.T) {
	p := propagatorOrGlobal(nil)
	require.NotNil(t, p)
}

func Test_PropagatorOrGlobal_Custom(t *testing.T) {
	custom := propagation.TraceContext{}
	p := propagatorOrGlobal(custom)
	require.Equal(t, custom, p)
}

// testPropagator is a minimal TextMapPropagator for testing.
type testPropagator struct {
	key string
}

func (p *testPropagator) Inject(ctx context.Context, carrier propagation.TextMapCarrier) {
	sc := trace.SpanContextFromContext(ctx)
	if sc.IsValid() {
		carrier.Set(p.key, sc.TraceID().String())
	}
}

func (p *testPropagator) Extract(ctx context.Context, carrier propagation.TextMapCarrier) context.Context {
	return ctx
}

func (p *testPropagator) Fields() []string {
	return []string{p.key}
}
