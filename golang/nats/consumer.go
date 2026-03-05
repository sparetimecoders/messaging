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
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	natsgo "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/sparetimecoders/messaging/specification/spec"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// consumerConfig configures a JetStream or Core NATS consumer.
type consumerConfig struct {
	stream       string
	routingKey   string
	consumerName string
	ephemeral    bool
	handler      wrappedHandler
	suffix       string
	maxDeliver   int             // 0 = use connection default
	backOff      []time.Duration // nil = use connection default
}

// ConsumerOptions is a setup function for consumer configuration.
type ConsumerOptions func(config *consumerConfig) error

// AddConsumerNameSuffix appends the provided suffix to the consumer name.
func AddConsumerNameSuffix(suffix string) ConsumerOptions {
	return func(config *consumerConfig) error {
		if suffix == "" {
			return ErrEmptySuffix
		}
		config.suffix = suffix
		return nil
	}
}

// WithMaxDeliver sets the maximum number of delivery attempts for this consumer.
// After max deliveries, the message is terminated by the server.
// Zero means use the connection-level default (see WithConsumerDefaults).
func WithMaxDeliver(n int) ConsumerOptions {
	return func(config *consumerConfig) error {
		if n < 0 {
			return fmt.Errorf("MaxDeliver must be >= 0, got %d", n)
		}
		config.maxDeliver = n
		return nil
	}
}

// WithBackOff sets redelivery backoff durations for this consumer.
// The server applies these delays between redelivery attempts.
// Requires MaxDeliver >= len(durations).
func WithBackOff(durations ...time.Duration) ConsumerOptions {
	return func(config *consumerConfig) error {
		config.backOff = durations
		return nil
	}
}

func newConsumerConfig(stream, routingKey string, ephemeral bool, handler wrappedHandler, serviceName string, opts ...ConsumerOptions) (*consumerConfig, error) {
	cfg := &consumerConfig{
		stream:     stream,
		routingKey: routingKey,
		ephemeral:  ephemeral,
		handler:    handler,
	}

	for _, f := range opts {
		if err := f(cfg); err != nil {
			return nil, fmt.Errorf("consumer option failed: %w", err)
		}
	}

	if !ephemeral {
		name := consumerName(serviceName)
		if cfg.suffix != "" {
			name = consumerNameWithSuffix(serviceName, cfg.suffix)
		}
		cfg.consumerName = name
	}

	return cfg, nil
}

// jsConsumer manages a single JetStream consumer's message loop.
type jsConsumer struct {
	name           string
	stream         string
	serviceName    string
	handlers       routingKeyHandler
	notificationCh chan<- spec.Notification
	errorCh        chan<- spec.ErrorNotification
	spanNameFn     func(info spec.DeliveryInfo) string
	logger         *slog.Logger
	tracer         trace.Tracer
	propagator     propagation.TextMapPropagator
}

func (c *jsConsumer) log() *slog.Logger {
	if c.logger != nil {
		return c.logger
	}
	return slog.Default()
}

func (c *jsConsumer) getTracer() trace.Tracer {
	if c.tracer != nil {
		return c.tracer
	}
	return tracerFromProvider(nil)
}

// handleMessage processes a single JetStream message.
func (c *jsConsumer) handleMessage(msg jetstream.Msg) {
	subject := msg.Subject()
	// Extract the routing key: strip stream prefix from subject
	routingKey := subject
	if idx := strings.Index(subject, "."); idx >= 0 {
		routingKey = subject[idx+1:]
	}

	eventReceived(c.name, routingKey)

	handler, ok := c.handlers.get(routingKey)
	if !ok {
		c.log().Warn("no handler for routing key, rejecting",
			"routingKey", routingKey,
			"stream", c.stream,
		)
		eventWithoutHandler(c.name, routingKey)
		_ = msg.Term()
		return
	}

	c.handleDelivery(handler, msg, routingKey)
}

