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
	"testing"
	"time"

	natsgo "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testMessage struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
}

func TestEventStreamPublisher(t *testing.T) {
	s := startTestServer(t)
	url := serverURL(s)

	pub := NewPublisher()
	conn, err := NewConnection("test-service", url)
	require.NoError(t, err)

	err = conn.Start(context.Background(), EventStreamPublisher(pub))
	require.NoError(t, err)
	defer conn.Close()

	// Subscribe to verify message arrives
	nc, err := natsgo.Connect(url)
	require.NoError(t, err)
	defer nc.Close()

	js, err := jetstream.New(nc)
	require.NoError(t, err)

	// Create consumer to read from events stream
	cons, err := js.CreateOrUpdateConsumer(context.Background(), "events", jetstream.ConsumerConfig{
		FilterSubject: "events.>",
	})
	require.NoError(t, err)

	// Publish a message
	err = pub.Publish(context.Background(), "Order.Created", testMessage{Name: "test-order", Value: 42})
	require.NoError(t, err)

	// Verify message
	msg, err := cons.Next(jetstream.FetchMaxWait(2 * time.Second))
	require.NoError(t, err)
	assert.Equal(t, "events.Order.Created", msg.Subject())
	assert.Contains(t, string(msg.Data()), "test-order")

	// Verify CloudEvents headers
	headers := msg.Headers()
	assert.Equal(t, "1.0", headers.Get("ce-specversion"))
	assert.Equal(t, "Order.Created", headers.Get("ce-type"))
	assert.Equal(t, "test-service", headers.Get("ce-source"))
	assert.NotEmpty(t, headers.Get("ce-id"))
	assert.NotEmpty(t, headers.Get("ce-time"))
	assert.Equal(t, "application/json", headers.Get("ce-datacontenttype"))
}

func TestStreamPublisher(t *testing.T) {
	s := startTestServer(t)
	url := serverURL(s)

	pub := NewPublisher()
	conn, err := NewConnection("test-service", url)
	require.NoError(t, err)

	err = conn.Start(context.Background(), StreamPublisher("audit", pub))
	require.NoError(t, err)
	defer conn.Close()

	nc, err := natsgo.Connect(url)
	require.NoError(t, err)
	defer nc.Close()

	js, err := jetstream.New(nc)
	require.NoError(t, err)

	cons, err := js.CreateOrUpdateConsumer(context.Background(), "audit", jetstream.ConsumerConfig{
		FilterSubject: "audit.>",
	})
	require.NoError(t, err)

	err = pub.Publish(context.Background(), "Audit.Entry", testMessage{Name: "audit-entry", Value: 1})
	require.NoError(t, err)

	msg, err := cons.Next(jetstream.FetchMaxWait(2 * time.Second))
	require.NoError(t, err)
	assert.Equal(t, "audit.Audit.Entry", msg.Subject())
	assert.Contains(t, string(msg.Data()), "audit-entry")
}

func TestPublisherWithCustomHeaders(t *testing.T) {
	s := startTestServer(t)
	url := serverURL(s)

	pub := NewPublisher()
	conn, err := NewConnection("test-service", url)
	require.NoError(t, err)

	err = conn.Start(context.Background(), EventStreamPublisher(pub))
	require.NoError(t, err)
	defer conn.Close()

	nc, err := natsgo.Connect(url)
	require.NoError(t, err)
	defer nc.Close()

	js, err := jetstream.New(nc)
	require.NoError(t, err)

	cons, err := js.CreateOrUpdateConsumer(context.Background(), "events", jetstream.ConsumerConfig{
		FilterSubject: "events.>",
	})
	require.NoError(t, err)

	err = pub.Publish(context.Background(), "Order.Created",
		testMessage{Name: "test", Value: 1},
		Header{Key: "x-custom", Value: "custom-value"},
	)
	require.NoError(t, err)

	msg, err := cons.Next(jetstream.FetchMaxWait(2 * time.Second))
	require.NoError(t, err)
	assert.Equal(t, "custom-value", msg.Headers().Get("x-custom"))
}

