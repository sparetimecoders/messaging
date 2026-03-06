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

package messaging

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDiscoverTopologies(t *testing.T) {
	exchanges := []rmqExchange{
		{Name: "", Type: "direct"},                      // default exchange, filtered
		{Name: "amq.topic", Type: "topic"},              // internal, filtered
		{Name: "events.topic.exchange", Type: "topic"},  // our event exchange
		{Name: "audit.topic.exchange", Type: "topic"},   // custom stream
		{Name: "email-svc.direct.exchange.request", Type: "direct"},
		{Name: "email-svc.headers.exchange.response", Type: "headers"},
	}

	bindings := []rmqBinding{
		// Default exchange binding (filtered — source is empty)
		{Source: "", Destination: "some-queue", DestinationType: "queue", RoutingKey: "some-queue"},
		// notification-service consuming Order.Created from event stream
		{Source: "events.topic.exchange", Destination: "events.topic.exchange.queue.notifications", DestinationType: "queue", RoutingKey: "Order.Created"},
		// audit consuming with wildcard
		{Source: "events.topic.exchange", Destination: "events.topic.exchange.queue.audit", DestinationType: "queue", RoutingKey: "Order.#"},
		// analytics consuming from custom stream
		{Source: "audit.topic.exchange", Destination: "audit.topic.exchange.queue.analytics", DestinationType: "queue", RoutingKey: "Audit.Entry"},
		// email-svc consuming requests
		{Source: "email-svc.direct.exchange.request", Destination: "email-svc.direct.exchange.request.queue", DestinationType: "queue", RoutingKey: "email.send"},
		// webapp consuming responses
		{Source: "email-svc.headers.exchange.response", Destination: "email-svc.headers.exchange.response.queue.webapp", DestinationType: "queue", RoutingKey: "email.send"},
		// exchange-to-exchange binding (filtered)
		{Source: "events.topic.exchange", Destination: "other-exchange", DestinationType: "exchange", RoutingKey: "#"},
	}

	srv := newTestServer(t, exchanges, bindings)
	defer srv.Close()

	topos, err := DiscoverTopologies(BrokerConfig{
		URL:    srv.URL,
		Client: srv.Client(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should discover: analytics, audit, email-svc, notifications, webapp (sorted)
	if len(topos) != 5 {
		t.Fatalf("expected 5 services, got %d: %v", len(topos), serviceNames(topos))
	}

	expected := map[string]struct {
		endpoints int
		pattern   Pattern
	}{
		"analytics":     {1, PatternCustomStream},
		"audit":         {1, PatternEventStream},
		"email-svc":     {1, PatternServiceRequest},
		"notifications": {1, PatternEventStream},
		"webapp":        {1, PatternServiceResponse},
	}

	for _, topo := range topos {
		exp, ok := expected[topo.ServiceName]
		if !ok {
			t.Errorf("unexpected service %q", topo.ServiceName)
			continue
		}
		if len(topo.Endpoints) != exp.endpoints {
			t.Errorf("service %q: expected %d endpoints, got %d", topo.ServiceName, exp.endpoints, len(topo.Endpoints))
		}
		if topo.Endpoints[0].Pattern != exp.pattern {
			t.Errorf("service %q: expected pattern %q, got %q", topo.ServiceName, exp.pattern, topo.Endpoints[0].Pattern)
		}
	}
}

func TestDiscoverTopologies_Empty(t *testing.T) {
	srv := newTestServer(t, []rmqExchange{}, []rmqBinding{})
	defer srv.Close()

	topos, err := DiscoverTopologies(BrokerConfig{
		URL:    srv.URL,
		Client: srv.Client(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(topos) != 0 {
		t.Errorf("expected 0 topologies, got %d", len(topos))
	}
}

func TestDiscoverTopologies_TransientQueue(t *testing.T) {
	exchanges := []rmqExchange{
		{Name: "events.topic.exchange", Type: "topic"},
	}
	bindings := []rmqBinding{
		{Source: "events.topic.exchange", Destination: "events.topic.exchange.queue.dashboard-abc12345-def6-7890-abcd-ef1234567890", DestinationType: "queue", RoutingKey: "Order.#"},
	}

	srv := newTestServer(t, exchanges, bindings)
	defer srv.Close()

	topos, err := DiscoverTopologies(BrokerConfig{
		URL:    srv.URL,
		Client: srv.Client(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(topos) != 1 {
		t.Fatalf("expected 1 service, got %d", len(topos))
	}
	if topos[0].ServiceName != "dashboard" {
		t.Errorf("expected service name dashboard, got %q", topos[0].ServiceName)
	}
}

func TestDiscoverTopologies_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"not authorised"}`)) //nolint:errcheck
	}))
	defer srv.Close()

	_, err := DiscoverTopologies(BrokerConfig{
		URL:    srv.URL,
		Client: srv.Client(),
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !contains(err.Error(), "401") {
		t.Errorf("expected 401 in error, got: %v", err)
	}
}

func TestDiscoverTopologies_ChecksAuth(t *testing.T) {
	var gotUser, gotPass string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser, gotPass, _ = r.BasicAuth()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[]`)) //nolint:errcheck
	}))
	defer srv.Close()

	_, err := DiscoverTopologies(BrokerConfig{
		URL:      srv.URL,
		Username: "admin",
		Password: "secret",
		Client:   srv.Client(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotUser != "admin" || gotPass != "secret" {
		t.Errorf("expected admin:secret, got %s:%s", gotUser, gotPass)
	}
}

func TestDiscoverTopologiesAndVisualize(t *testing.T) {
	exchanges := []rmqExchange{
		{Name: "events.topic.exchange", Type: "topic"},
	}
	bindings := []rmqBinding{
		{Source: "events.topic.exchange", Destination: "events.topic.exchange.queue.notifications", DestinationType: "queue", RoutingKey: "Order.Created"},
	}

	srv := newTestServer(t, exchanges, bindings)
	defer srv.Close()

	topos, err := DiscoverTopologies(BrokerConfig{
		URL:    srv.URL,
		Client: srv.Client(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	diagram := Mermaid(topos)
	if !contains(diagram, "graph LR") {
		t.Error("expected graph LR header")
	}
	if !contains(diagram, "notifications") {
		t.Error("expected notifications in diagram")
	}
	if !contains(diagram, "events.topic.exchange") {
		t.Error("expected exchange in diagram")
	}
}

func TestInferServiceAndPattern(t *testing.T) {
	tests := []struct {
		name         string
		queue        string
		exchange     string
		kind         ExchangeKind
		wantService  string
		wantPattern  Pattern
	}{
		{
			name:        "event stream consumer",
			queue:       "events.topic.exchange.queue.orders",
			exchange:    "events.topic.exchange",
			kind:        ExchangeTopic,
			wantService: "orders",
			wantPattern: PatternEventStream,
		},
		{
			name:        "custom stream consumer",
			queue:       "audit.topic.exchange.queue.analytics",
			exchange:    "audit.topic.exchange",
			kind:        ExchangeTopic,
			wantService: "analytics",
			wantPattern: PatternCustomStream,
		},
		{
			name:        "transient consumer",
			queue:       "events.topic.exchange.queue.dashboard-abc12345-def6-7890-abcd-ef1234567890",
			exchange:    "events.topic.exchange",
			kind:        ExchangeTopic,
			wantService: "dashboard",
			wantPattern: PatternEventStream,
		},
		{
			name:        "service request",
			queue:       "email-svc.direct.exchange.request.queue",
			exchange:    "email-svc.direct.exchange.request",
			kind:        ExchangeDirect,
			wantService: "email-svc",
			wantPattern: PatternServiceRequest,
		},
		{
			name:        "service response",
			queue:       "email-svc.headers.exchange.response.queue.webapp",
			exchange:    "email-svc.headers.exchange.response",
			kind:        ExchangeHeaders,
			wantService: "webapp",
			wantPattern: PatternServiceResponse,
		},
		{
			name:        "unknown queue format",
			queue:       "random-queue",
			exchange:    "events.topic.exchange",
			kind:        ExchangeTopic,
			wantService: "",
			wantPattern: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, pat := inferServiceAndPattern(tt.queue, tt.exchange, tt.kind)
			if svc != tt.wantService {
				t.Errorf("service: got %q, want %q", svc, tt.wantService)
			}
			if pat != tt.wantPattern {
				t.Errorf("pattern: got %q, want %q", pat, tt.wantPattern)
			}
		})
	}
}

func newTestServer(t *testing.T, exchanges []rmqExchange, bindings []rmqBinding) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case contains(r.URL.Path, "/api/exchanges/"):
			json.NewEncoder(w).Encode(exchanges) //nolint:errcheck
		case contains(r.URL.Path, "/api/bindings/"):
			json.NewEncoder(w).Encode(bindings) //nolint:errcheck
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(substr) <= len(s) && searchString(s, substr)))
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func serviceNames(topos []Topology) []string {
	names := make([]string, len(topos))
	for i, t := range topos {
		names[i] = t.ServiceName
	}
	return names
}
