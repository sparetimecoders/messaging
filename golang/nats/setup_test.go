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
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/sparetimecoders/gomessaging/spec"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/propagation"
)

func TestWithLogger(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	conn := newConnection("test-svc", "nats://localhost:4222")
	err := WithLogger(logger)(conn)
	assert.NoError(t, err)
	assert.NotNil(t, conn.logger)
}

func TestWithPropagator(t *testing.T) {
	prop := propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
	conn := newConnection("test-svc", "nats://localhost:4222")
	err := WithPropagator(prop)(conn)
	assert.NoError(t, err)
	assert.Equal(t, prop, conn.propagator)
}

func TestWithNotificationChannel(t *testing.T) {
	ch := make(chan spec.Notification, 1)
	conn := newConnection("test-svc", "nats://localhost:4222")
	err := WithNotificationChannel(ch)(conn)
	assert.NoError(t, err)
	assert.Equal(t, (chan<- spec.Notification)(ch), conn.notificationCh)
}

func TestWithErrorChannel(t *testing.T) {
	ch := make(chan spec.ErrorNotification, 1)
	conn := newConnection("test-svc", "nats://localhost:4222")
	err := WithErrorChannel(ch)(conn)
	assert.NoError(t, err)
	assert.Equal(t, (chan<- spec.ErrorNotification)(ch), conn.errorCh)
}

func TestWithSpanNameFn(t *testing.T) {
	fn := func(info spec.DeliveryInfo) string {
		return "custom:" + info.Key
	}
	conn := newConnection("test-svc", "nats://localhost:4222")
	err := WithSpanNameFn(fn)(conn)
	assert.NoError(t, err)
	assert.NotNil(t, conn.spanNameFn)
	result := conn.spanNameFn(spec.DeliveryInfo{Key: "test"})
	assert.Equal(t, "custom:test", result)
}

func TestWithPublishSpanNameFn(t *testing.T) {
	fn := func(stream, routingKey string) string {
		return stream + ":" + routingKey
	}
	conn := newConnection("test-svc", "nats://localhost:4222")
	err := WithPublishSpanNameFn(fn)(conn)
	assert.NoError(t, err)
	assert.NotNil(t, conn.publishSpanNameFn)
	result := conn.publishSpanNameFn("events", "Order.Created")
	assert.Equal(t, "events:Order.Created", result)
}

func TestWithStreamDefaults(t *testing.T) {
	conn := newConnection("test-svc", "nats://localhost:4222")
	err := WithStreamDefaults(StreamConfig{
		MaxAge:   7 * 24 * time.Hour,
		MaxBytes: 1 << 30, // 1 GiB
	})(conn)
	assert.NoError(t, err)
	assert.Equal(t, 7*24*time.Hour, conn.streamDefaults.MaxAge)
	assert.Equal(t, int64(1<<30), conn.streamDefaults.MaxBytes)
}

func TestWithStreamConfig(t *testing.T) {
	conn := newConnection("test-svc", "nats://localhost:4222")
	err := WithStreamConfig("audit", StreamConfig{
		MaxAge: 365 * 24 * time.Hour,
	})(conn)
	assert.NoError(t, err)
	assert.Len(t, conn.streamConfigs, 1)
	cfg := conn.streamConfigs[streamName("audit")]
	assert.Equal(t, 365*24*time.Hour, cfg.MaxAge)
}

func TestWithStreamConfig_OverridesDefaults(t *testing.T) {
	conn := newConnection("test-svc", "nats://localhost:4222")
	_ = WithStreamDefaults(StreamConfig{MaxAge: 7 * 24 * time.Hour})(conn)
	_ = WithStreamConfig("audit", StreamConfig{MaxAge: 365 * 24 * time.Hour})(conn)

	// Per-stream config should take precedence (verified via integration test)
	assert.Equal(t, 365*24*time.Hour, conn.streamConfigs[streamName("audit")].MaxAge)
}

func TestWithStreamConfig_EmptyStreamName(t *testing.T) {
	conn := newConnection("test-svc", "nats://localhost:4222")
	err := WithStreamConfig("", StreamConfig{MaxAge: time.Hour})(conn)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "stream name must not be empty")
}

func TestWithRequestTimeout(t *testing.T) {
	conn := newConnection("test-svc", "nats://localhost:4222")
	err := WithRequestTimeout(5 * time.Second)(conn)
	assert.NoError(t, err)
	assert.Equal(t, 5*time.Second, conn.requestTimeout)
}

func TestWithRequestTimeout_Negative(t *testing.T) {
	conn := newConnection("test-svc", "nats://localhost:4222")
	err := WithRequestTimeout(-1 * time.Second)(conn)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "request timeout must be positive")
}

func TestWithRequestTimeout_Zero(t *testing.T) {
	conn := newConnection("test-svc", "nats://localhost:4222")
	err := WithRequestTimeout(0)(conn)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "request timeout must be positive")
}

func TestWithConsumerDefaults(t *testing.T) {
	conn := newConnection("test-svc", "nats://localhost:4222")
	err := WithConsumerDefaults(ConsumerDefaults{
		MaxDeliver: 5,
		BackOff:    []time.Duration{100 * time.Millisecond, 500 * time.Millisecond},
	})(conn)
	assert.NoError(t, err)
	assert.Equal(t, 5, conn.consumerDefaults.MaxDeliver)
	assert.Equal(t, []time.Duration{100 * time.Millisecond, 500 * time.Millisecond}, conn.consumerDefaults.BackOff)
}
