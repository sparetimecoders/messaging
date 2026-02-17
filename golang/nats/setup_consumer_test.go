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
	"reflect"
	"sync"
	"testing"
	"time"

	natsgo "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/sparetimecoders/gomessaging/spec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServiceRequestConsumer(t *testing.T) {
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

	conn, err := NewConnection("email-svc", url)
	require.NoError(t, err)

	err = conn.Start(context.Background(),
		ServiceRequestConsumer("email.send", handler),
	)
	require.NoError(t, err)
	defer conn.Close()

	// Send a request via NATS Core
	nc, err := natsgo.Connect(url)
	require.NoError(t, err)
	defer nc.Close()

	payload, _ := json.Marshal(testMessage{Name: "send-email", Value: 42})
	err = nc.Publish("email-svc.request.email.send", payload)
	require.NoError(t, err)

	assert.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(received) == 1
	}, 5*time.Second, 50*time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, "send-email", received[0].Name)
}

func TestRequestResponseHandler(t *testing.T) {
	s := startTestServer(t)
	url := serverURL(s)

	type emailResponse struct {
		Status string `json:"status"`
	}

	handler := func(ctx context.Context, event spec.ConsumableEvent[testMessage]) (emailResponse, error) {
		return emailResponse{Status: "sent"}, nil
	}

	conn, err := NewConnection("email-svc", url)
	require.NoError(t, err)

	err = conn.Start(context.Background(),
		RequestResponseHandler("email.send", handler),
	)
	require.NoError(t, err)
	defer conn.Close()

	// Send a request and wait for reply
	nc, err := natsgo.Connect(url)
	require.NoError(t, err)
	defer nc.Close()

	payload, _ := json.Marshal(testMessage{Name: "send-email", Value: 1})
	msg := &natsgo.Msg{
		Subject: "email-svc.request.email.send",
		Data:    payload,
	}
	reply, err := nc.RequestMsg(msg, 5*time.Second)
	require.NoError(t, err)

	var resp emailResponse
	err = json.Unmarshal(reply.Data, &resp)
	require.NoError(t, err)
	assert.Equal(t, "sent", resp.Status)
}

