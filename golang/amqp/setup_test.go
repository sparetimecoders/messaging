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
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/sparetimecoders/messaging/specification/spec"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

func Test_WithLogger(t *testing.T) {
	channel := NewMockAmqpChannel()
	conn := mockConnection(channel)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	err := WithLogger(logger)(conn)
	require.NoError(t, err)
	require.NotNil(t, conn.logger)
}

func Test_WithTracing(t *testing.T) {
	channel := NewMockAmqpChannel()
	conn := mockConnection(channel)
	tp := trace.NewNoopTracerProvider()
	err := WithTracing(tp)(conn)
	require.NoError(t, err)
	require.Equal(t, tp, conn.tracerProvider)
}

func Test_WithSpanNameFn(t *testing.T) {
	channel := NewMockAmqpChannel()
	conn := mockConnection(channel)
	fn := func(info spec.DeliveryInfo) string {
		return "custom-span"
	}
	err := WithSpanNameFn(fn)(conn)
	require.NoError(t, err)
	require.Equal(t, "custom-span", conn.spanNameFn(spec.DeliveryInfo{}))
}

func Test_WithPropagator(t *testing.T) {
	channel := NewMockAmqpChannel()
	conn := mockConnection(channel)
	prop := propagation.TraceContext{}
	err := WithPropagator(prop)(conn)
	require.NoError(t, err)
	require.Equal(t, prop, conn.propagator)
}

func Test_WithPublishSpanNameFn(t *testing.T) {
	channel := NewMockAmqpChannel()
	conn := mockConnection(channel)
	fn := func(exchange, routingKey string) string {
		return exchange + ":" + routingKey
	}
	err := WithPublishSpanNameFn(fn)(conn)
	require.NoError(t, err)
	require.Equal(t, "ex:key", conn.publishSpanNameFn("ex", "key"))
}

func Test_CloseListener(t *testing.T) {
	channel := NewMockAmqpChannel()
	conn := mockConnection(channel)
	ch := make(chan error, 1)
	err := CloseListener(ch)(conn)
	require.NoError(t, err)
	require.Equal(t, ch, conn.closeListener)
}

func Test_WithLegacySupport(t *testing.T) {
	channel := NewMockAmqpChannel()
	conn := mockConnection(channel)
	require.False(t, conn.legacySupport)

	err := WithLegacySupport()(conn)
	require.NoError(t, err)
	require.True(t, conn.legacySupport)
}

func Test_EventStreamPublisher_FailedToCreateExchange(t *testing.T) {
	channel := NewMockAmqpChannel()
	conn := mockConnection(channel)
	p := NewPublisher()

	e := errors.New("failed to create exchange")
	channel.ExchangeDeclarationError = &e
	err := EventStreamPublisher(p)(conn)
	require.Error(t, err)
	require.EqualError(t, err, "failed to declare exchange events.topic.exchange, failed to create exchange")
}

func Test_ServicePublisher_ExchangeDeclareFail(t *testing.T) {
	e := errors.New("failed")
	channel := NewMockAmqpChannel()
	channel.ExchangeDeclarationError = &e
	conn := mockConnection(channel)

	p := NewPublisher()

	err := ServicePublisher("svc", p)(conn)
	require.Error(t, err)
	require.EqualError(t, err, e.Error())
}

func Test_Start_WithPrefetchLimit_Resets_Qos(t *testing.T) {
	mockChannel := &MockAmqpChannel{
		qosFn: func(cc int) func(prefetchCount, prefetchSize int, global bool) error {
			return func(prefetchCount, prefetchSize int, global bool) error {
				defer func() {
					cc++
				}()
				if cc == 0 {
					require.Equal(t, 20, prefetchCount)
				} else {
					require.Equal(t, 1, prefetchCount)
				}
				return nil
			}
		}(0),
	}
	mockAmqpConnection := &MockAmqpConnection{
		channelFn: func() (amqpChannel, error) {
			return mockChannel, nil
		},
	}
	conn := &Connection{
		serviceName:    "test",
		connection:     mockAmqpConnection,
		queueConsumers: &queueConsumers{},
	}
	notifications := make(chan<- spec.Notification)
	errChannel := make(chan<- spec.ErrorNotification)
	err := conn.Start(context.Background(),
		WithPrefetchLimit(1),
		WithNotificationChannel(notifications),
		WithErrorChannel(errChannel),
	)
	require.NoError(t, err)
	require.Equal(t, notifications, conn.notificationCh)
}
