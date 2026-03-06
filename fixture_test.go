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
	"os"
	"path/filepath"
	"testing"
)

// Fixture types for naming.json

type namingFixtures struct {
	TopicExchangeName           []topicExchangeNameCase        `json:"topicExchangeName"`
	ServiceEventQueueName       []serviceEventQueueNameCase    `json:"serviceEventQueueName"`
	ServiceRequestExchangeName  []serviceExchangeNameCase      `json:"serviceRequestExchangeName"`
	ServiceResponseExchangeName []serviceExchangeNameCase      `json:"serviceResponseExchangeName"`
	ServiceRequestQueueName     []serviceExchangeNameCase      `json:"serviceRequestQueueName"`
	ServiceResponseQueueName    []serviceResponseQueueNameCase `json:"serviceResponseQueueName"`
	NATSStreamName              []natsStreamNameCase           `json:"natsStreamName"`
	NATSSubject                 []natsSubjectCase              `json:"natsSubject"`
	TranslateWildcard           []translateWildcardCase        `json:"translateWildcard"`
}

type topicExchangeNameCase struct {
	Input struct {
		Name string `json:"name"`
	} `json:"input"`
	Expected string `json:"expected"`
}

type serviceEventQueueNameCase struct {
	Input struct {
		ExchangeName string `json:"exchangeName"`
		Service      string `json:"service"`
	} `json:"input"`
	Expected string `json:"expected"`
}

type serviceExchangeNameCase struct {
	Input struct {
		Service string `json:"service"`
	} `json:"input"`
	Expected string `json:"expected"`
}

type serviceResponseQueueNameCase struct {
	Input struct {
		TargetService string `json:"targetService"`
		ServiceName   string `json:"serviceName"`
	} `json:"input"`
	Expected string `json:"expected"`
}

type natsStreamNameCase struct {
	Input struct {
		Name string `json:"name"`
	} `json:"input"`
	Expected string `json:"expected"`
}

type natsSubjectCase struct {
	Input struct {
		Stream     string `json:"stream"`
		RoutingKey string `json:"routingKey"`
	} `json:"input"`
	Expected string `json:"expected"`
}

type translateWildcardCase struct {
	Input struct {
		RoutingKey string `json:"routingKey"`
	} `json:"input"`
	Expected string `json:"expected"`
}

// Fixture types for validate.json

type validateFixtures struct {
	Single []singleValidateCase `json:"single"`
	Cross  []crossValidateCase  `json:"cross"`
}

type singleValidateCase struct {
	Name          string   `json:"name"`
	Topology      Topology `json:"topology"`
	ExpectedError *string  `json:"expectedError"`
}

type crossValidateCase struct {
	Name          string     `json:"name"`
	Topologies    []Topology `json:"topologies"`
	ExpectedError *string    `json:"expectedError"`
}

// Fixture types for constants.json

type constantsFixture struct {
	DefaultEventExchangeName string            `json:"defaultEventExchangeName"`
	ExchangeKinds            map[string]string `json:"exchangeKinds"`
	Directions               map[string]string `json:"directions"`
	Patterns                 map[string]string `json:"patterns"`
	Transports               map[string]string `json:"transports"`
	CloudEvents              map[string]string `json:"cloudEvents"`
}

// TestGenerateFixtures generates and verifies all fixture files.
// Run with: go test -run TestGenerateFixtures ./spec/...
func TestGenerateFixtures(t *testing.T) {
	dir := filepath.Join("testdata")

	t.Run("naming", func(t *testing.T) {
		fixtures := generateNamingFixtures()
		writeAndVerifyFixture(t, filepath.Join(dir, "naming.json"), fixtures)
	})

	t.Run("validate", func(t *testing.T) {
		fixtures := generateValidateFixtures()
		writeAndVerifyFixture(t, filepath.Join(dir, "validate.json"), fixtures)
	})

	t.Run("constants", func(t *testing.T) {
		fixtures := generateConstantsFixture()
		writeAndVerifyFixture(t, filepath.Join(dir, "constants.json"), fixtures)
	})

	t.Run("topology", func(t *testing.T) {
		fixtures := generateTopologyFixtures()
		writeAndVerifyFixture(t, filepath.Join(dir, "topology.json"), fixtures)
	})

	t.Run("cloudevents", func(t *testing.T) {
		fixtures := generateCloudEventsFixtures()
		writeAndVerifyFixture(t, filepath.Join(dir, "cloudevents.json"), fixtures)
	})
}

func writeAndVerifyFixture(t *testing.T, path string, fixture any) {
	t.Helper()

	generated, err := json.MarshalIndent(fixture, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal fixture: %v", err)
	}
	generated = append(generated, '\n')

	existing, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read existing fixture %s: %v", path, err)
	}

	if string(existing) != string(generated) {
		if err := os.WriteFile(path, generated, 0o644); err != nil {
			t.Fatalf("failed to write fixture %s: %v", path, err)
		}
		t.Errorf("fixture %s was out of date and has been regenerated; re-run tests to verify", path)
		return
	}

	// Round-trip: unmarshal and re-marshal to verify JSON is valid.
	var roundTrip any
	if err := json.Unmarshal(existing, &roundTrip); err != nil {
		t.Fatalf("fixture %s is not valid JSON: %v", path, err)
	}
}

