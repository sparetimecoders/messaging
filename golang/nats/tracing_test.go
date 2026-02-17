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
	"sync"
	"testing"
	"time"

	"github.com/sparetimecoders/gomessaging/spec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestTracingPublishAndConsume(t *testing.T) {
	s := startTestServer(t)
	url := serverURL(s)

	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))

	var (
		mu       sync.Mutex
		received bool
	)

	handler := func(ctx context.Context, event spec.ConsumableEvent[testMessage]) error {
		mu.Lock()
		defer mu.Unlock()
		received = true
		return nil
	}

	pub := NewPublisher()
	conn, err := NewConnection("tracing-svc", url)
	require.NoError(t, err)

	err = conn.Start(context.Background(),
		WithTracing(tp),
		EventStreamPublisher(pub),
		EventStreamConsumer("Order.Created", handler),
	)
	require.NoError(t, err)
	defer conn.Close()

	err = pub.Publish(context.Background(), "Order.Created", testMessage{Name: "test", Value: 1})
	require.NoError(t, err)

	assert.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return received
	}, 5*time.Second, 50*time.Millisecond)

	tp.ForceFlush(context.Background())

	spans := exporter.GetSpans()
	require.GreaterOrEqual(t, len(spans), 2, "expected at least 2 spans (publish + consume)")

	// Verify publish span
	var publishSpan, consumeSpan *tracetest.SpanStub
	for i := range spans {
		for _, attr := range spans[i].Attributes {
			if attr.Key == "messaging.operation" {
				switch attr.Value.AsString() {
				case "publish":
					publishSpan = &spans[i]
				case "receive":
					consumeSpan = &spans[i]
				}
			}
		}
	}

	require.NotNil(t, publishSpan, "expected a publish span")
	require.NotNil(t, consumeSpan, "expected a consume span")

	// Verify publish span attributes
	assertAttr(t, publishSpan.Attributes, "messaging.system", "nats")
	assertAttr(t, publishSpan.Attributes, "messaging.operation", "publish")

	// Verify consume span attributes
	assertAttr(t, consumeSpan.Attributes, "messaging.system", "nats")
	assertAttr(t, consumeSpan.Attributes, "messaging.operation", "receive")
}

func TestCustomSpanNameFn(t *testing.T) {
	s := startTestServer(t)
	url := serverURL(s)

	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))

	var (
		mu   sync.Mutex
		done bool
	)

	handler := func(ctx context.Context, event spec.ConsumableEvent[testMessage]) error {
		mu.Lock()
		defer mu.Unlock()
		done = true
		return nil
	}

	pub := NewPublisher()
	conn, err := NewConnection("span-svc", url)
	require.NoError(t, err)

	err = conn.Start(context.Background(),
		WithTracing(tp),
		WithSpanNameFn(func(info spec.DeliveryInfo) string {
			return "custom:" + info.Key
		}),
		WithPublishSpanNameFn(func(stream, routingKey string) string {
			return "pub:" + routingKey
		}),
		EventStreamPublisher(pub),
		EventStreamConsumer("Order.Created", handler),
	)
	require.NoError(t, err)
	defer conn.Close()

	err = pub.Publish(context.Background(), "Order.Created", testMessage{Name: "test", Value: 1})
	require.NoError(t, err)

	assert.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return done
	}, 5*time.Second, 50*time.Millisecond)

	tp.ForceFlush(context.Background())

	spans := exporter.GetSpans()
	var foundPublish, foundConsume bool
	for _, span := range spans {
		if span.Name == "pub:Order.Created" {
			foundPublish = true
		}
		if span.Name == "custom:Order.Created" {
			foundConsume = true
		}
	}
	assert.True(t, foundPublish, "expected custom publish span name")
	assert.True(t, foundConsume, "expected custom consume span name")
}

func assertAttr(t *testing.T, attrs []attribute.KeyValue, key, expectedValue string) {
	t.Helper()
	for _, attr := range attrs {
		if string(attr.Key) == key {
			assert.Equal(t, expectedValue, attr.Value.AsString(), "attribute %s", key)
			return
		}
	}
	t.Errorf("attribute %s not found", key)
}
