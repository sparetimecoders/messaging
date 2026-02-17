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
	"fmt"
	"log/slog"
	"time"

	"github.com/sparetimecoders/gomessaging/spec"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// StreamConfig configures retention limits for JetStream streams.
type StreamConfig struct {
	// MaxAge is the maximum age of messages in the stream.
	// Zero means unlimited (NATS default).
	MaxAge time.Duration

	// MaxBytes is the maximum total size of messages in the stream.
	// Zero means unlimited.
	MaxBytes int64

	// MaxMsgs is the maximum number of messages in the stream.
	// Zero means unlimited.
	MaxMsgs int64
}

// ConsumerDefaults configures default JetStream consumer behavior.
type ConsumerDefaults struct {
	// MaxDeliver is the maximum number of delivery attempts.
	// Zero means unlimited (NATS default).
	MaxDeliver int

	// BackOff specifies redelivery backoff durations.
	// The server waits BackOff[min(attempt, len-1)] before redelivering.
	BackOff []time.Duration
}

// WithConsumerDefaults sets default MaxDeliver and BackOff applied to all
// JetStream consumers. Per-consumer options (WithMaxDeliver, WithBackOff)
// take precedence.
func WithConsumerDefaults(cfg ConsumerDefaults) Setup {
	return func(conn *Connection) error {
		conn.consumerDefaults = cfg
		return nil
	}
}

// WithStreamDefaults sets default retention limits applied to all streams
// created by this connection. Per-stream overrides take precedence.
func WithStreamDefaults(cfg StreamConfig) Setup {
	return func(conn *Connection) error {
		conn.streamDefaults = cfg
		return nil
	}
}

// WithStreamConfig sets retention limits for a specific named stream,
// overriding any connection-level defaults.
func WithStreamConfig(stream string, cfg StreamConfig) Setup {
	return func(conn *Connection) error {
		if stream == "" {
			return fmt.Errorf("stream name must not be empty")
		}
		name := streamName(stream)
		if conn.streamConfigs == nil {
			conn.streamConfigs = make(map[string]StreamConfig)
		}
		conn.streamConfigs[name] = cfg
		return nil
	}
}

// Setup is a setup function that takes a Connection and configures it.
type Setup func(conn *Connection) error

// WithLogger configures structured logging for the connection.
func WithLogger(logger *slog.Logger) Setup {
	return func(conn *Connection) error {
		conn.logger = logger.With("service", conn.serviceName)
		return nil
	}
}

// WithTracing configures a TracerProvider for the connection.
func WithTracing(tp trace.TracerProvider) Setup {
	return func(conn *Connection) error {
		conn.tracerProvider = tp
		return nil
	}
}

// WithPropagator configures a TextMapPropagator for the connection.
func WithPropagator(p propagation.TextMapPropagator) Setup {
	return func(conn *Connection) error {
		conn.propagator = p
		return nil
	}
}

// WithSpanNameFn specifies a function called when a new consumer span is created.
func WithSpanNameFn(f func(spec.DeliveryInfo) string) Setup {
	return func(conn *Connection) error {
		conn.spanNameFn = f
		return nil
	}
}

// WithPublishSpanNameFn specifies a function called when a new publish span is created.
// The function receives the stream name and routing key.
func WithPublishSpanNameFn(f func(stream, routingKey string) string) Setup {
	return func(conn *Connection) error {
		conn.publishSpanNameFn = f
		return nil
	}
}

// WithNotificationChannel specifies a channel to receive success notifications.
func WithNotificationChannel(ch chan<- spec.Notification) Setup {
	return func(conn *Connection) error {
		conn.notificationCh = ch
		return nil
	}
}

// WithErrorChannel specifies a channel to receive error notifications.
func WithErrorChannel(ch chan<- spec.ErrorNotification) Setup {
	return func(conn *Connection) error {
		conn.errorCh = ch
		return nil
	}
}