func generateNamingFixtures() namingFixtures {
	return namingFixtures{
		TopicExchangeName: []topicExchangeNameCase{
			{
				Input: struct {
					Name string `json:"name"`
				}{Name: DefaultEventExchangeName},
				Expected: TopicExchangeName(DefaultEventExchangeName),
			},
			{
				Input: struct {
					Name string `json:"name"`
				}{Name: "orders"},
				Expected: TopicExchangeName("orders"),
			},
			{
				Input: struct {
					Name string `json:"name"`
				}{Name: "audit-log"},
				Expected: TopicExchangeName("audit-log"),
			},
			{
				Input: struct {
					Name string `json:"name"`
				}{Name: "x"},
				Expected: TopicExchangeName("x"),
			},
			{
				Input: struct {
					Name string `json:"name"`
				}{Name: "service-v2"},
				Expected: TopicExchangeName("service-v2"),
			},
		},
		ServiceEventQueueName: []serviceEventQueueNameCase{
			{
				Input: struct {
					ExchangeName string `json:"exchangeName"`
					Service      string `json:"service"`
				}{ExchangeName: "events.topic.exchange", Service: "svc"},
				Expected: ServiceEventQueueName("events.topic.exchange", "svc"),
			},
			{
				Input: struct {
					ExchangeName string `json:"exchangeName"`
					Service      string `json:"service"`
				}{ExchangeName: "orders.topic.exchange", Service: "notifications"},
				Expected: ServiceEventQueueName("orders.topic.exchange", "notifications"),
			},
			{
				Input: struct {
					ExchangeName string `json:"exchangeName"`
					Service      string `json:"service"`
				}{ExchangeName: "audit-log.topic.exchange", Service: "analytics-v2"},
				Expected: ServiceEventQueueName("audit-log.topic.exchange", "analytics-v2"),
			},
		},
		ServiceRequestExchangeName: []serviceExchangeNameCase{
			{
				Input: struct {
					Service string `json:"service"`
				}{Service: "svc"},
				Expected: ServiceRequestExchangeName("svc"),
			},
			{
				Input: struct {
					Service string `json:"service"`
				}{Service: "email-service"},
				Expected: ServiceRequestExchangeName("email-service"),
			},
			{
				Input: struct {
					Service string `json:"service"`
				}{Service: "payment-gateway-v3"},
				Expected: ServiceRequestExchangeName("payment-gateway-v3"),
			},
		},
		ServiceResponseExchangeName: []serviceExchangeNameCase{
			{
				Input: struct {
					Service string `json:"service"`
				}{Service: "svc"},
				Expected: ServiceResponseExchangeName("svc"),
			},
			{
				Input: struct {
					Service string `json:"service"`
				}{Service: "email-service"},
				Expected: ServiceResponseExchangeName("email-service"),
			},
			{
				Input: struct {
					Service string `json:"service"`
				}{Service: "payment-gateway-v3"},
				Expected: ServiceResponseExchangeName("payment-gateway-v3"),
			},
		},
		ServiceRequestQueueName: []serviceExchangeNameCase{
			{
				Input: struct {
					Service string `json:"service"`
				}{Service: "svc"},
				Expected: ServiceRequestQueueName("svc"),
			},
			{
				Input: struct {
					Service string `json:"service"`
				}{Service: "email-service"},
				Expected: ServiceRequestQueueName("email-service"),
			},
			{
				Input: struct {
					Service string `json:"service"`
				}{Service: "payment-gateway-v3"},
				Expected: ServiceRequestQueueName("payment-gateway-v3"),
			},
		},
		ServiceResponseQueueName: []serviceResponseQueueNameCase{
			{
				Input: struct {
					TargetService string `json:"targetService"`
					ServiceName   string `json:"serviceName"`
				}{TargetService: "target", ServiceName: "svc"},
				Expected: ServiceResponseQueueName("target", "svc"),
			},
			{
				Input: struct {
					TargetService string `json:"targetService"`
					ServiceName   string `json:"serviceName"`
				}{TargetService: "email-service", ServiceName: "orders"},
				Expected: ServiceResponseQueueName("email-service", "orders"),
			},
			{
				Input: struct {
					TargetService string `json:"targetService"`
					ServiceName   string `json:"serviceName"`
				}{TargetService: "payment-gateway-v3", ServiceName: "checkout"},
				Expected: ServiceResponseQueueName("payment-gateway-v3", "checkout"),
			},
		},
		NATSStreamName: []natsStreamNameCase{
			{
				Input: struct {
					Name string `json:"name"`
				}{Name: "events"},
				Expected: NATSStreamName("events"),
			},
			{
				Input: struct {
					Name string `json:"name"`
				}{Name: "events.topic.exchange"},
				Expected: NATSStreamName("events.topic.exchange"),
			},
			{
				Input: struct {
					Name string `json:"name"`
				}{Name: "audit"},
				Expected: NATSStreamName("audit"),
			},
			{
				Input: struct {
					Name string `json:"name"`
				}{Name: "audit.topic.exchange"},
				Expected: NATSStreamName("audit.topic.exchange"),
			},
			{
				Input: struct {
					Name string `json:"name"`
				}{Name: "my-service"},
				Expected: NATSStreamName("my-service"),
			},
			{
				Input: struct {
					Name string `json:"name"`
				}{Name: "events.other.suffix"},
				Expected: NATSStreamName("events.other.suffix"),
			},
		},
		NATSSubject: []natsSubjectCase{
			{
				Input: struct {
					Stream     string `json:"stream"`
					RoutingKey string `json:"routingKey"`
				}{Stream: "events", RoutingKey: "Order.Created"},
				Expected: NATSSubject("events", "Order.Created"),
			},
			{
				Input: struct {
					Stream     string `json:"stream"`
					RoutingKey string `json:"routingKey"`
				}{Stream: "audit", RoutingKey: "Audit.Entry"},
				Expected: NATSSubject("audit", "Audit.Entry"),
			},
			{
				Input: struct {
					Stream     string `json:"stream"`
					RoutingKey string `json:"routingKey"`
				}{Stream: "events", RoutingKey: "User.Profile.Updated"},
				Expected: NATSSubject("events", "User.Profile.Updated"),
			},
		},
		TranslateWildcard: []translateWildcardCase{
			{
				Input: struct {
					RoutingKey string `json:"routingKey"`
				}{RoutingKey: "Order.#"},
				Expected: TranslateWildcard("Order.#"),
			},
			{
				Input: struct {
					RoutingKey string `json:"routingKey"`
				}{RoutingKey: "Order.*"},
				Expected: TranslateWildcard("Order.*"),
			},
			{
				Input: struct {
					RoutingKey string `json:"routingKey"`
				}{RoutingKey: "Order.Created"},
				Expected: TranslateWildcard("Order.Created"),
			},
			{
				Input: struct {
					RoutingKey string `json:"routingKey"`
				}{RoutingKey: "#"},
				Expected: TranslateWildcard("#"),
			},
			{
				Input: struct {
					RoutingKey string `json:"routingKey"`
				}{RoutingKey: "*.Created"},
				Expected: TranslateWildcard("*.Created"),
			},
			{
				Input: struct {
					RoutingKey string `json:"routingKey"`
				}{RoutingKey: "Order.*.Detail.#"},
				Expected: TranslateWildcard("Order.*.Detail.#"),
			},
		},
	}
}

