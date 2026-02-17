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
	"fmt"

	natsgo "github.com/nats-io/nats.go"
	"github.com/sparetimecoders/gomessaging/spec"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

const tracerName = "github.com/sparetimecoders/gomessaging/nats"

var (
	attrMessagingSystem       = attribute.String("messaging.system", "nats")
	attrMessagingOperationKey = attribute.Key("messaging.operation")
	attrMessagingDestKey      = attribute.Key("messaging.destination.name")
	attrMessagingRoutingKey   = attribute.Key("messaging.nats.subject")
	attrMessagingMessageID    = attribute.Key("messaging.message.id")
	attrMessagingBodySize     = attribute.Key("messaging.message.body.size")
	attrMessagingClientID     = attribute.Key("messaging.client_id")
	attrMessagingConsumer     = attribute.Key("messaging.destination.subscription")
)

func tracerFromProvider(provider trace.TracerProvider) trace.Tracer {
	if provider == nil {
		provider = otel.GetTracerProvider()
	}
	return provider.Tracer(tracerName)
}

func consumerSpanAttributes(info spec.DeliveryInfo, serviceName, messageID string, bodySize int) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		attrMessagingSystem,
		attrMessagingOperationKey.String("receive"),
		attrMessagingDestKey.String(info.Source),
		attrMessagingRoutingKey.String(info.Key),
		attrMessagingConsumer.String(info.Destination),
		attrMessagingBodySize.Int(bodySize),
		attrMessagingClientID.String(serviceName),
	}
	if messageID != "" {
		attrs = append(attrs, attrMessagingMessageID.String(messageID))
	}
	return attrs
}

func publishSpanAttributes(stream, subject, serviceName string) []attribute.KeyValue {
	return []attribute.KeyValue{
		attrMessagingSystem,
		attrMessagingOperationKey.String("publish"),
		attrMessagingDestKey.String(stream),
		attrMessagingRoutingKey.String(subject),
		attrMessagingClientID.String(serviceName),
	}
}

func defaultPublishSpanName(_, routingKey string) string {
	return fmt.Sprintf("publish %s", routingKey)
}

func propagatorOrGlobal(p propagation.TextMapPropagator) propagation.TextMapPropagator {
	if p != nil {
		return p
	}
	return otel.GetTextMapPropagator()
}

// injectToHeaders injects the span context into NATS headers for propagation.
func injectToHeaders(ctx context.Context, headers natsgo.Header, p propagation.TextMapPropagator) natsgo.Header {
	carrier := propagation.MapCarrier{}
	propagatorOrGlobal(p).Inject(ctx, carrier)
	for k, v := range carrier {
		headers.Set(k, v)
	}
	return headers
}

// extractToContext extracts the span context from NATS headers.
func extractToContext(headers natsgo.Header, p propagation.TextMapPropagator) context.Context {
	carrier := propagation.MapCarrier{}
	for k := range headers {
		carrier[k] = headers.Get(k)
	}
	return propagatorOrGlobal(p).Extract(context.Background(), carrier)
}
