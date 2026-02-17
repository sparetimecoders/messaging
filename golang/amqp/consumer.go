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
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/sparetimecoders/gomessaging/spec"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

type queueConsumer struct {
	queue          string
	serviceName    string
	handlers       routingKeyHandler
	notificationCh chan<- spec.Notification
	errorCh        chan<- spec.ErrorNotification
	spanNameFn     func(info spec.DeliveryInfo) string
	logger         *slog.Logger
	tracer         trace.Tracer
	propagator     propagation.TextMapPropagator
	legacySupport  bool
}

func (c *queueConsumer) log() *slog.Logger {
	if c.logger != nil {
		return c.logger
	}
	return slog.Default()
}

func (c *queueConsumer) consume(channel amqpChannel, notificationCh chan<- spec.Notification, errorCh chan<- spec.ErrorNotification) (<-chan amqp.Delivery, error) {
	c.notificationCh = notificationCh
	c.errorCh = errorCh
	deliveries, err := channel.Consume(c.queue, "", false, false, false, false, nil)
	if err != nil {
		return nil, err
	}
	return deliveries, nil
}

func (c *queueConsumer) loop(deliveries <-chan amqp.Delivery) {
	for delivery := range deliveries {
		deliveryInfo := getDeliveryInfo(c.queue, delivery)
		eventReceived(c.queue, deliveryInfo.Key)

		// Establish which handler is invoked
		handler, ok := c.handlers.get(deliveryInfo.Key)
		if !ok {
			c.log().Warn("no handler for routing key, rejecting",
				"routingKey", deliveryInfo.Key,
				"exchange", deliveryInfo.Source,
			)
			eventWithoutHandler(c.queue, deliveryInfo.Key)
			_ = delivery.Reject(false)
			continue
		}
		c.handleDelivery(handler, delivery, deliveryInfo)
	}
	c.log().Error("consumer loop exited, delivery channel closed", "queue", c.queue)
}

func (c *queueConsumer) handleDelivery(handler wrappedHandler, delivery amqp.Delivery, deliveryInfo spec.DeliveryInfo) {
	// Normalize CE headers first: accept cloudEvents:*, cloudEvents_*, and ce-* prefixes.
	// IMPORTANT: This MUST happen before HasCEHeaders/ValidateCEHeaders/MetadataFromHeaders,
	// which all expect the canonical "ce-" prefix.
	deliveryInfo.Headers = spec.NormalizeCEHeaders(deliveryInfo.Headers)

	isLegacy := !spec.HasCEHeaders(deliveryInfo.Headers)
	if isLegacy {
		c.log().Debug("received legacy message without CloudEvents headers",
			"routingKey", deliveryInfo.Key,
			"exchange", deliveryInfo.Source,
		)
	} else if warnings := spec.ValidateCEHeaders(deliveryInfo.Headers); len(warnings) > 0 {
		c.log().Warn("incoming message has invalid CloudEvents headers",
			"routingKey", deliveryInfo.Key,
			"exchange", deliveryInfo.Source,
			"warnings", warnings,
		)
	}

	headerCtx := extractToContext(delivery.Headers, c.propagator)

	metadata := spec.MetadataFromHeaders(deliveryInfo.Headers)
	if isLegacy && c.legacySupport {
		metadata = spec.EnrichLegacyMetadata(metadata, deliveryInfo, func() string {
			return uuid.New().String()
		})
		c.log().Debug("enriched legacy message with synthetic metadata",
			"routingKey", deliveryInfo.Key,
			"syntheticID", metadata.ID,
		)
	}

	messageID := metadata.ID
	spanAttrs := consumerSpanAttributes(deliveryInfo, c.serviceName, messageID, len(delivery.Body))
	tracingCtx, span := c.getTracer().Start(headerCtx, c.spanNameFn(deliveryInfo),
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(spanAttrs...),
	)
	defer span.End()
	startTime := time.Now()

	uevt := unmarshalEvent{Metadata: metadata, DeliveryInfo: deliveryInfo, Payload: delivery.Body}
	if err := handler(tracingCtx, uevt); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		elapsed := time.Since(startTime).Milliseconds()
		notifyEventHandlerFailed(c.errorCh, deliveryInfo, elapsed, err)
		if errors.Is(err, spec.ErrParseJSON) {
			c.log().Warn("failed to parse message, nacking without requeue",
				"routingKey", deliveryInfo.Key,
				"exchange", deliveryInfo.Source,
				"error", err,
			)
			eventNotParsable(deliveryInfo.Destination, deliveryInfo.Key)
			_ = delivery.Nack(false, false)
		} else if errors.Is(err, ErrNoMessageTypeForRouteKey) {
			c.log().Warn("no type mapping for routing key, rejecting",
				"routingKey", deliveryInfo.Key,
				"exchange", deliveryInfo.Source,
			)
			eventWithoutHandler(deliveryInfo.Destination, deliveryInfo.Key)
			_ = delivery.Reject(false)
		} else {
			c.log().Error("handler failed, nacking with requeue",
				"routingKey", deliveryInfo.Key,
				"exchange", deliveryInfo.Source,
				"error", err,
				"durationMs", elapsed,
			)
			eventNack(deliveryInfo.Destination, deliveryInfo.Key, elapsed)
			_ = delivery.Nack(false, true)
		}
		return
	}

	elapsed := time.Since(startTime).Milliseconds()
	span.SetStatus(codes.Ok, "")
	notifyEventHandlerSucceed(c.notificationCh, deliveryInfo, elapsed)
	_ = delivery.Ack(false)
	eventAck(deliveryInfo.Destination, deliveryInfo.Key, elapsed)
	c.log().Debug("message processed",
		"routingKey", deliveryInfo.Key,
		"exchange", deliveryInfo.Source,
		"durationMs", elapsed,
	)
}

func (c *queueConsumer) getTracer() trace.Tracer {
	if c.tracer != nil {
		return c.tracer
	}
	return tracerFromProvider(nil)
}

type queueConsumers struct {
	consumers  map[string]*queueConsumer
	spanNameFn func(info spec.DeliveryInfo) string
}

func (c *queueConsumers) get(queueName, routingKey string) (wrappedHandler, bool) {
	consumerForQueue, ok := c.consumers[queueName]
	if !ok {
		return nil, false
	}
	return consumerForQueue.handlers.get(routingKey)
}

func (c *queueConsumers) add(queueName, routingKey string, handler wrappedHandler) error {
	consumerForQueue, ok := c.consumers[queueName]
	if !ok {
		consumerForQueue = &queueConsumer{
			queue:      queueName,
			handlers:   make(routingKeyHandler),
			spanNameFn: c.spanNameFn,
		}
		c.consumers[queueName] = consumerForQueue
	}
	if mappedRoutingKey, exists := consumerForQueue.handlers.exists(routingKey); exists {
		return fmt.Errorf("routingkey %s overlaps %s for queue %s, consider using AddQueueNameSuffix", routingKey, mappedRoutingKey, queueName)
	}
	consumerForQueue.handlers.add(routingKey, handler)
	return nil
}

func getDeliveryInfo(queueName string, delivery amqp.Delivery) spec.DeliveryInfo {
	deliveryInfo := spec.DeliveryInfo{
		Destination: queueName,
		Source:      delivery.Exchange,
		Key:         delivery.RoutingKey,
		Headers:     spec.Headers(delivery.Headers),
	}
	return deliveryInfo
}