func (c *jsConsumer) handleDelivery(handler wrappedHandler, msg jetstream.Msg, routingKey string) {
	headers := fromNATSHeaders(msg.Headers())

	deliveryInfo := spec.DeliveryInfo{
		Destination: c.name,
		Source:      c.stream,
		Key:         routingKey,
		Headers:     headers,
	}

	if warnings := spec.ValidateCEHeaders(headers); len(warnings) > 0 {
		c.log().Warn("incoming message has invalid CloudEvents headers",
			"routingKey", routingKey,
			"stream", c.stream,
			"warnings", warnings,
		)
	}

	headerCtx := extractToContext(msg.Headers(), c.propagator)
	messageID, _ := headers[spec.CEID].(string)
	spanAttrs := consumerSpanAttributes(deliveryInfo, c.serviceName, messageID, len(msg.Data()))
	tracingCtx, span := c.getTracer().Start(headerCtx, c.spanNameFn(deliveryInfo),
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(spanAttrs...),
	)
	defer span.End()
	startTime := time.Now()

	uevt := unmarshalEvent{
		Metadata:     spec.MetadataFromHeaders(headers),
		DeliveryInfo: deliveryInfo,
		Payload:      msg.Data(),
	}
	if err := handler(tracingCtx, uevt); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		elapsed := time.Since(startTime).Milliseconds()
		notifyEventHandlerFailed(c.errorCh, deliveryInfo, elapsed, err)
		if errors.Is(err, spec.ErrParseJSON) {
			c.log().Warn("failed to parse message, terminating",
				"routingKey", routingKey,
				"stream", c.stream,
				"error", err,
			)
			eventNotParsable(c.name, routingKey)
			_ = msg.Term()
		} else if errors.Is(err, ErrNoMessageTypeForRouteKey) {
			c.log().Warn("no type mapping for routing key, terminating",
				"routingKey", routingKey,
				"stream", c.stream,
			)
			eventWithoutHandler(c.name, routingKey)
			_ = msg.Term()
		} else {
			c.log().Error("handler failed, naking for redelivery",
				"routingKey", routingKey,
				"stream", c.stream,
				"error", err,
				"durationMs", elapsed,
			)
			eventNak(c.name, routingKey, elapsed)
			_ = msg.Nak()
		}
		return
	}

	elapsed := time.Since(startTime).Milliseconds()
	span.SetStatus(codes.Ok, "")
	notifyEventHandlerSucceed(c.notificationCh, deliveryInfo, elapsed)
	_ = msg.Ack()
	eventAck(c.name, routingKey, elapsed)
	c.log().Debug("message processed",
		"routingKey", routingKey,
		"stream", c.stream,
		"durationMs", elapsed,
	)
}

// handleCoreMessage processes a NATS Core message (request-reply pattern).
func (c *jsConsumer) handleCoreMessage(msg *natsgo.Msg, routingKey string) {
	eventReceived(c.name, routingKey)

	handler, ok := c.handlers.get(routingKey)
	if !ok {
		c.log().Warn("no handler for routing key",
			"routingKey", routingKey,
			"subject", msg.Subject,
		)
		eventWithoutHandler(c.name, routingKey)
		return
	}

	headers := fromNATSHeaders(msg.Header)
	deliveryInfo := spec.DeliveryInfo{
		Destination: c.name,
		Source:      msg.Subject,
		Key:         routingKey,
		Headers:     headers,
	}

	headerCtx := extractToContext(msg.Header, c.propagator)
	messageID, _ := headers[spec.CEID].(string)
	spanAttrs := consumerSpanAttributes(deliveryInfo, c.serviceName, messageID, len(msg.Data))
	tracingCtx, span := c.getTracer().Start(headerCtx, c.spanNameFn(deliveryInfo),
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(spanAttrs...),
	)
	defer span.End()
	startTime := time.Now()

	uevt := unmarshalEvent{
		Metadata:     spec.MetadataFromHeaders(headers),
		DeliveryInfo: deliveryInfo,
		Payload:      msg.Data,
	}
	if err := handler(tracingCtx, uevt); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		elapsed := time.Since(startTime).Milliseconds()
		notifyEventHandlerFailed(c.errorCh, deliveryInfo, elapsed, err)
		c.log().Error("handler failed",
			"routingKey", routingKey,
			"error", err,
			"durationMs", elapsed,
		)
		eventNak(c.name, routingKey, elapsed)
		// For request-reply, respond with error
		if msg.Reply != "" {
			errResp, _ := json.Marshal(map[string]string{"error": err.Error()})
			_ = msg.Respond(errResp)
		}
		return
	}

	elapsed := time.Since(startTime).Milliseconds()
	span.SetStatus(codes.Ok, "")
	notifyEventHandlerSucceed(c.notificationCh, deliveryInfo, elapsed)
	eventAck(c.name, routingKey, elapsed)
	c.log().Debug("message processed",
		"routingKey", routingKey,
		"durationMs", elapsed,
	)
}
