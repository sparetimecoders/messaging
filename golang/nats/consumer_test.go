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
	"sync"
	"testing"
	"time"

	"github.com/sparetimecoders/gomessaging/spec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventStreamConsumer(t *testing.T) {
	s := startTestServer(t)
	url := serverURL(s)

	var (
		mu       sync.Mutex
		received []testMessage
	)

	handler := func(ctx context.Context, event spec.ConsumableEvent[testMessage]) error {
		mu.Lock()
		defer mu.Unlock()
		received = append(received, event.Payload)
		return nil
	}

	pub := NewPublisher()
	conn, err := NewConnection("consumer-svc", url)
	require.NoError(t, err)

	err = conn.Start(context.Background(),
		EventStreamPublisher(pub),
		EventStreamConsumer("Order.Created", handler),
	)
	require.NoError(t, err)
	defer conn.Close()

	// Publish a message
	err = pub.Publish(context.Background(), "Order.Created", testMessage{Name: "order-1", Value: 100})
	require.NoError(t, err)

	// Wait for message to be consumed
	assert.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(received) == 1
	}, 5*time.Second, 50*time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, "order-1", received[0].Name)
	assert.Equal(t, 100, received[0].Value)
}

func TestStreamConsumer(t *testing.T) {
	s := startTestServer(t)
	url := serverURL(s)

	var (
		mu       sync.Mutex
		received []testMessage
	)

	handler := func(ctx context.Context, event spec.ConsumableEvent[testMessage]) error {
		mu.Lock()
		defer mu.Unlock()
		received = append(received, event.Payload)
		return nil
	}

	pub := NewPublisher()
	conn, err := NewConnection("analytics-svc", url)
	require.NoError(t, err)

	err = conn.Start(context.Background(),
		StreamPublisher("audit", pub),
		StreamConsumer("audit", "Audit.Entry", handler),
	)
	require.NoError(t, err)
	defer conn.Close()

	err = pub.Publish(context.Background(), "Audit.Entry", testMessage{Name: "audit-1", Value: 1})
	require.NoError(t, err)

	assert.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(received) == 1
	}, 5*time.Second, 50*time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, "audit-1", received[0].Name)
}

func TestConsumerMetadata(t *testing.T) {
	s := startTestServer(t)
	url := serverURL(s)

	var (
		mu   sync.Mutex
		meta spec.Metadata
		info spec.DeliveryInfo
	)

	handler := func(ctx context.Context, event spec.ConsumableEvent[testMessage]) error {
		mu.Lock()
		defer mu.Unlock()
		meta = event.Metadata
		info = event.DeliveryInfo
		return nil
	}

	pub := NewPublisher()
	conn, err := NewConnection("meta-svc", url)
	require.NoError(t, err)

	err = conn.Start(context.Background(),
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
		return meta.ID != ""
	}, 5*time.Second, 50*time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, "1.0", meta.SpecVersion)
	assert.Equal(t, "Order.Created", meta.Type)
	assert.Equal(t, "meta-svc", meta.Source)
	assert.NotEmpty(t, meta.ID)
	assert.Equal(t, "Order.Created", info.Key)
	assert.Equal(t, "events", info.Source)
}

func TestConsumerNotifications(t *testing.T) {
	s := startTestServer(t)
	url := serverURL(s)

	notifCh := make(chan spec.Notification, 10)

	handler := func(ctx context.Context, event spec.ConsumableEvent[testMessage]) error {
		return nil
	}

	pub := NewPublisher()
	conn, err := NewConnection("notif-svc", url)
	require.NoError(t, err)

	err = conn.Start(context.Background(),
		WithNotificationChannel(notifCh),
		EventStreamPublisher(pub),
		EventStreamConsumer("Order.Created", handler),
	)
	require.NoError(t, err)
	defer conn.Close()

	err = pub.Publish(context.Background(), "Order.Created", testMessage{Name: "test", Value: 1})
	require.NoError(t, err)

	select {
	case notif := <-notifCh:
		assert.Equal(t, spec.NotificationSourceConsumer, notif.Source)
		assert.Equal(t, "Order.Created", notif.DeliveryInfo.Key)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for notification")
	}
}

func TestConsumerErrorNotifications(t *testing.T) {
	s := startTestServer(t)
	url := serverURL(s)

	errorCh := make(chan spec.ErrorNotification, 10)

	handler := func(ctx context.Context, event spec.ConsumableEvent[testMessage]) error {
		return spec.ErrParseJSON
	}

	pub := NewPublisher()
	conn, err := NewConnection("err-svc", url)
	require.NoError(t, err)

	err = conn.Start(context.Background(),
		WithErrorChannel(errorCh),
		EventStreamPublisher(pub),
		EventStreamConsumer("Order.Created", handler),
	)
	require.NoError(t, err)
	defer conn.Close()

	err = pub.Publish(context.Background(), "Order.Created", testMessage{Name: "test", Value: 1})
	require.NoError(t, err)

	select {
	case errNotif := <-errorCh:
		assert.Equal(t, spec.NotificationSourceConsumer, errNotif.Source)
		assert.ErrorIs(t, errNotif.Error, spec.ErrParseJSON)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for error notification")
	}
}

