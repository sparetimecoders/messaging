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
	"strings"
	"testing"
)

func TestMermaid(t *testing.T) {
	tests := []struct {
		name       string
		topologies []Topology
		want       string
	}{
		{
			name: "single publisher",
			topologies: []Topology{
				{
					ServiceName: "orders",
					Endpoints: []Endpoint{
						{
							Direction:    DirectionPublish,
							Pattern:      PatternEventStream,
							ExchangeName: "events.topic.exchange",
							ExchangeKind: ExchangeTopic,
							RoutingKey:   "Order.Created",
						},
					},
				},
			},
			want: strings.Join([]string{
				"graph LR",
				`    orders["orders"]`,
				`    events_topic_exchange{{"events.topic.exchange"}}`,
				`    orders -->|"Order.Created"| events_topic_exchange`,
				`    style orders fill:#f9f,stroke:#333`,
				`    style events_topic_exchange fill:#bbf,stroke:#333`,
				"",
			}, "\n"),
		},
		{
			name: "single consumer",
			topologies: []Topology{
				{
					ServiceName: "notifications",
					Endpoints: []Endpoint{
						{
							Direction:    DirectionConsume,
							Pattern:      PatternEventStream,
							ExchangeName: "events.topic.exchange",
							ExchangeKind: ExchangeTopic,
							QueueName:    "events.topic.exchange.queue.notifications",
							RoutingKey:   "Order.Created",
						},
					},
				},
			},
			want: strings.Join([]string{
				"graph LR",
				`    notifications["notifications"]`,
				`    events_topic_exchange{{"events.topic.exchange"}}`,
				`    events_topic_exchange -->|"Order.Created"| notifications`,
				`    style notifications fill:#f9f,stroke:#333`,
				`    style events_topic_exchange fill:#bbf,stroke:#333`,
				"",
			}, "\n"),
		},
		{
			name: "publisher and consumer",
			topologies: []Topology{
				{
					ServiceName: "orders",
					Endpoints: []Endpoint{
						{
							Direction:    DirectionPublish,
							Pattern:      PatternEventStream,
							ExchangeName: "events.topic.exchange",
							ExchangeKind: ExchangeTopic,
							RoutingKey:   "Order.Created",
						},
					},
				},
				{
					ServiceName: "notifications",
					Endpoints: []Endpoint{
						{
							Direction:    DirectionConsume,
							Pattern:      PatternEventStream,
							ExchangeName: "events.topic.exchange",
							ExchangeKind: ExchangeTopic,
							QueueName:    "events.topic.exchange.queue.notifications",
							RoutingKey:   "Order.Created",
						},
					},
				},
			},
			want: strings.Join([]string{
				"graph LR",
				`    notifications["notifications"]`,
				`    orders["orders"]`,
				`    events_topic_exchange{{"events.topic.exchange"}}`,
				`    events_topic_exchange -->|"Order.Created"| notifications`,
				`    orders -->|"Order.Created"| events_topic_exchange`,
				`    style notifications fill:#f9f,stroke:#333`,
				`    style orders fill:#f9f,stroke:#333`,
				`    style events_topic_exchange fill:#bbf,stroke:#333`,
				"",
			}, "\n"),
		},
		{
			name: "request response pattern",
			topologies: []Topology{
				{
					ServiceName: "email-svc",
					Endpoints: []Endpoint{
						{
							Direction:    DirectionConsume,
							Pattern:      PatternServiceRequest,
							ExchangeName: "email-svc.direct.exchange.request",
							ExchangeKind: ExchangeDirect,
							QueueName:    "email-svc.direct.exchange.request.queue",
							RoutingKey:   "email.send",
						},
						{
							Direction:    DirectionPublish,
							Pattern:      PatternServiceResponse,
							ExchangeName: "email-svc.headers.exchange.response",
							ExchangeKind: ExchangeHeaders,
						},
					},
				},
				{
					ServiceName: "web-app",
					Endpoints: []Endpoint{
						{
							Direction:    DirectionPublish,
							Pattern:      PatternServiceRequest,
							ExchangeName: "email-svc.direct.exchange.request",
							ExchangeKind: ExchangeDirect,
							RoutingKey:   "email.send",
						},
						{
							Direction:    DirectionConsume,
							Pattern:      PatternServiceResponse,
							ExchangeName: "email-svc.headers.exchange.response",
							ExchangeKind: ExchangeHeaders,
							QueueName:    "email-svc.headers.exchange.response.queue.web-app",
						},
					},
				},
			},
			want: strings.Join([]string{
				"graph LR",
				`    email_svc["email-svc"]`,
				`    web_app["web-app"]`,
				`    email_svc_direct_exchange_request["email-svc.direct.exchange.request"]`,
				`    email_svc_headers_exchange_response(("email-svc.headers.exchange.response"))`,
				`    email_svc -.-> email_svc_headers_exchange_response`,
				`    email_svc_direct_exchange_request -->|"email.send"| email_svc`,
				`    email_svc_headers_exchange_response -.-> web_app`,
				`    web_app -->|"email.send"| email_svc_direct_exchange_request`,
				`    style email_svc fill:#f9f,stroke:#333`,
				`    style web_app fill:#f9f,stroke:#333`,
				`    style email_svc_direct_exchange_request fill:#bbf,stroke:#333`,
				`    style email_svc_headers_exchange_response fill:#bbf,stroke:#333`,
				"",
			}, "\n"),
		},
		{
			name: "wildcard consumer",
			topologies: []Topology{
				{
					ServiceName: "audit",
					Endpoints: []Endpoint{
						{
							Direction:    DirectionConsume,
							Pattern:      PatternEventStream,
							ExchangeName: "events.topic.exchange",
							ExchangeKind: ExchangeTopic,
							QueueName:    "events.topic.exchange.queue.audit",
							RoutingKey:   "Order.#",
						},
					},
				},
			},
			want: strings.Join([]string{
				"graph LR",
				`    audit["audit"]`,
				`    events_topic_exchange{{"events.topic.exchange"}}`,
				`    events_topic_exchange -->|"Order.#"| audit`,
				`    style audit fill:#f9f,stroke:#333`,
				`    style events_topic_exchange fill:#bbf,stroke:#333`,
				"",
			}, "\n"),
		},
		{
			name: "multiple exchanges",
			topologies: []Topology{
				{
					ServiceName: "analytics",
					Endpoints: []Endpoint{
						{
							Direction:    DirectionPublish,
							Pattern:      PatternCustomStream,
							ExchangeName: "audit.topic.exchange",
							ExchangeKind: ExchangeTopic,
							RoutingKey:   "Audit.Entry",
						},
						{
							Direction:    DirectionConsume,
							Pattern:      PatternEventStream,
							ExchangeName: "events.topic.exchange",
							ExchangeKind: ExchangeTopic,
							QueueName:    "events.topic.exchange.queue.analytics",
							RoutingKey:   "Order.Created",
						},
					},
				},
			},
			want: strings.Join([]string{
				"graph LR",
				`    analytics["analytics"]`,
				`    audit_topic_exchange{{"audit.topic.exchange"}}`,
				`    events_topic_exchange{{"events.topic.exchange"}}`,
				`    analytics -->|"Audit.Entry"| audit_topic_exchange`,
				`    events_topic_exchange -->|"Order.Created"| analytics`,
				`    style analytics fill:#f9f,stroke:#333`,
				`    style audit_topic_exchange fill:#bbf,stroke:#333`,
				`    style events_topic_exchange fill:#bbf,stroke:#333`,
				"",
			}, "\n"),
		},
		{
			name:       "empty topologies",
			topologies: nil,
			want: strings.Join([]string{
				"graph LR",
				"",
			}, "\n"),
		},
		{
			name:       "empty slice",
			topologies: []Topology{},
			want: strings.Join([]string{
				"graph LR",
				"",
			}, "\n"),
		},
		{
			name: "multi-transport subgraphs",
			topologies: []Topology{
				{
					Transport:   TransportAMQP,
					ServiceName: "orders",
					Endpoints: []Endpoint{
						{
							Direction:    DirectionPublish,
							Pattern:      PatternEventStream,
							ExchangeName: "events.topic.exchange",
							ExchangeKind: ExchangeTopic,
							RoutingKey:   "Order.Created",
						},
					},
				},
				{
					Transport:   TransportNATS,
					ServiceName: "orders",
					Endpoints: []Endpoint{
						{
							Direction:    DirectionPublish,
							Pattern:      PatternEventStream,
							ExchangeName: "events",
							ExchangeKind: ExchangeTopic,
							RoutingKey:   "Order.Created",
						},
					},
				},
			},
			want: strings.Join([]string{
				"graph LR",
				`    orders["orders"]`,
				`    subgraph AMQP`,
				`        amqp_events_topic_exchange{{"events.topic.exchange"}}`,
				`    end`,
				`    subgraph NATS`,
				`        nats_events{{"events"}}`,
				`    end`,
				`    orders -->|"Order.Created"| nats_events`,
				`    orders -->|"Order.Created"| amqp_events_topic_exchange`,
				`    style orders fill:#f9f,stroke:#333`,
				`    style nats_events fill:#bbf,stroke:#333`,
				`    style amqp_events_topic_exchange fill:#bbf,stroke:#333`,
				"",
			}, "\n"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Mermaid(tt.topologies)
			if got != tt.want {
				t.Errorf("Mermaid() mismatch\ngot:\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}

func TestMermaidWithFixtureTopologies(t *testing.T) {
	fixtures := loadValidateFixtures(t)

	// Use the cross-validation "matching publisher" case — it has two services
	for _, tc := range fixtures.Cross {
		if tc.ExpectedError != nil {
			continue // skip error cases
		}
		t.Run(tc.Name, func(t *testing.T) {
			got := Mermaid(tc.Topologies)
			if !strings.HasPrefix(got, "graph LR\n") {
				t.Errorf("expected graph LR header, got:\n%s", got)
			}
			// Verify each service appears as a node
			for _, topo := range tc.Topologies {
				if !strings.Contains(got, topo.ServiceName) {
					t.Errorf("expected service %q in diagram, got:\n%s", topo.ServiceName, got)
				}
			}
			// Verify each exchange appears
			for _, topo := range tc.Topologies {
				for _, ep := range topo.Endpoints {
					if !strings.Contains(got, ep.ExchangeName) {
						t.Errorf("expected exchange %q in diagram, got:\n%s", ep.ExchangeName, got)
					}
				}
			}
		})
	}
}

func TestSanitizeID(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"orders", "orders"},
		{"events.topic.exchange", "events_topic_exchange"},
		{"email-svc", "email_svc"},
		{"email-svc.direct.exchange.request", "email_svc_direct_exchange_request"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := sanitizeID(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeID(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