func TestPublisherReservedHeader(t *testing.T) {
	s := startTestServer(t)
	url := serverURL(s)

	pub := NewPublisher()
	conn, err := NewConnection("test-service", url)
	require.NoError(t, err)

	err = conn.Start(context.Background(), EventStreamPublisher(pub))
	require.NoError(t, err)
	defer conn.Close()

	err = pub.Publish(context.Background(), "Order.Created",
		testMessage{},
		Header{Key: headerService, Value: "hacked"},
	)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "reserved key")
}

func TestPublisherEmptyHeaderKey(t *testing.T) {
	s := startTestServer(t)
	url := serverURL(s)

	pub := NewPublisher()
	conn, err := NewConnection("test-service", url)
	require.NoError(t, err)

	err = conn.Start(context.Background(), EventStreamPublisher(pub))
	require.NoError(t, err)
	defer conn.Close()

	err = pub.Publish(context.Background(), "Order.Created",
		testMessage{},
		Header{Key: "", Value: "val"},
	)
	assert.ErrorIs(t, err, ErrEmptyHeaderKey)
}

func TestPublisherMarshalError(t *testing.T) {
	s := startTestServer(t)
	url := serverURL(s)

	pub := NewPublisher()
	conn, err := NewConnection("test-service", url)
	require.NoError(t, err)

	err = conn.Start(context.Background(), EventStreamPublisher(pub))
	require.NoError(t, err)
	defer conn.Close()

	// channels are not JSON-marshalable
	err = pub.Publish(context.Background(), "Order.Created", make(chan int))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "chan")
}

func TestPublisherCEHeaderDefaults(t *testing.T) {
	s := startTestServer(t)
	url := serverURL(s)

	pub := NewPublisher()
	conn, err := NewConnection("ce-svc", url)
	require.NoError(t, err)

	err = conn.Start(context.Background(), EventStreamPublisher(pub))
	require.NoError(t, err)
	defer conn.Close()

	nc, err := natsgo.Connect(url)
	require.NoError(t, err)
	defer nc.Close()

	js, err := jetstream.New(nc)
	require.NoError(t, err)

	cons, err := js.CreateOrUpdateConsumer(context.Background(), "events", jetstream.ConsumerConfig{
		FilterSubject: "events.>",
	})
	require.NoError(t, err)

	// Publish with CE header overrides — they should NOT be replaced by defaults
	err = pub.Publish(context.Background(), "Order.Created",
		testMessage{Name: "test", Value: 1},
		Header{Key: "ce-source", Value: "override-source"},
		Header{Key: "ce-type", Value: "override-type"},
	)
	require.NoError(t, err)

	msg, err := cons.Next(jetstream.FetchMaxWait(2 * time.Second))
	require.NoError(t, err)

	headers := msg.Headers()
	assert.Equal(t, "override-source", headers.Get("ce-source"))
	assert.Equal(t, "override-type", headers.Get("ce-type"))
	// ce-id is always set (not overridable via setDefault)
	assert.NotEmpty(t, headers.Get("ce-id"))
}

func TestPublisherWithPropagator(t *testing.T) {
	s := startTestServer(t)
	url := serverURL(s)

	pub := NewPublisher()
	conn, err := NewConnection("prop-svc", url)
	require.NoError(t, err)

	err = conn.Start(context.Background(),
		WithPropagator(nil), // nil falls back to global propagator
		EventStreamPublisher(pub),
	)
	require.NoError(t, err)
	defer conn.Close()

	err = pub.Publish(context.Background(), "Order.Created", testMessage{Name: "test", Value: 1})
	assert.NoError(t, err)
}

func TestPublisherTopology(t *testing.T) {
	s := startTestServer(t)
	url := serverURL(s)

	pub := NewPublisher()
	conn, err := NewConnection("test-service", url)
	require.NoError(t, err)

	err = conn.Start(context.Background(), EventStreamPublisher(pub))
	require.NoError(t, err)
	defer conn.Close()

	topo := conn.Topology()
	require.Len(t, topo.Endpoints, 1)
	assert.Equal(t, "publish", string(topo.Endpoints[0].Direction))
	assert.Equal(t, "event-stream", string(topo.Endpoints[0].Pattern))
	assert.Equal(t, "events", topo.Endpoints[0].ExchangeName)
}