func TestConsumerWithSuffix(t *testing.T) {
	s := startTestServer(t)
	url := serverURL(s)

	var (
		mu       sync.Mutex
		received int
	)

	handler := func(ctx context.Context, event spec.ConsumableEvent[testMessage]) error {
		mu.Lock()
		defer mu.Unlock()
		received++
		return nil
	}

	pub := NewPublisher()
	conn, err := NewConnection("suffix-svc", url)
	require.NoError(t, err)

	err = conn.Start(context.Background(),
		EventStreamPublisher(pub),
		EventStreamConsumer("Order.Created", handler, AddConsumerNameSuffix("v2")),
	)
	require.NoError(t, err)
	defer conn.Close()

	err = pub.Publish(context.Background(), "Order.Created", testMessage{Name: "test", Value: 1})
	require.NoError(t, err)

	assert.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return received == 1
	}, 5*time.Second, 50*time.Millisecond)
}

func TestConsumerHandlerError_GenericNak(t *testing.T) {
	s := startTestServer(t)
	url := serverURL(s)

	errorCh := make(chan spec.ErrorNotification, 10)

	handler := func(ctx context.Context, event spec.ConsumableEvent[testMessage]) error {
		return fmt.Errorf("transient failure")
	}

	pub := NewPublisher()
	conn, err := NewConnection("nak-svc", url)
	require.NoError(t, err)

	err = conn.Start(context.Background(),
		WithErrorChannel(errorCh),
		EventStreamPublisher(pub),
		EventStreamConsumer("Order.Created", handler),
	)
	require.NoError(t, err)
	defer conn.Close()

	err = pub.Publish(context.Background(), "Order.Created", testMessage{Name: "test", Value: 1})
	require.NoError(t, err)

	// Should receive error notification for generic handler error
	select {
	case errNotif := <-errorCh:
		assert.Equal(t, spec.NotificationSourceConsumer, errNotif.Source)
		assert.Contains(t, errNotif.Error.Error(), "transient failure")
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for error notification")
	}
}

func TestConsumerHandlerError_NoMessageType(t *testing.T) {
	s := startTestServer(t)
	url := serverURL(s)

	errorCh := make(chan spec.ErrorNotification, 10)

	handler := func(ctx context.Context, event spec.ConsumableEvent[testMessage]) error {
		return ErrNoMessageTypeForRouteKey
	}

	pub := NewPublisher()
	conn, err := NewConnection("nomap-svc", url)
	require.NoError(t, err)

	err = conn.Start(context.Background(),
		WithErrorChannel(errorCh),
		EventStreamPublisher(pub),
		EventStreamConsumer("Order.Created", handler),
	)
	require.NoError(t, err)
	defer conn.Close()

	err = pub.Publish(context.Background(), "Order.Created", testMessage{Name: "test", Value: 1})
	require.NoError(t, err)

	select {
	case errNotif := <-errorCh:
		assert.ErrorIs(t, errNotif.Error, ErrNoMessageTypeForRouteKey)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for error notification")
	}
}

func TestAddConsumerNameSuffixEmpty(t *testing.T) {
	cfg := &consumerConfig{}
	err := AddConsumerNameSuffix("")(cfg)
	assert.ErrorIs(t, err, ErrEmptySuffix)
}

func TestWithMaxDeliver(t *testing.T) {
	cfg := &consumerConfig{}
	err := WithMaxDeliver(5)(cfg)
	require.NoError(t, err)
	assert.Equal(t, 5, cfg.maxDeliver)
}

func TestWithMaxDeliver_Zero(t *testing.T) {
	cfg := &consumerConfig{}
	err := WithMaxDeliver(0)(cfg)
	require.NoError(t, err)
	assert.Equal(t, 0, cfg.maxDeliver)
}

func TestWithMaxDeliver_Negative(t *testing.T) {
	cfg := &consumerConfig{}
	err := WithMaxDeliver(-1)(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "MaxDeliver must be >= 0")
}

func TestWithBackOff(t *testing.T) {
	cfg := &consumerConfig{}
	err := WithBackOff(100*time.Millisecond, 500*time.Millisecond, 2*time.Second)(cfg)
	require.NoError(t, err)
	assert.Equal(t, []time.Duration{100 * time.Millisecond, 500 * time.Millisecond, 2 * time.Second}, cfg.backOff)
}