func generateValidateFixtures() validateFixtures {
	noErr := func() *string { return nil }
	errStr := func(s string) *string { return &s }

	return validateFixtures{
		Single: []singleValidateCase{
			{
				Name: "valid AMQP event stream",
				Topology: Topology{
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
						{
							Direction:    DirectionConsume,
							Pattern:      PatternEventStream,
							ExchangeName: "events.topic.exchange",
							ExchangeKind: ExchangeTopic,
							QueueName:    "events.topic.exchange.queue.orders",
							RoutingKey:   "User.Created",
						},
					},
				},
				ExpectedError: noErr(),
			},
			{
				Name:          "empty service name",
				Topology:      Topology{},
				ExpectedError: errStr("service name must not be empty"),
			},
			{
				Name: "missing exchange name",
				Topology: Topology{
					ServiceName: "svc",
					Endpoints: []Endpoint{
						{
							Direction:    DirectionPublish,
							ExchangeKind: ExchangeTopic,
							RoutingKey:   "Event.X",
						},
					},
				},
				ExpectedError: errStr("exchange name must not be empty"),
			},
			{
				Name: "consume missing queue",
				Topology: Topology{
					Transport:   TransportAMQP,
					ServiceName: "svc",
					Endpoints: []Endpoint{
						{
							Direction:    DirectionConsume,
							Pattern:      PatternEventStream,
							ExchangeName: "events.topic.exchange",
							ExchangeKind: ExchangeTopic,
							RoutingKey:   "Event.X",
						},
					},
				},
				ExpectedError: errStr("consume endpoint must have a queue name"),
			},
			{
				Name: "bad topic exchange format",
				Topology: Topology{
					Transport:   TransportAMQP,
					ServiceName: "svc",
					Endpoints: []Endpoint{
						{
							Direction:    DirectionPublish,
							Pattern:      PatternEventStream,
							ExchangeName: "bad-exchange",
							ExchangeKind: ExchangeTopic,
							RoutingKey:   "Event.X",
						},
					},
				},
				ExpectedError: errStr("must end with .topic.exchange"),
			},
			{
				Name: "valid AMQP request-response",
				Topology: Topology{
					Transport:   TransportAMQP,
					ServiceName: "email-service",
					Endpoints: []Endpoint{
						{
							Direction:    DirectionConsume,
							Pattern:      PatternServiceRequest,
							ExchangeName: "email-service.direct.exchange.request",
							ExchangeKind: ExchangeDirect,
							QueueName:    "email-service.direct.exchange.request.queue",
							RoutingKey:   "email.send",
						},
						{
							Direction:    DirectionPublish,
							Pattern:      PatternServiceResponse,
							ExchangeName: "email-service.headers.exchange.response",
							ExchangeKind: ExchangeHeaders,
						},
					},
				},
				ExpectedError: noErr(),
			},
			{
				Name: "missing routing key for topic exchange",
				Topology: Topology{
					ServiceName: "svc",
					Endpoints: []Endpoint{
						{
							Direction:    DirectionPublish,
							Pattern:      PatternEventStream,
							ExchangeName: "events.topic.exchange",
							ExchangeKind: ExchangeTopic,
						},
					},
				},
				ExpectedError: errStr("routing key must not be empty for topic exchange"),
			},
			{
				Name: "bad direct exchange format",
				Topology: Topology{
					Transport:   TransportAMQP,
					ServiceName: "svc",
					Endpoints: []Endpoint{
						{
							Direction:    DirectionPublish,
							Pattern:      PatternServiceRequest,
							ExchangeName: "bad-exchange",
							ExchangeKind: ExchangeDirect,
							RoutingKey:   "req.do",
						},
					},
				},
				ExpectedError: errStr("must end with .direct.exchange.request"),
			},
			{
				Name: "bad headers exchange format",
				Topology: Topology{
					Transport:   TransportAMQP,
					ServiceName: "svc",
					Endpoints: []Endpoint{
						{
							Direction:    DirectionPublish,
							Pattern:      PatternServiceResponse,
							ExchangeName: "bad-exchange",
							ExchangeKind: ExchangeHeaders,
						},
					},
				},
				ExpectedError: errStr("must end with .headers.exchange.response"),
			},
			// NATS single-validation cases
			{
				Name: "valid NATS event stream",
				Topology: Topology{
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
						{
							Direction:    DirectionConsume,
							Pattern:      PatternEventStream,
							ExchangeName: "events",
							ExchangeKind: ExchangeTopic,
							QueueName:    "orders",
							RoutingKey:   "Order.Created",
						},
					},
				},
				ExpectedError: noErr(),
			},
			{
				Name: "NATS ephemeral consumer no queue",
				Topology: Topology{
					Transport:   TransportNATS,
					ServiceName: "dashboard",
					Endpoints: []Endpoint{
						{
							Direction:    DirectionConsume,
							Pattern:      PatternEventStream,
							ExchangeName: "events",
							ExchangeKind: ExchangeTopic,
							RoutingKey:   "Order.Created",
							Ephemeral:    true,
						},
					},
				},
				ExpectedError: noErr(),
			},
			{
				Name: "NATS exchange with AMQP suffix",
				Topology: Topology{
					Transport:   TransportNATS,
					ServiceName: "svc",
					Endpoints: []Endpoint{
						{
							Direction:    DirectionPublish,
							Pattern:      PatternEventStream,
							ExchangeName: "events.topic.exchange",
							ExchangeKind: ExchangeTopic,
							RoutingKey:   "Event.X",
						},
					},
				},
				ExpectedError: errStr("must not use AMQP suffix"),
			},
			{
				Name: "routing key with > wildcard",
				Topology: Topology{
					Transport:   TransportNATS,
					ServiceName: "svc",
					Endpoints: []Endpoint{
						{
							Direction:    DirectionConsume,
							Pattern:      PatternEventStream,
							ExchangeName: "events",
							ExchangeKind: ExchangeTopic,
							QueueName:    "svc",
							RoutingKey:   "Order.>",
						},
					},
				},
				ExpectedError: errStr("routing key must not contain '>'"),
			},
			// Additional edge cases
			{
				Name: "NATS exchange with direct AMQP suffix",
				Topology: Topology{
					Transport:   TransportNATS,
					ServiceName: "svc",
					Endpoints: []Endpoint{
						{
							Direction:    DirectionPublish,
							Pattern:      PatternServiceRequest,
							ExchangeName: "svc.direct.exchange.request",
							ExchangeKind: ExchangeDirect,
							RoutingKey:   "req.do",
						},
					},
				},
				ExpectedError: errStr("must not use AMQP suffix"),
			},
			{
				Name: "NATS exchange with headers AMQP suffix",
				Topology: Topology{
					Transport:   TransportNATS,
					ServiceName: "svc",
					Endpoints: []Endpoint{
						{
							Direction:    DirectionPublish,
							Pattern:      PatternServiceResponse,
							ExchangeName: "svc.headers.exchange.response",
							ExchangeKind: ExchangeHeaders,
						},
					},
				},
				ExpectedError: errStr("must not use AMQP suffix"),
			},
			{
				Name: "NATS durable consume missing queue",
				Topology: Topology{
					Transport:   TransportNATS,
					ServiceName: "svc",
					Endpoints: []Endpoint{
						{
							Direction:    DirectionConsume,
							Pattern:      PatternEventStream,
							ExchangeName: "events",
							ExchangeKind: ExchangeTopic,
							RoutingKey:   "Order.Created",
						},
					},
				},
				ExpectedError: errStr("consume endpoint must have a queue name"),
			},
			{
				Name: "valid AMQP wildcard consumer",
				Topology: Topology{
					Transport:   TransportAMQP,
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
				ExpectedError: noErr(),
			},
			{
				Name: "valid publisher no routing key for headers exchange",
				Topology: Topology{
					Transport:   TransportAMQP,
					ServiceName: "email-service",
					Endpoints: []Endpoint{
						{
							Direction:    DirectionPublish,
							Pattern:      PatternServiceResponse,
							ExchangeName: "email-service.headers.exchange.response",
							ExchangeKind: ExchangeHeaders,
						},
					},
				},
				ExpectedError: noErr(),
			},
			{
				Name: "multiple endpoints mixed valid",
				Topology: Topology{
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
						{
							Direction:    DirectionPublish,
							Pattern:      PatternEventStream,
							ExchangeName: "events.topic.exchange",
							ExchangeKind: ExchangeTopic,
							RoutingKey:   "Order.Updated",
						},
						{
							Direction:    DirectionConsume,
							Pattern:      PatternEventStream,
							ExchangeName: "events.topic.exchange",
							ExchangeKind: ExchangeTopic,
							QueueName:    "events.topic.exchange.queue.orders",
							RoutingKey:   "Payment.Completed",
						},
					},
				},
				ExpectedError: noErr(),
			},
		},
		Cross: []crossValidateCase{
			{
				Name: "missing publisher",
				Topologies: []Topology{
					{
						ServiceName: "consumer-svc",
						Endpoints: []Endpoint{
							{
								Direction:    DirectionConsume,
								Pattern:      PatternEventStream,
								ExchangeName: "events.topic.exchange",
								ExchangeKind: ExchangeTopic,
								QueueName:    "events.topic.exchange.queue.consumer-svc",
								RoutingKey:   "Order.Created",
							},
						},
					},
				},
				ExpectedError: errStr("no service publishes it"),
			},
			{
				Name: "matching publisher",
				Topologies: []Topology{
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
				ExpectedError: noErr(),
			},
			{
				Name: "wildcard consumer skipped",
				Topologies: []Topology{
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
				ExpectedError: noErr(),
			},
			{
				Name: "NATS matching publisher",
				Topologies: []Topology{
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
					{
						Transport:   TransportNATS,
						ServiceName: "notifications",
						Endpoints: []Endpoint{
							{
								Direction:    DirectionConsume,
								Pattern:      PatternEventStream,
								ExchangeName: "events",
								ExchangeKind: ExchangeTopic,
								QueueName:    "notifications",
								RoutingKey:   "Order.Created",
							},
						},
					},
				},
				ExpectedError: noErr(),
			},
			{
				Name: "mixed transport no cross-match",
				Topologies: []Topology{
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
						ServiceName: "nats-consumer",
						Endpoints: []Endpoint{
							{
								Direction:    DirectionConsume,
								Pattern:      PatternEventStream,
								ExchangeName: "events",
								ExchangeKind: ExchangeTopic,
								QueueName:    "nats-consumer",
								RoutingKey:   "Order.Created",
							},
						},
					},
				},
				ExpectedError: errStr("no service publishes it"),
			},
			{
				Name: "multiple publishers multiple consumers",
				Topologies: []Topology{
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
							{
								Direction:    DirectionPublish,
								Pattern:      PatternEventStream,
								ExchangeName: "events.topic.exchange",
								ExchangeKind: ExchangeTopic,
								RoutingKey:   "Order.Updated",
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
					{
						ServiceName: "analytics",
						Endpoints: []Endpoint{
							{
								Direction:    DirectionConsume,
								Pattern:      PatternEventStream,
								ExchangeName: "events.topic.exchange",
								ExchangeKind: ExchangeTopic,
								QueueName:    "events.topic.exchange.queue.analytics",
								RoutingKey:   "Order.Updated",
							},
						},
					},
				},
				ExpectedError: noErr(),
			},
			{
				Name: "consumer on different exchange than publisher",
				Topologies: []Topology{
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
						ServiceName: "consumer-svc",
						Endpoints: []Endpoint{
							{
								Direction:    DirectionConsume,
								Pattern:      PatternCustomStream,
								ExchangeName: "audit.topic.exchange",
								ExchangeKind: ExchangeTopic,
								QueueName:    "audit.topic.exchange.queue.consumer-svc",
								RoutingKey:   "Order.Created",
							},
						},
					},
				},
				ExpectedError: errStr("no service publishes it"),
			},
			{
				Name: "NATS wildcard consumer skipped",
				Topologies: []Topology{
					{
						Transport:   TransportNATS,
						ServiceName: "audit",
						Endpoints: []Endpoint{
							{
								Direction:    DirectionConsume,
								Pattern:      PatternEventStream,
								ExchangeName: "events",
								ExchangeKind: ExchangeTopic,
								QueueName:    "audit",
								RoutingKey:   "Order.*",
							},
						},
					},
				},
				ExpectedError: noErr(),
			},
		},
	}
}

