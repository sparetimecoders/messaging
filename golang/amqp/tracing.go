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
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/sparetimecoders/gomessaging/spec"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

const tracerName = "github.com/sparetimecoders/gomessaging/amqp"

// OTel semantic convention attribute keys for messaging.
// Using string constants to avoid a dependency on the semconv package.
var (
	attrMessagingSystem       = attribute.String("messaging.system", "rabbitmq")
	attrMessagingOperationKey = attribute.Key("messaging.operation")
	attrMessagingDestKey      = attribute.Key("messaging.destination.name")
	attrMessagingRoutingKey   = attribute.Key("messaging.rabbitmq.routing_key")
	attrMessagingMessageID    = attribute.Key("messaging.message.id")
	attrMessagingBodySize     = attribute.Key("messaging.message.body.size")
	attrMessagingClientID     = attribute.Key("messaging.client_id")
)

// tracerFromProvider returns a Tracer from the given provider, falling back to
// the global TracerProvider if provider is nil.
func tracerFromProvider(provider trace.TracerProvider) trace.Tracer {
	if provider == nil {
		provider = otel.GetTracerProvider()
	}
	return provider.Tracer(tracerName)
}

// consumerSpanAttributes returns span attributes for a consume operation.
func consumerSpanAttributes(info spec.DeliveryInfo, serviceName, messageID string, bodySize int) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		attrMessagingSystem,
		attrMessagingOperationKey.String("receive"),
		attrMessagingDestKey.String(info.Source),
		attrMessagingRoutingKey.String(info.Key),
		attribute.String("messaging.destination.queue", info.Destination),
		attrMessagingBodySize.Int(bodySize),
		attrMessagingClientID.String(serviceName),
	}
	if messageID != "" {
		attrs = append(attrs, attrMessagingMessageID.String(messageID))
	}
	return attrs
}

// publishSpanAttributes returns span attributes for a publish operation.
func publishSpanAttributes(exchange, routingKey, serviceName string) []attribute.KeyValue {
	return []attribute.KeyValue{
		attrMessagingSystem,
		attrMessagingOperationKey.String("publish"),
		attrMessagingDestKey.String(exchange),
		attrMessagingRoutingKey.String(routingKey),
		attrMessagingClientID.String(serviceName),
	}
}

// defaultPublishSpanName returns the default span name for a publish operation.
func defaultPublishSpanName(_, routingKey string) string {
	return fmt.Sprintf("publish %s", routingKey)
}

// propagatorOrGlobal returns p if non-nil, otherwise the global propagator.
func propagatorOrGlobal(p propagation.TextMapPropagator) propagation.TextMapPropagator {
	if p != nil {
		return p
	}
	return otel.GetTextMapPropagator()
}

// injectToHeaders injects the span context into AMQP headers for propagation.
func injectToHeaders(ctx context.Context, headers amqp.Table, p propagation.TextMapPropagator) amqp.Table {
	carrier := propagation.MapCarrier{}
	propagatorOrGlobal(p).Inject(ctx, carrier)
	for k, v := range carrier {
		headers[k] = v
	}
	return headers
}

// extractToContext extracts the span context from AMQP headers.
func extractToContext(headers amqp.Table, p propagation.TextMapPropagator) context.Context {
	carrier := propagation.MapCarrier{}
	for k, v := range headers {
		value, ok := v.(string)
		if ok {
			carrier[k] = value
		}
	}

	return propagatorOrGlobal(p).Extract(context.Background(), carrier)
}
