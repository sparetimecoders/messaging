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

	natsgo "github.com/nats-io/nats.go"
	"github.com/sparetimecoders/messaging/specification/spec"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// RequestResponseHandler sets up a ServiceRequestConsumer that automatically
// sends the handler's response back via NATS reply.
func RequestResponseHandler[T any, R any](routingKey string, handler spec.RequestResponseEventHandler[T, R]) Setup {
	return func(c *Connection) error {
		wrappedFn := requestResponseWrapper[T, R](handler)
		if !c.collectMode {
			subject := serviceRequestSubject(c.serviceName, routingKey)
			if err := c.startRequestResponseConsumer(subject, routingKey, c.serviceName, wrappedFn); err != nil {
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
		})
		return nil
	}
}

type requestResponseFn func(ctx context.Context, event unmarshalEvent) (json.RawMessage, error)

func requestResponseWrapper[T any, R any](handler spec.RequestResponseEventHandler[T, R]) requestResponseFn {
	return func(ctx context.Context, event unmarshalEvent) (json.RawMessage, error) {
		var payload T
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return nil, fmt.Errorf("%v: %w", err, spec.ErrParseJSON)
		}

		consumableEvent := spec.ConsumableEvent[T]{
			Metadata:     event.Metadata,
			DeliveryInfo: event.DeliveryInfo,
			Payload:      payload,
		}
		resp, err := handler(ctx, consumableEvent)
		if err != nil {
			return nil, err
		}

		respBytes, err := json.Marshal(resp)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal response: %w", err)
		}
		return respBytes, nil
	}
}

// startRequestResponseConsumer subscribes to a NATS Core subject and sends responses via reply.
func (c *Connection) startRequestResponseConsumer(subject, routingKey, consName string, handler requestResponseFn) error {
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

	sub, err := c.nc.Subscribe(subject, func(msg *natsgo.Msg) {
		headers := fromNATSHeaders(msg.Header)
		deliveryInfo := spec.DeliveryInfo{
			Destination: consName,
			Source:      msg.Subject,
			Key:         routingKey,
			Headers:     headers,
		}

		uevt := unmarshalEvent{
			Metadata:     spec.MetadataFromHeaders(headers),
			DeliveryInfo: deliveryInfo,
			Payload:      msg.Data,
		}

		headerCtx := extractToContext(msg.Header, consumer.propagator)
		messageID, _ := headers[spec.CEID].(string)
		spanAttrs := consumerSpanAttributes(deliveryInfo, c.serviceName, messageID, len(msg.Data))
		tracingCtx, span := consumer.getTracer().Start(headerCtx, consumer.spanNameFn(deliveryInfo),
			trace.WithSpanKind(trace.SpanKindConsumer),
			trace.WithAttributes(spanAttrs...),
		)
		defer span.End()

		resp, err := handler(tracingCtx, uevt)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			consumer.log().Error("request handler failed",
				"routingKey", routingKey,
				"error", err,
			)
			if msg.Reply != "" {
				errResp, _ := json.Marshal(map[string]string{"error": err.Error()})
				_ = msg.Respond(errResp)
			}
			return
		}

		span.SetStatus(codes.Ok, "")
		if msg.Reply != "" {
			_ = msg.Respond(resp)
		}
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe to %s: %w", subject, err)
	}

	c.subscriptions = append(c.subscriptions, sub)
	c.log().Info("started request-response consumer", "subject", subject)
	return nil
}