func generateConstantsFixture() constantsFixture {
	return constantsFixture{
		DefaultEventExchangeName: DefaultEventExchangeName,
		ExchangeKinds: map[string]string{
			"topic":   string(ExchangeTopic),
			"direct":  string(ExchangeDirect),
			"headers": string(ExchangeHeaders),
		},
		Directions: map[string]string{
			"publish": string(DirectionPublish),
			"consume": string(DirectionConsume),
		},
		Patterns: map[string]string{
			"eventStream":     string(PatternEventStream),
			"customStream":    string(PatternCustomStream),
			"serviceRequest":  string(PatternServiceRequest),
			"serviceResponse": string(PatternServiceResponse),
			"queuePublish":    string(PatternQueuePublish),
		},
		Transports: map[string]string{
			"amqp": string(TransportAMQP),
			"nats": string(TransportNATS),
		},
		CloudEvents: map[string]string{
			"specVersion":      CESpecVersion,
			"type":             CEType,
			"source":           CESource,
			"id":               CEID,
			"time":             CETime,
			"specVersionValue": CESpecVersionValue,
			"dataContentType":  CEDataContentType,
			"subject":          CESubject,
			"dataSchema":       CEDataSchema,
			"correlationId":    CECorrelationID,
		},
	}
}

// Topology fixture types

type topologyFixtures struct {
	Scenarios []topologyScenario `json:"scenarios"`
}

type setupIntent struct {
	Pattern          string `json:"pattern"`
	Direction        string `json:"direction"`
	RoutingKey       string `json:"routingKey,omitempty"`
	Exchange         string `json:"exchange,omitempty"`
	TargetService    string `json:"targetService,omitempty"`
	DestinationQueue string `json:"destinationQueue,omitempty"`
	Ephemeral        bool   `json:"ephemeral,omitempty"`
}

type topologyScenario struct {
	Name              string                        `json:"name"`
	ServiceName       string                        `json:"serviceName"`
	Setups            []setupIntent                 `json:"setups"`
	ExpectedEndpoints map[string][]expectedEndpoint `json:"expectedEndpoints"`
	Broker            brokerState                   `json:"broker"`
}

type expectedEndpoint struct {
	Direction       string `json:"direction"`
	Pattern         string `json:"pattern"`
	ExchangeName    string `json:"exchangeName"`
	ExchangeKind    string `json:"exchangeKind"`
	QueueName       string `json:"queueName,omitempty"`
	QueueNamePrefix string `json:"queueNamePrefix,omitempty"`
	RoutingKey      string `json:"routingKey,omitempty"`
	Ephemeral       bool   `json:"ephemeral,omitempty"`
}

type brokerState struct {
	AMQP amqpBrokerState `json:"amqp"`
	NATS natsBrokerState `json:"nats"`
}

type natsBrokerState struct {
	Streams   []natsStream   `json:"streams"`
	Consumers []natsConsumer `json:"consumers"`
}

type natsStream struct {
	Name     string   `json:"name"`
	Subjects []string `json:"subjects"`
	Storage  string   `json:"storage"`
}

type natsConsumer struct {
	Stream         string   `json:"stream"`
	Durable        string   `json:"durable,omitempty"`
	FilterSubject  string   `json:"filterSubject,omitempty"`
	FilterSubjects []string `json:"filterSubjects,omitempty"`
	AckPolicy      string   `json:"ackPolicy"`
}

type amqpBrokerState struct {
	Exchanges []amqpExchange `json:"exchanges"`
	Queues    []amqpQueue    `json:"queues"`
	Bindings  []amqpBinding  `json:"bindings"`
}

type amqpExchange struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Durable    bool   `json:"durable"`
	AutoDelete bool   `json:"autoDelete"`
}

type amqpQueue struct {
	Name       string         `json:"name,omitempty"`
	NamePrefix string         `json:"namePrefix,omitempty"`
	Durable    bool           `json:"durable"`
	AutoDelete bool           `json:"autoDelete"`
	Arguments  queueArguments `json:"arguments"`
}

type queueArguments struct {
	XQueueType string `json:"x-queue-type"`
	XExpires   int    `json:"x-expires"`
}

type amqpBinding struct {
	Source            string `json:"source"`
	Destination       string `json:"destination,omitempty"`
	DestinationPrefix string `json:"destinationPrefix,omitempty"`
	RoutingKey        string `json:"routingKey"`
}

// AMQP broker constants
const (
	durableQueueExpiry   = 432000000 // 5 days in ms
	transientQueueExpiry = 1000      // 1 second in ms
	quorumQueueType      = "quorum"
)

