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

	"github.com/sparetimecoders/messaging/specification/spec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollectTopologyEventStreamPublisher(t *testing.T) {
	pub := NewPublisher()
	topo, err := CollectTopology("orders", EventStreamPublisher(pub))
	require.NoError(t, err)

	assert.Equal(t, "orders", topo.ServiceName)
	require.Len(t, topo.Endpoints, 1)
	assert.Equal(t, spec.DirectionPublish, topo.Endpoints[0].Direction)
	assert.Equal(t, spec.PatternEventStream, topo.Endpoints[0].Pattern)
	assert.Equal(t, "events", topo.Endpoints[0].ExchangeName)
	assert.Equal(t, spec.ExchangeTopic, topo.Endpoints[0].ExchangeKind)
}

func TestCollectTopologyStreamPublisher(t *testing.T) {
	pub := NewPublisher()
	topo, err := CollectTopology("analytics", StreamPublisher("audit", pub))
	require.NoError(t, err)

	require.Len(t, topo.Endpoints, 1)
	assert.Equal(t, spec.DirectionPublish, topo.Endpoints[0].Direction)
	assert.Equal(t, spec.PatternCustomStream, topo.Endpoints[0].Pattern)
	assert.Equal(t, "audit", topo.Endpoints[0].ExchangeName)
}

func TestCollectTopologyEventStreamConsumer(t *testing.T) {
	handler := func(_ context.Context, _ spec.ConsumableEvent[testMessage]) error { return nil }
	topo, err := CollectTopology("orders",
		EventStreamConsumer("Order.Created", handler),
	)
	require.NoError(t, err)

	require.Len(t, topo.Endpoints, 1)
	ep := topo.Endpoints[0]
	assert.Equal(t, spec.DirectionConsume, ep.Direction)
	assert.Equal(t, spec.PatternEventStream, ep.Pattern)
	assert.Equal(t, "events", ep.ExchangeName)
	assert.Equal(t, "orders", ep.QueueName)
	assert.Equal(t, "Order.Created", ep.RoutingKey)
	assert.False(t, ep.Ephemeral)
}

func TestCollectTopologyTransientConsumer(t *testing.T) {
	handler := func(_ context.Context, _ spec.ConsumableEvent[testMessage]) error { return nil }
	topo, err := CollectTopology("dashboard",
		TransientEventStreamConsumer("Order.Created", handler),
	)
	require.NoError(t, err)

	require.Len(t, topo.Endpoints, 1)
	ep := topo.Endpoints[0]
	assert.Equal(t, spec.DirectionConsume, ep.Direction)
	assert.True(t, ep.Ephemeral)
	assert.Empty(t, ep.QueueName) // ephemeral has no durable name
}

func TestCollectTopologyServiceRequest(t *testing.T) {
	handler := func(_ context.Context, _ spec.ConsumableEvent[testMessage]) error { return nil }
	topo, err := CollectTopology("email-svc",
		ServiceRequestConsumer("email.send", handler),
	)
	require.NoError(t, err)

	require.Len(t, topo.Endpoints, 1)
	ep := topo.Endpoints[0]
	assert.Equal(t, spec.DirectionConsume, ep.Direction)
	assert.Equal(t, spec.PatternServiceRequest, ep.Pattern)
	assert.Equal(t, "email-svc", ep.ExchangeName)
	assert.Equal(t, "email.send", ep.RoutingKey)
}

func TestCollectTopologyServicePublisher(t *testing.T) {
	pub := NewPublisher()
	topo, err := CollectTopology("web-app",
		ServicePublisher("email-svc", pub),
	)
	require.NoError(t, err)

	require.Len(t, topo.Endpoints, 1)
	ep := topo.Endpoints[0]
	assert.Equal(t, spec.DirectionPublish, ep.Direction)
	assert.Equal(t, spec.PatternServiceRequest, ep.Pattern)
	assert.Equal(t, "email-svc", ep.ExchangeName)
}

func TestCollectTopologyServiceResponse(t *testing.T) {
	handler := func(_ context.Context, _ spec.ConsumableEvent[testMessage]) error { return nil }
	topo, err := CollectTopology("web-app",
		ServiceResponseConsumer("email-svc", "email.send", handler),
	)
	require.NoError(t, err)

	require.Len(t, topo.Endpoints, 1)
	ep := topo.Endpoints[0]
	assert.Equal(t, spec.DirectionConsume, ep.Direction)
	assert.Equal(t, spec.PatternServiceResponse, ep.Pattern)
	assert.Equal(t, "email-svc", ep.ExchangeName)
	assert.Equal(t, "email.send", ep.RoutingKey)
}

func TestCollectTopologyMixed(t *testing.T) {
	pub := NewPublisher()
	handler := func(_ context.Context, _ spec.ConsumableEvent[testMessage]) error { return nil }

	topo, err := CollectTopology("orders",
		EventStreamPublisher(pub),
		EventStreamConsumer("User.Created", handler),
	)
	require.NoError(t, err)

	assert.Equal(t, "orders", topo.ServiceName)
	require.Len(t, topo.Endpoints, 2)
	assert.Equal(t, spec.DirectionPublish, topo.Endpoints[0].Direction)
	assert.Equal(t, spec.DirectionConsume, topo.Endpoints[1].Direction)
}
