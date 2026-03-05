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
	"fmt"
	"log/slog"
	"reflect"
	"runtime"

	"github.com/sparetimecoders/messaging/specification/spec"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// Setup is a setup function that takes a Connection and use it to set up AMQP
// An example is to create exchanges and queues
type Setup func(conn *Connection) error

// WithLogger configures structured logging for the connection.
// If not set, slog.Default() is used.
func WithLogger(logger *slog.Logger) Setup {
	return func(conn *Connection) error {
		conn.logger = logger.With("service", conn.serviceName)
		return nil
	}
}

// WithTracing configures a TracerProvider for the connection.
// If not set, the global TracerProvider (otel.GetTracerProvider()) is used.
func WithTracing(tp trace.TracerProvider) Setup {
	return func(conn *Connection) error {
		conn.tracerProvider = tp
		return nil
	}
}

// WithPrefetchLimit configures the number of messages to prefetch from the server.
// To get round-robin behavior between queueConsumers consuming from the same queue on
// different connections, set the prefetch count to 1, and the next available
// message on the server will be delivered to the next available consumer.
// If your consumer work time is reasonably consistent and not much greater
// than two times your network round trip time, you will see significant
// throughput improvements starting with a prefetch count of 2 or slightly
// greater, as described by benchmarks on RabbitMQ.
//
// http://www.rabbitmq.com/blog/2012/04/25/rabbitmq-performance-measurements-part-2/
func WithPrefetchLimit(limit int) Setup {
	return func(conn *Connection) error {
		conn.prefetchLimit = limit
		return nil
	}
}

// WithNotificationChannel specifies a go channel to receive messages
// such as connection event published, consumed, etc.
func WithNotificationChannel(notificationCh chan<- spec.Notification) Setup {
	return func(conn *Connection) error {
		conn.notificationCh = notificationCh
		return nil
	}
}

// WithErrorChannel specifies a go channel to receive messages such as event failed
func WithErrorChannel(errorCh chan<- spec.ErrorNotification) Setup {
	return func(conn *Connection) error {
		conn.errorCh = errorCh
		return nil
	}
}

var spanNameFn = func(info spec.DeliveryInfo) string {
	return fmt.Sprintf("%s#%s", trimExchangeFromQueue(info.Destination, info.Source), info.Key)
}

// WithPropagator configures a TextMapPropagator for the connection.
// If not set, the global propagator (otel.GetTextMapPropagator()) is used.
func WithPropagator(p propagation.TextMapPropagator) Setup {
	return func(conn *Connection) error {
		conn.propagator = p
		return nil
	}
}

// WithSpanNameFn specifies a function that will get called when a new consumer span is created.
// By default the spanNameFn will be used.
func WithSpanNameFn(f func(spec.DeliveryInfo) string) Setup {
	return func(conn *Connection) error {
		conn.spanNameFn = f
		return nil
	}
}

// WithPublishSpanNameFn specifies a function that will get called when a new publish span is created.
// The function receives the exchange name and routing key. By default "publish {routingKey}" is used.
func WithPublishSpanNameFn(f func(exchange, routingKey string) string) Setup {
	return func(conn *Connection) error {
		conn.publishSpanNameFn = f
		return nil
	}
}

// WithLegacySupport enables automatic metadata enrichment for messages
// from legacy (pre-CloudEvents) publishers. When enabled, incoming messages
// that lack CloudEvents headers will have their Metadata populated with
// synthetic values (generated UUID, current timestamp, routing key as type,
// exchange as source).
//
// Without this option, legacy messages arrive with zero-valued Metadata,
// which is the existing default behavior.
func WithLegacySupport() Setup {
	return func(conn *Connection) error {
		conn.legacySupport = true
		return nil
	}
}

// CloseListener receives close errors from both AMQP channels and the
// underlying connection. Use this to detect unexpected disconnects and
// implement fail-fast behavior (e.g. os.Exit).
func CloseListener(e chan error) Setup {
	return func(c *Connection) error {
		c.closeListener = e
		return nil
	}
}

// getSetupFuncName returns the name of the Setup function
func getSetupFuncName(f Setup) string {
	return runtime.FuncForPC(reflect.ValueOf(f).Pointer()).Name()
}