func generateTopologyFixtures() topologyFixtures {
	eventsExchange := TopicExchangeName(DefaultEventExchangeName)
	natsEvents := NATSStreamName(DefaultEventExchangeName) // "events"

	return topologyFixtures{
		Scenarios: []topologyScenario{
			{
				Name:        "event stream publisher",
				ServiceName: "orders",
				Setups: []setupIntent{
					{Pattern: "event-stream", Direction: "publish"},
				},
				ExpectedEndpoints: map[string][]expectedEndpoint{
					"amqp": {
						{
							Direction:    "publish",
							Pattern:      "event-stream",
							ExchangeName: eventsExchange,
							ExchangeKind: "topic",
						},
					},
					"nats": {
						{
							Direction:    "publish",
							Pattern:      "event-stream",
							ExchangeName: natsEvents,
							ExchangeKind: "topic",
						},
					},
				},
				Broker: brokerState{
					AMQP: amqpBrokerState{
						Exchanges: []amqpExchange{
							{Name: eventsExchange, Type: "topic", Durable: true, AutoDelete: false},
						},
						Queues:   []amqpQueue{},
						Bindings: []amqpBinding{},
					},
					NATS: natsBrokerState{
						Streams: []natsStream{
							{Name: natsEvents, Subjects: []string{NATSSubject(natsEvents, ">")}, Storage: "file"},
						},
						Consumers: []natsConsumer{},
					},
				},
			},
			{
				Name:        "event stream consumer durable",
				ServiceName: "orders",
				Setups: []setupIntent{
					{Pattern: "event-stream", Direction: "consume", RoutingKey: "Order.Created"},
				},
				ExpectedEndpoints: map[string][]expectedEndpoint{
					"amqp": {
						{
							Direction:    "consume",
							Pattern:      "event-stream",
							ExchangeName: eventsExchange,
							ExchangeKind: "topic",
							QueueName:    ServiceEventQueueName(eventsExchange, "orders"),
							RoutingKey:   "Order.Created",
						},
					},
					"nats": {
						{
							Direction:    "consume",
							Pattern:      "event-stream",
							ExchangeName: natsEvents,
							ExchangeKind: "topic",
							QueueName:    "orders",
							RoutingKey:   "Order.Created",
						},
					},
				},
				Broker: brokerState{
					AMQP: amqpBrokerState{
						Exchanges: []amqpExchange{
							{Name: eventsExchange, Type: "topic", Durable: true, AutoDelete: false},
						},
						Queues: []amqpQueue{
							{
								Name:       ServiceEventQueueName(eventsExchange, "orders"),
								Durable:    true,
								AutoDelete: false,
								Arguments:  queueArguments{XQueueType: quorumQueueType, XExpires: durableQueueExpiry},
							},
						},
						Bindings: []amqpBinding{
							{
								Source:      eventsExchange,
								Destination: ServiceEventQueueName(eventsExchange, "orders"),
								RoutingKey:  "Order.Created",
							},
						},
					},
					NATS: natsBrokerState{
						Streams: []natsStream{
							{Name: natsEvents, Subjects: []string{NATSSubject(natsEvents, ">")}, Storage: "file"},
						},
						Consumers: []natsConsumer{
							{Stream: natsEvents, Durable: "orders", FilterSubject: NATSSubject(natsEvents, "Order.Created"), AckPolicy: "explicit"},
						},
					},
				},
			},
			{
				Name:        "event stream consumer transient",
				ServiceName: "dashboard",
				Setups: []setupIntent{
					{Pattern: "event-stream", Direction: "consume", RoutingKey: "Order.Created", Ephemeral: true},
				},
				ExpectedEndpoints: map[string][]expectedEndpoint{
					"amqp": {
						{
							Direction:       "consume",
							Pattern:         "event-stream",
							ExchangeName:    eventsExchange,
							ExchangeKind:    "topic",
							QueueNamePrefix: ServiceEventQueueName(eventsExchange, "dashboard") + "-",
							RoutingKey:      "Order.Created",
							Ephemeral:       true,
						},
					},
					"nats": {
						{
							Direction:    "consume",
							Pattern:      "event-stream",
							ExchangeName: natsEvents,
							ExchangeKind: "topic",
							RoutingKey:   "Order.Created",
							Ephemeral:    true,
						},
					},
				},
				Broker: brokerState{
					AMQP: amqpBrokerState{
						Exchanges: []amqpExchange{
							{Name: eventsExchange, Type: "topic", Durable: true, AutoDelete: false},
						},
						Queues: []amqpQueue{
							{
								NamePrefix: ServiceEventQueueName(eventsExchange, "dashboard") + "-",
								Durable:    true,
								AutoDelete: false,
								Arguments:  queueArguments{XQueueType: quorumQueueType, XExpires: transientQueueExpiry},
							},
						},
						Bindings: []amqpBinding{
							{
								Source:            eventsExchange,
								DestinationPrefix: ServiceEventQueueName(eventsExchange, "dashboard") + "-",
								RoutingKey:        "Order.Created",
							},
						},
					},
					NATS: natsBrokerState{
						Streams: []natsStream{
							{Name: natsEvents, Subjects: []string{NATSSubject(natsEvents, ">")}, Storage: "file"},
						},
						Consumers: []natsConsumer{
							{Stream: natsEvents, FilterSubject: NATSSubject(natsEvents, "Order.Created"), AckPolicy: "explicit"},
						},
					},
				},
			},
			{
				Name:        "custom stream publisher and consumer",
				ServiceName: "analytics",
				Setups: []setupIntent{
					{Pattern: "custom-stream", Direction: "publish", Exchange: "audit"},
					{Pattern: "custom-stream", Direction: "consume", Exchange: "audit", RoutingKey: "Audit.Entry"},
				},
				ExpectedEndpoints: map[string][]expectedEndpoint{
					"amqp": {
						{
							Direction:    "publish",
							Pattern:      "custom-stream",
							ExchangeName: TopicExchangeName("audit"),
							ExchangeKind: "topic",
						},
						{
							Direction:    "consume",
							Pattern:      "custom-stream",
							ExchangeName: TopicExchangeName("audit"),
							ExchangeKind: "topic",
							QueueName:    ServiceEventQueueName(TopicExchangeName("audit"), "analytics"),
							RoutingKey:   "Audit.Entry",
						},
					},
					"nats": {
						{
							Direction:    "publish",
							Pattern:      "custom-stream",
							ExchangeName: NATSStreamName("audit"),
							ExchangeKind: "topic",
						},
						{
							Direction:    "consume",
							Pattern:      "custom-stream",
							ExchangeName: NATSStreamName("audit"),
							ExchangeKind: "topic",
							QueueName:    "analytics",
							RoutingKey:   "Audit.Entry",
						},
					},
				},
				Broker: brokerState{
					AMQP: amqpBrokerState{
						Exchanges: []amqpExchange{
							{Name: TopicExchangeName("audit"), Type: "topic", Durable: true, AutoDelete: false},
						},
						Queues: []amqpQueue{
							{
								Name:       ServiceEventQueueName(TopicExchangeName("audit"), "analytics"),
								Durable:    true,
								AutoDelete: false,
								Arguments:  queueArguments{XQueueType: quorumQueueType, XExpires: durableQueueExpiry},
							},
						},
						Bindings: []amqpBinding{
							{
								Source:      TopicExchangeName("audit"),
								Destination: ServiceEventQueueName(TopicExchangeName("audit"), "analytics"),
								RoutingKey:  "Audit.Entry",
							},
						},
					},
					NATS: natsBrokerState{
						Streams: []natsStream{
							{Name: NATSStreamName("audit"), Subjects: []string{NATSSubject(NATSStreamName("audit"), ">")}, Storage: "file"},
						},
						Consumers: []natsConsumer{
							{Stream: NATSStreamName("audit"), Durable: "analytics", FilterSubject: NATSSubject(NATSStreamName("audit"), "Audit.Entry"), AckPolicy: "explicit"},
						},
					},
				},
			},
			{
				Name:        "service request consumer",
				ServiceName: "email-svc",
				Setups: []setupIntent{
					{Pattern: "service-request", Direction: "consume", RoutingKey: "email.send"},
				},
				ExpectedEndpoints: map[string][]expectedEndpoint{
					"amqp": {
						{
							Direction:    "consume",
							Pattern:      "service-request",
							ExchangeName: ServiceRequestExchangeName("email-svc"),
							ExchangeKind: "direct",
							QueueName:    ServiceRequestQueueName("email-svc"),
							RoutingKey:   "email.send",
						},
					},
					"nats": {
						{
							Direction:    "consume",
							Pattern:      "service-request",
							ExchangeName: "email-svc",
							ExchangeKind: "direct",
							QueueName:    "email-svc",
							RoutingKey:   "email.send",
						},
					},
				},
				Broker: brokerState{
					AMQP: amqpBrokerState{
						Exchanges: []amqpExchange{
							{Name: ServiceRequestExchangeName("email-svc"), Type: "direct", Durable: true, AutoDelete: false},
							{Name: ServiceResponseExchangeName("email-svc"), Type: "headers", Durable: true, AutoDelete: false},
						},
						Queues: []amqpQueue{
							{
								Name:       ServiceRequestQueueName("email-svc"),
								Durable:    true,
								AutoDelete: false,
								Arguments:  queueArguments{XQueueType: quorumQueueType, XExpires: durableQueueExpiry},
							},
						},
						Bindings: []amqpBinding{
							{
								Source:      ServiceRequestExchangeName("email-svc"),
								Destination: ServiceRequestQueueName("email-svc"),
								RoutingKey:  "email.send",
							},
						},
					},
					NATS: natsBrokerState{
						Streams:   []natsStream{},
						Consumers: []natsConsumer{},
					},
				},
			},
			{
				Name:        "service publisher and response consumer",
				ServiceName: "web-app",
				Setups: []setupIntent{
					{Pattern: "service-request", Direction: "publish", TargetService: "email-svc"},
					{Pattern: "service-response", Direction: "consume", TargetService: "email-svc", RoutingKey: "email.send"},
				},
				ExpectedEndpoints: map[string][]expectedEndpoint{
					"amqp": {
						{
							Direction:    "publish",
							Pattern:      "service-request",
							ExchangeName: ServiceRequestExchangeName("email-svc"),
							ExchangeKind: "direct",
						},
						{
							Direction:    "consume",
							Pattern:      "service-response",
							ExchangeName: ServiceResponseExchangeName("email-svc"),
							ExchangeKind: "headers",
							QueueName:    ServiceResponseQueueName("email-svc", "web-app"),
							RoutingKey:   "email.send",
						},
					},
					"nats": {
						{
							Direction:    "publish",
							Pattern:      "service-request",
							ExchangeName: "email-svc",
							ExchangeKind: "direct",
						},
						{
							Direction:    "consume",
							Pattern:      "service-response",
							ExchangeName: "email-svc",
							ExchangeKind: "headers",
							QueueName:    "web-app",
							RoutingKey:   "email.send",
						},
					},
				},
				Broker: brokerState{
					AMQP: amqpBrokerState{
						Exchanges: []amqpExchange{
							{Name: ServiceRequestExchangeName("email-svc"), Type: "direct", Durable: true, AutoDelete: false},
							{Name: ServiceResponseExchangeName("email-svc"), Type: "headers", Durable: true, AutoDelete: false},
						},
						Queues: []amqpQueue{
							{
								Name:       ServiceResponseQueueName("email-svc", "web-app"),
								Durable:    true,
								AutoDelete: false,
								Arguments:  queueArguments{XQueueType: quorumQueueType, XExpires: durableQueueExpiry},
							},
						},
						Bindings: []amqpBinding{
							{
								Source:      ServiceResponseExchangeName("email-svc"),
								Destination: ServiceResponseQueueName("email-svc", "web-app"),
								RoutingKey:  "email.send",
							},
						},
					},
					NATS: natsBrokerState{
						Streams:   []natsStream{},
						Consumers: []natsConsumer{},
					},
				},
			},
			{
				Name:        "event stream durable and transient mixed",
				ServiceName: "client1",
				Setups: []setupIntent{
					{Pattern: "event-stream", Direction: "consume", RoutingKey: "Order.Created", Ephemeral: true},
					{Pattern: "event-stream", Direction: "consume", RoutingKey: "Order.Updated"},
				},
				ExpectedEndpoints: map[string][]expectedEndpoint{
					"amqp": {
						{
							Direction:       "consume",
							Pattern:         "event-stream",
							ExchangeName:    eventsExchange,
							ExchangeKind:    "topic",
							QueueNamePrefix: ServiceEventQueueName(eventsExchange, "client1") + "-",
							RoutingKey:      "Order.Created",
							Ephemeral:       true,
						},
						{
							Direction:    "consume",
							Pattern:      "event-stream",
							ExchangeName: eventsExchange,
							ExchangeKind: "topic",
							QueueName:    ServiceEventQueueName(eventsExchange, "client1"),
							RoutingKey:   "Order.Updated",
						},
					},
					"nats": {
						{
							Direction:    "consume",
							Pattern:      "event-stream",
							ExchangeName: natsEvents,
							ExchangeKind: "topic",
							RoutingKey:   "Order.Created",
							Ephemeral:    true,
						},
						{
							Direction:    "consume",
							Pattern:      "event-stream",
							ExchangeName: natsEvents,
							ExchangeKind: "topic",
							QueueName:    "client1",
							RoutingKey:   "Order.Updated",
						},
					},
				},
				Broker: brokerState{
					AMQP: amqpBrokerState{
						Exchanges: []amqpExchange{
							{Name: eventsExchange, Type: "topic", Durable: true, AutoDelete: false},
						},
						Queues: []amqpQueue{
							{
								NamePrefix: ServiceEventQueueName(eventsExchange, "client1") + "-",
								Durable:    true,
								AutoDelete: false,
								Arguments:  queueArguments{XQueueType: quorumQueueType, XExpires: transientQueueExpiry},
							},
							{
								Name:       ServiceEventQueueName(eventsExchange, "client1"),
								Durable:    true,
								AutoDelete: false,
								Arguments:  queueArguments{XQueueType: quorumQueueType, XExpires: durableQueueExpiry},
							},
						},
						Bindings: []amqpBinding{
							{
								Source:            eventsExchange,
								DestinationPrefix: ServiceEventQueueName(eventsExchange, "client1") + "-",
								RoutingKey:        "Order.Created",
							},
							{
								Source:      eventsExchange,
								Destination: ServiceEventQueueName(eventsExchange, "client1"),
								RoutingKey:  "Order.Updated",
							},
						},
					},
					NATS: natsBrokerState{
						Streams: []natsStream{
							{Name: natsEvents, Subjects: []string{NATSSubject(natsEvents, ">")}, Storage: "file"},
						},
						Consumers: []natsConsumer{
							{Stream: natsEvents, FilterSubject: NATSSubject(natsEvents, "Order.Created"), AckPolicy: "explicit"},
							{Stream: natsEvents, Durable: "client1", FilterSubject: NATSSubject(natsEvents, "Order.Updated"), AckPolicy: "explicit"},
						},
					},
				},
			},
			{
				Name:        "queue publish",
				ServiceName: "task-sender",
				Setups: []setupIntent{
					{Pattern: "queue-publish", Direction: "publish", DestinationQueue: "task-queue"},
				},
				ExpectedEndpoints: map[string][]expectedEndpoint{
					"amqp": {
						{
							Direction:    "publish",
							Pattern:      "queue-publish",
							ExchangeName: "(default)",
							QueueName:    "task-queue",
						},
					},
					"nats": {
						{
							Direction:    "publish",
							Pattern:      "queue-publish",
							ExchangeName: "task-queue",
							QueueName:    "task-queue",
						},
					},
				},
				Broker: brokerState{
					AMQP: amqpBrokerState{
						Exchanges: []amqpExchange{},
						Queues:    []amqpQueue{},
						Bindings:  []amqpBinding{},
					},
					NATS: natsBrokerState{
						Streams:   []natsStream{},
						Consumers: []natsConsumer{},
					},
				},
			},
			{
				Name:        "event stream consumer second service",
				ServiceName: "notifications",
				Setups: []setupIntent{
					{Pattern: "event-stream", Direction: "consume", RoutingKey: "Order.Created"},
				},
				ExpectedEndpoints: map[string][]expectedEndpoint{
					"amqp": {
						{
							Direction:    "consume",
							Pattern:      "event-stream",
							ExchangeName: eventsExchange,
							ExchangeKind: "topic",
							QueueName:    ServiceEventQueueName(eventsExchange, "notifications"),
							RoutingKey:   "Order.Created",
						},
					},
					"nats": {
						{
							Direction:    "consume",
							Pattern:      "event-stream",
							ExchangeName: natsEvents,
							ExchangeKind: "topic",
							QueueName:    "notifications",
							RoutingKey:   "Order.Created",
						},
					},
				},
				Broker: brokerState{
					AMQP: amqpBrokerState{
						Exchanges: []amqpExchange{
							{Name: eventsExchange, Type: "topic", Durable: true, AutoDelete: false},
						},
						Queues: []amqpQueue{
							{
								Name:       ServiceEventQueueName(eventsExchange, "notifications"),
								Durable:    true,
								AutoDelete: false,
								Arguments:  queueArguments{XQueueType: quorumQueueType, XExpires: durableQueueExpiry},
							},
						},
						Bindings: []amqpBinding{
							{
								Source:      eventsExchange,
								Destination: ServiceEventQueueName(eventsExchange, "notifications"),
								RoutingKey:  "Order.Created",
							},
						},
					},
					NATS: natsBrokerState{
						Streams: []natsStream{
							{Name: natsEvents, Subjects: []string{NATSSubject(natsEvents, ">")}, Storage: "file"},
						},
						Consumers: []natsConsumer{
							{Stream: natsEvents, Durable: "notifications", FilterSubject: NATSSubject(natsEvents, "Order.Created"), AckPolicy: "explicit"},
						},
					},
				},
			},
		},
	}
}