func TestTransientStreamConsumer(t *testing.T) {
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
	conn, err := NewConnection("transient-svc", url)
	require.NoError(t, err)

	err = conn.Start(context.Background(),
		EventStreamPublisher(pub),
		TransientEventStreamConsumer("Order.Created", handler),
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

func TestRequestResponseHandler_Error(t *testing.T) {
	s := startTestServer(t)
	url := serverURL(s)

	type emailResponse struct {
		Status string `json:"status"`
	}

	handler := func(ctx context.Context, event spec.ConsumableEvent[testMessage]) (emailResponse, error) {
		return emailResponse{}, fmt.Errorf("handler error")
	}

	conn, err := NewConnection("email-svc", url)
	require.NoError(t, err)

	err = conn.Start(context.Background(),
		RequestResponseHandler("email.send", handler),
	)
	require.NoError(t, err)
	defer conn.Close()

	nc, err := natsgo.Connect(url)
	require.NoError(t, err)
	defer nc.Close()

	payload, _ := json.Marshal(testMessage{Name: "send-email", Value: 1})
	msg := &natsgo.Msg{
		Subject: "email-svc.request.email.send",
		Data:    payload,
	}
	reply, err := nc.RequestMsg(msg, 5*time.Second)
	require.NoError(t, err)

	// Error response should contain error message
	assert.Contains(t, string(reply.Data), "handler error")
}

func TestRequestResponseHandler_InvalidJSON(t *testing.T) {
	s := startTestServer(t)
	url := serverURL(s)

	type emailResponse struct {
		Status string `json:"status"`
	}

	handler := func(ctx context.Context, event spec.ConsumableEvent[testMessage]) (emailResponse, error) {
		return emailResponse{Status: "sent"}, nil
	}

	conn, err := NewConnection("email-svc", url)
	require.NoError(t, err)

	err = conn.Start(context.Background(),
		RequestResponseHandler("email.send", handler),
	)
	require.NoError(t, err)
	defer conn.Close()

	nc, err := natsgo.Connect(url)
	require.NoError(t, err)
	defer nc.Close()

	// Send invalid JSON
	msg := &natsgo.Msg{
		Subject: "email-svc.request.email.send",
		Data:    []byte("not json"),
	}
	reply, err := nc.RequestMsg(msg, 5*time.Second)
	require.NoError(t, err)

	// Should get error response about parse failure
	assert.Contains(t, string(reply.Data), "error")
}

func TestServiceRequestConsumer_HandlerError(t *testing.T) {
	s := startTestServer(t)
	url := serverURL(s)

	errorCh := make(chan spec.ErrorNotification, 10)

	handler := func(ctx context.Context, event spec.ConsumableEvent[testMessage]) error {
		return fmt.Errorf("processing failed")
	}

	conn, err := NewConnection("err-core-svc", url)
	require.NoError(t, err)

	err = conn.Start(context.Background(),
		WithErrorChannel(errorCh),
		ServiceRequestConsumer("email.send", handler),
	)
	require.NoError(t, err)
	defer conn.Close()

	nc, err := natsgo.Connect(url)
	require.NoError(t, err)
	defer nc.Close()

	payload, _ := json.Marshal(testMessage{Name: "test", Value: 1})
	// Use request to get the error response back
	msg := &natsgo.Msg{
		Subject: "err-core-svc.request.email.send",
		Data:    payload,
	}
	reply, err := nc.RequestMsg(msg, 5*time.Second)
	require.NoError(t, err)
	assert.Contains(t, string(reply.Data), "processing failed")

	// Verify error notification was sent
	select {
	case errNotif := <-errorCh:
		assert.Equal(t, spec.NotificationSourceConsumer, errNotif.Source)
		assert.Contains(t, errNotif.Error.Error(), "processing failed")
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for error notification")
	}
}

func TestTypeMappingHandler_NilMapper(t *testing.T) {
	handler := TypeMappingHandler(func(ctx context.Context, event spec.ConsumableEvent[any]) error {
		return nil
	}, nil)

	err := handler(context.Background(), spec.ConsumableEvent[any]{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "TypeMapper is nil")
}

func TestTypeMappingHandler_UnknownRoutingKey(t *testing.T) {
	mapper := func(routingKey string) (reflect.Type, bool) {
		return nil, false
	}

	handler := TypeMappingHandler(func(ctx context.Context, event spec.ConsumableEvent[any]) error {
		return nil
	}, mapper)

	evt := spec.ConsumableEvent[any]{
		DeliveryInfo: spec.DeliveryInfo{Key: "Unknown.Event"},
	}
	err := handler(context.Background(), evt)
	assert.ErrorIs(t, err, ErrNoMessageTypeForRouteKey)
}

func TestConsumerWithMaxDeliver(t *testing.T) {
	s := startTestServer(t)
	url := serverURL(s)

	var (
		mu        sync.Mutex
		delivered int
	)

	handler := func(ctx context.Context, event spec.ConsumableEvent[testMessage]) error {
		mu.Lock()
		defer mu.Unlock()
		delivered++
		return fmt.Errorf("always fail")
	}

	pub := NewPublisher()
	conn, err := NewConnection("maxdeliver-svc", url)
	require.NoError(t, err)

	err = conn.Start(context.Background(),
		EventStreamPublisher(pub),
		EventStreamConsumer("Order.Created", handler, WithMaxDeliver(3)),
	)
	require.NoError(t, err)
	defer conn.Close()

	err = pub.Publish(context.Background(), "Order.Created", testMessage{Name: "test", Value: 1})
	require.NoError(t, err)

	// Wait for deliveries to stop — should be exactly 3
	assert.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return delivered >= 3
	}, 5*time.Second, 50*time.Millisecond)

	// Give a small window to ensure no more deliveries happen
	time.Sleep(500 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 3, delivered, "message should be delivered exactly MaxDeliver times")
}

func TestConsumerWithBackOff(t *testing.T) {
	s := startTestServer(t)
	url := serverURL(s)

	handler := func(ctx context.Context, event spec.ConsumableEvent[testMessage]) error {
		return nil
	}

	pub := NewPublisher()
	conn, err := NewConnection("backoff-svc", url)
	require.NoError(t, err)

	err = conn.Start(context.Background(),
		EventStreamPublisher(pub),
		EventStreamConsumer("Order.Created", handler,
			WithMaxDeliver(5),
			WithBackOff(200*time.Millisecond, 500*time.Millisecond),
		),
	)
	require.NoError(t, err)
	defer conn.Close()

	// Verify the consumer config was applied by reading it back from JetStream
	nc, err := natsgo.Connect(url)
	require.NoError(t, err)
	defer nc.Close()

	js, err := jetstream.New(nc)
	require.NoError(t, err)

	cons, err := js.Consumer(context.Background(), "events", consumerName("backoff-svc"))
	require.NoError(t, err)

	info := cons.CachedInfo()
	assert.Equal(t, 5, info.Config.MaxDeliver)
	assert.Equal(t, []time.Duration{200 * time.Millisecond, 500 * time.Millisecond}, info.Config.BackOff)
}

func TestConsumerDefaultsMaxDeliver(t *testing.T) {
	s := startTestServer(t)
	url := serverURL(s)

	var (
		mu        sync.Mutex
		delivered int
	)

	handler := func(ctx context.Context, event spec.ConsumableEvent[testMessage]) error {
		mu.Lock()
		defer mu.Unlock()
		delivered++
		return fmt.Errorf("always fail")
	}

	pub := NewPublisher()
	conn, err := NewConnection("defaults-svc", url)
	require.NoError(t, err)

	err = conn.Start(context.Background(),
		WithConsumerDefaults(ConsumerDefaults{MaxDeliver: 2}),
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
		return delivered >= 2
	}, 5*time.Second, 50*time.Millisecond)

	time.Sleep(500 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 2, delivered, "connection-level MaxDeliver should limit deliveries")
}

func TestTypeMappingHandler(t *testing.T) {
	s := startTestServer(t)
	url := serverURL(s)

	type orderCreated struct {
		OrderID string `json:"orderId"`
	}

	var (
		mu       sync.Mutex
		received any
	)

	mapper := func(routingKey string) (reflect.Type, bool) {
		switch routingKey {
		case "Order.Created":
			return reflect.TypeOf((*orderCreated)(nil)), true
		}
		return nil, false
	}

	baseHandler := func(ctx context.Context, event spec.ConsumableEvent[any]) error {
		mu.Lock()
		defer mu.Unlock()
		received = event.Payload
		return nil
	}

	handler := TypeMappingHandler(baseHandler, mapper)

	pub := NewPublisher()
	conn, err := NewConnection("mapping-svc", url)
	require.NoError(t, err)

	err = conn.Start(context.Background(),
		EventStreamPublisher(pub),
		EventStreamConsumer("Order.#", handler),
	)
	require.NoError(t, err)
	defer conn.Close()

	err = pub.Publish(context.Background(), "Order.Created", orderCreated{OrderID: "ord-123"})
	require.NoError(t, err)

	assert.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return received != nil
	}, 5*time.Second, 50*time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	order, ok := received.(*orderCreated)
	require.True(t, ok)
	assert.Equal(t, "ord-123", order.OrderID)
}
