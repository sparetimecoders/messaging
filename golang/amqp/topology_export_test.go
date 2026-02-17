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
	"fmt"
	"testing"

	"github.com/sparetimecoders/gomessaging/spec"
)

func TestCollectTopology_EventStreamPublisher(t *testing.T) {
	pub := NewPublisher()
	topo, err := CollectTopology("order-service",
		EventStreamPublisher(pub),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if topo.ServiceName != "order-service" {
		t.Errorf("expected service name order-service, got %q", topo.ServiceName)
	}
	if len(topo.Endpoints) != 1 {
		t.Fatalf("expected 1 endpoint, got %d", len(topo.Endpoints))
	}
	ep := topo.Endpoints[0]
	if ep.Direction != spec.DirectionPublish {
		t.Errorf("expected publish direction, got %q", ep.Direction)
	}
	if ep.ExchangeName != "events.topic.exchange" {
		t.Errorf("expected events.topic.exchange, got %q", ep.ExchangeName)
	}
	if ep.Pattern != spec.PatternEventStream {
		t.Errorf("expected event-stream pattern, got %q", ep.Pattern)
	}
}

func TestCollectTopology_EventStreamConsumer(t *testing.T) {
	topo, err := CollectTopology("notification-service",
		EventStreamConsumer("Order.Created", func(_ context.Context, _ spec.ConsumableEvent[any]) error {
			return nil
		}),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(topo.Endpoints) != 1 {
		t.Fatalf("expected 1 endpoint, got %d", len(topo.Endpoints))
	}
	ep := topo.Endpoints[0]
	if ep.Direction != spec.DirectionConsume {
		t.Errorf("expected consume direction, got %q", ep.Direction)
	}
	if ep.RoutingKey != "Order.Created" {
		t.Errorf("expected Order.Created routing key, got %q", ep.RoutingKey)
	}
	if ep.QueueName != "events.topic.exchange.queue.notification-service" {
		t.Errorf("expected queue name events.topic.exchange.queue.notification-service, got %q", ep.QueueName)
	}
}

func TestCollectTopology_RequestResponse(t *testing.T) {
	// RequestResponseHandler wraps ServiceRequestConsumer, which records the
	// consume endpoint and declares the response exchange. The response publish
	// endpoint is recorded separately by ServiceRequestConsumer's addEndpoint.
	topo, err := CollectTopology("email-service",
		RequestResponseHandler("email.send", func(_ context.Context, _ spec.ConsumableEvent[any]) (any, error) {
			return nil, nil
		}),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(topo.Endpoints) != 1 {
		t.Fatalf("expected 1 endpoint, got %d", len(topo.Endpoints))
	}
	ep := topo.Endpoints[0]
	if ep.Direction != spec.DirectionConsume {
		t.Errorf("expected consume direction, got %q", ep.Direction)
	}
	if ep.Pattern != spec.PatternServiceRequest {
		t.Errorf("expected service-request pattern, got %q", ep.Pattern)
	}
	if ep.RoutingKey != "email.send" {
		t.Errorf("expected email.send routing key, got %q", ep.RoutingKey)
	}
}

func TestCollectTopology_ServiceRequestAndResponse(t *testing.T) {
	// Full request-response pair as declared by both sides
	pub := NewPublisher()
	topo, err := CollectTopology("webapp-service",
		ServicePublisher("email-service", pub),
		ServiceResponseConsumer("email-service", "email.send",
			func(_ context.Context, _ spec.ConsumableEvent[any]) error { return nil }),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(topo.Endpoints) != 2 {
		t.Fatalf("expected 2 endpoints, got %d", len(topo.Endpoints))
	}
	var hasPublish, hasConsume bool
	for _, ep := range topo.Endpoints {
		if ep.Direction == spec.DirectionPublish && ep.Pattern == spec.PatternServiceRequest {
			hasPublish = true
		}
		if ep.Direction == spec.DirectionConsume && ep.Pattern == spec.PatternServiceResponse {
			hasConsume = true
		}
	}
	if !hasPublish {
		t.Error("expected service-request publish endpoint")
	}
	if !hasConsume {
		t.Error("expected service-response consume endpoint")
	}
}

func TestCollectTopology_MultipleSetups(t *testing.T) {
	pub := NewPublisher()
	auditPub := NewPublisher()
	topo, err := CollectTopology("order-service",
		EventStreamPublisher(pub),
		StreamPublisher("audit", auditPub),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(topo.Endpoints) != 2 {
		t.Fatalf("expected 2 endpoints, got %d", len(topo.Endpoints))
	}
}

func TestCollectTopology_SetupError(t *testing.T) {
	failing := func(c *Connection) error {
		return fmt.Errorf("setup failed")
	}
	_, err := CollectTopology("svc", failing)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestCollectTopology_NoSetups(t *testing.T) {
	topo, err := CollectTopology("empty-service")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if topo.ServiceName != "empty-service" {
		t.Errorf("expected empty-service, got %q", topo.ServiceName)
	}
	if len(topo.Endpoints) != 0 {
		t.Errorf("expected 0 endpoints, got %d", len(topo.Endpoints))
	}
}