// CloudEvents fixture types

type cloudEventsFixtures struct {
	ValidateHeaders     []validateHeadersCase     `json:"validateHeaders"`
	MetadataFromHeaders []metadataFromHeadersCase `json:"metadataFromHeaders"`
	MessageFormat       messageFormatSpec         `json:"messageFormat"`
}

type validateHeadersCase struct {
	Name             string         `json:"name"`
	Conformance      string         `json:"conformance"`
	Headers          map[string]any `json:"headers"`
	ExpectedWarnings []string       `json:"expectedWarnings"`
}

type metadataFromHeadersCase struct {
	Name     string           `json:"name"`
	Headers  map[string]any   `json:"headers"`
	Expected expectedMetadata `json:"expected"`
}

type expectedMetadata struct {
	ID              string `json:"id"`
	Source          string `json:"source"`
	Type            string `json:"type"`
	Subject         string `json:"subject"`
	DataContentType string `json:"dataContentType"`
	SpecVersion     string `json:"specVersion"`
	CorrelationID   string `json:"correlationId"`
	Timestamp       string `json:"timestamp"`
}

type messageFormatSpec struct {
	ContentMode      string              `json:"contentMode"`
	Description      string              `json:"description"`
	HeaderPrefix     string              `json:"headerPrefix"`
	Idempotency      string              `json:"idempotency"`
	DeduplicationKey string              `json:"deduplicationKey"`
	Required         []headerRequirement `json:"required"`
	Optional         []headerRequirement `json:"optional"`
	Extensions       []headerRequirement `json:"extensions"`
	BodyEncoding     string              `json:"bodyEncoding"`
}

type headerRequirement struct {
	Header      string `json:"header"`
	Conformance string `json:"conformance"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Example     string `json:"example,omitempty"`
}

func generateCloudEventsFixtures() cloudEventsFixtures {
	return cloudEventsFixtures{
		ValidateHeaders: []validateHeadersCase{
			{
				Name:        "all required headers present",
				Conformance: "MUST",
				Headers: map[string]any{
					CESpecVersion: CESpecVersionValue,
					CEType:        "Order.Created",
					CESource:      "orders-svc",
					CEID:          "evt-123",
					CETime:        "2025-06-15T10:30:00Z",
				},
				ExpectedWarnings: []string{},
			},
			{
				Name:        "all headers including optional",
				Conformance: "MUST",
				Headers: map[string]any{
					CESpecVersion:     CESpecVersionValue,
					CEType:            "Order.Created",
					CESource:          "orders-svc",
					CEID:              "evt-456",
					CETime:            "2025-06-15T10:30:00Z",
					CESubject:         "order-789",
					CEDataContentType: "application/json",
					CEDataSchema:      "https://schema.example.com/order.json",
					CECorrelationID:   "corr-abc",
				},
				ExpectedWarnings: []string{},
			},
			{
				Name:        "empty headers",
				Conformance: "MUST",
				Headers:     map[string]any{},
				ExpectedWarnings: []string{
					"missing required attribute \"ce-specversion\"",
					"missing required attribute \"ce-type\"",
					"missing required attribute \"ce-source\"",
					"missing required attribute \"ce-id\"",
					"missing required attribute \"ce-time\"",
				},
			},
			{
				Name:        "missing specversion only",
				Conformance: "MUST",
				Headers: map[string]any{
					CEType:   "Order.Created",
					CESource: "orders-svc",
					CEID:     "evt-123",
					CETime:   "2025-06-15T10:30:00Z",
				},
				ExpectedWarnings: []string{
					"missing required attribute \"ce-specversion\"",
				},
			},
			{
				Name:        "missing type only",
				Conformance: "MUST",
				Headers: map[string]any{
					CESpecVersion: CESpecVersionValue,
					CESource:      "orders-svc",
					CEID:          "evt-123",
					CETime:        "2025-06-15T10:30:00Z",
				},
				ExpectedWarnings: []string{
					"missing required attribute \"ce-type\"",
				},
			},
			{
				Name:        "missing source only",
				Conformance: "MUST",
				Headers: map[string]any{
					CESpecVersion: CESpecVersionValue,
					CEType:        "Order.Created",
					CEID:          "evt-123",
					CETime:        "2025-06-15T10:30:00Z",
				},
				ExpectedWarnings: []string{
					"missing required attribute \"ce-source\"",
				},
			},
			{
				Name:        "missing id only",
				Conformance: "MUST",
				Headers: map[string]any{
					CESpecVersion: CESpecVersionValue,
					CEType:        "Order.Created",
					CESource:      "orders-svc",
					CETime:        "2025-06-15T10:30:00Z",
				},
				ExpectedWarnings: []string{
					"missing required attribute \"ce-id\"",
				},
			},
			{
				Name:        "missing time only",
				Conformance: "MUST",
				Headers: map[string]any{
					CESpecVersion: CESpecVersionValue,
					CEType:        "Order.Created",
					CESource:      "orders-svc",
					CEID:          "evt-123",
				},
				ExpectedWarnings: []string{
					"missing required attribute \"ce-time\"",
				},
			},
			{
				Name:        "non-string specversion",
				Conformance: "MUST",
				Headers: map[string]any{
					CESpecVersion: 1.0,
					CEType:        "Order.Created",
					CESource:      "orders-svc",
					CEID:          "evt-123",
					CETime:        "2025-06-15T10:30:00Z",
				},
				ExpectedWarnings: []string{
					"attribute \"ce-specversion\" is not a string",
				},
			},
			{
				Name:        "non-string id",
				Conformance: "MUST",
				Headers: map[string]any{
					CESpecVersion: CESpecVersionValue,
					CEType:        "Order.Created",
					CESource:      "orders-svc",
					CEID:          123,
					CETime:        "2025-06-15T10:30:00Z",
				},
				ExpectedWarnings: []string{
					"attribute \"ce-id\" is not a string",
				},
			},
			{
				Name:        "all non-string values",
				Conformance: "MUST",
				Headers: map[string]any{
					CESpecVersion: 1.0,
					CEType:        42,
					CESource:      true,
					CEID:          123,
					CETime:        99,
				},
				ExpectedWarnings: []string{
					"attribute \"ce-specversion\" is not a string",
					"attribute \"ce-type\" is not a string",
					"attribute \"ce-source\" is not a string",
					"attribute \"ce-id\" is not a string",
					"attribute \"ce-time\" is not a string",
				},
			},
			{
				Name:        "missing multiple required attributes",
				Conformance: "MUST",
				Headers: map[string]any{
					CESpecVersion: CESpecVersionValue,
					CEID:          "evt-123",
				},
				ExpectedWarnings: []string{
					"missing required attribute \"ce-type\"",
					"missing required attribute \"ce-source\"",
					"missing required attribute \"ce-time\"",
				},
			},
		},
		MetadataFromHeaders: []metadataFromHeadersCase{
			{
				Name: "all CE headers present",
				Headers: map[string]any{
					CEID:              "abc-123",
					CETime:            "2025-06-15T10:30:00Z",
					CESource:          "orders-svc",
					CEType:            "Order.Created",
					CESubject:         "order-456",
					CEDataContentType: "application/json",
					CESpecVersion:     CESpecVersionValue,
					CECorrelationID:   "corr-789",
				},
				Expected: expectedMetadata{
					ID:              "abc-123",
					Timestamp:       "2025-06-15T10:30:00Z",
					Source:          "orders-svc",
					Type:            "Order.Created",
					Subject:         "order-456",
					DataContentType: "application/json",
					SpecVersion:     CESpecVersionValue,
					CorrelationID:   "corr-789",
				},
			},
			{
				Name:    "empty headers",
				Headers: map[string]any{},
				Expected: expectedMetadata{
					ID:              "",
					Timestamp:       "",
					Source:          "",
					Type:            "",
					Subject:         "",
					DataContentType: "",
					SpecVersion:     "",
					CorrelationID:   "",
				},
			},
			{
				Name:    "only id",
				Headers: map[string]any{CEID: "id-only"},
				Expected: expectedMetadata{
					ID:              "id-only",
					Timestamp:       "",
					Source:          "",
					Type:            "",
					Subject:         "",
					DataContentType: "",
					SpecVersion:     "",
					CorrelationID:   "",
				},
			},
			{
				Name:    "only time",
				Headers: map[string]any{CETime: "2025-01-01T00:00:00Z"},
				Expected: expectedMetadata{
					ID:              "",
					Timestamp:       "2025-01-01T00:00:00Z",
					Source:          "",
					Type:            "",
					Subject:         "",
					DataContentType: "",
					SpecVersion:     "",
					CorrelationID:   "",
				},
			},
			{
				Name:    "non-string id returns empty",
				Headers: map[string]any{CEID: 42},
				Expected: expectedMetadata{
					ID:              "",
					Timestamp:       "",
					Source:          "",
					Type:            "",
					Subject:         "",
					DataContentType: "",
					SpecVersion:     "",
					CorrelationID:   "",
				},
			},
			{
				Name:    "invalid time returns empty timestamp",
				Headers: map[string]any{CETime: "not-a-time"},
				Expected: expectedMetadata{
					ID:              "",
					Timestamp:       "",
					Source:          "",
					Type:            "",
					Subject:         "",
					DataContentType: "",
					SpecVersion:     "",
					CorrelationID:   "",
				},
			},
			{
				Name:    "non-string time returns empty timestamp",
				Headers: map[string]any{CETime: 12345},
				Expected: expectedMetadata{
					ID:              "",
					Timestamp:       "",
					Source:          "",
					Type:            "",
					Subject:         "",
					DataContentType: "",
					SpecVersion:     "",
					CorrelationID:   "",
				},
			},
			{
				Name: "source and type only",
				Headers: map[string]any{
					CESource: "my-service",
					CEType:   "Event.Happened",
				},
				Expected: expectedMetadata{
					ID:              "",
					Timestamp:       "",
					Source:          "my-service",
					Type:            "Event.Happened",
					Subject:         "",
					DataContentType: "",
					SpecVersion:     "",
					CorrelationID:   "",
				},
			},
			{
				Name: "only optional attributes",
				Headers: map[string]any{
					CESubject:         "subject-1",
					CEDataContentType: "application/xml",
					CECorrelationID:   "corr-123",
				},
				Expected: expectedMetadata{
					ID:              "",
					Timestamp:       "",
					Source:          "",
					Type:            "",
					Subject:         "subject-1",
					DataContentType: "application/xml",
					SpecVersion:     "",
					CorrelationID:   "corr-123",
				},
			},
			{
				Name: "time with timezone offset",
				Headers: map[string]any{
					CETime: "2025-06-15T12:30:00+02:00",
				},
				Expected: expectedMetadata{
					ID:              "",
					Timestamp:       "2025-06-15T12:30:00+02:00",
					Source:          "",
					Type:            "",
					Subject:         "",
					DataContentType: "",
					SpecVersion:     "",
					CorrelationID:   "",
				},
			},
		},
		MessageFormat: messageFormatSpec{
			ContentMode:      "binary",
			Description:      "CloudEvents attributes are carried as transport headers (AMQP application-properties, NATS headers). Event data occupies the message body unchanged.",
			HeaderPrefix:     "ce-",
			Idempotency:      "Consumers SHOULD be idempotent. Both AMQP and NATS provide at-least-once delivery, so duplicate messages are possible.",
			DeduplicationKey: "ce-id + ce-source",
			Required: []headerRequirement{
				{Header: CESpecVersion, Conformance: "MUST", Type: "string", Description: "CloudEvents specification version", Example: CESpecVersionValue},
				{Header: CEType, Conformance: "MUST", Type: "string", Description: "Event type identifier, typically RoutingKey format", Example: "Order.Created"},
				{Header: CESource, Conformance: "MUST", Type: "string", Description: "Event source identifier, typically service name", Example: "orders-svc"},
				{Header: CEID, Conformance: "MUST", Type: "string", Description: "Unique event identifier (UUID recommended)", Example: "550e8400-e29b-41d4-a716-446655440000"},
				{Header: CETime, Conformance: "MUST", Type: "string", Description: "Event timestamp in RFC 3339 format", Example: "2025-06-15T10:30:00Z"},
			},
			Optional: []headerRequirement{
				{Header: CEDataContentType, Conformance: "SHOULD", Type: "string", Description: "Content type of the data payload", Example: "application/json"},
				{Header: CESubject, Conformance: "SHOULD", Type: "string", Description: "Subject of the event in context of the source"},
				{Header: CEDataSchema, Conformance: "SHOULD", Type: "string", Description: "URI of the data schema"},
			},
			Extensions: []headerRequirement{
				{Header: CECorrelationID, Conformance: "SHOULD", Type: "string", Description: "Correlation identifier for tracing across services"},
			},
			BodyEncoding: "JSON (application/json) is the default encoding. The body contains the event payload directly, not wrapped in a CloudEvents envelope.",
		},
	}
}
