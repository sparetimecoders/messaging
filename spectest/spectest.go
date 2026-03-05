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

// Package spectest provides shared conformance test helpers for transport
// implementations. It loads topology scenarios from shared fixture files and
// verifies that actual topologies match expected endpoints.
package spectest

import (
	"encoding/json"
	"os"
	"sort"
	"strings"

	"github.com/sparetimecoders/messaging/specification/spec"
)

// SetupIntent describes a transport-agnostic setup action from the fixture file.
type SetupIntent struct {
	Pattern          string `json:"pattern"`
	Direction        string `json:"direction"`
	RoutingKey       string `json:"routingKey,omitempty"`
	Exchange         string `json:"exchange,omitempty"`
	TargetService    string `json:"targetService,omitempty"`
	Ephemeral        bool   `json:"ephemeral,omitempty"`
	QueueSuffix      string `json:"queueSuffix,omitempty"`
	DestinationQueue string `json:"destinationQueue,omitempty"`
}

// ExpectedEndpoint describes expected endpoint fields from the fixture file.
type ExpectedEndpoint struct {
	Direction       string `json:"direction"`
	Pattern         string `json:"pattern"`
	ExchangeName    string `json:"exchangeName"`
	ExchangeKind    string `json:"exchangeKind"`
	QueueName       string `json:"queueName,omitempty"`
	QueueNamePrefix string `json:"queueNamePrefix,omitempty"`
	RoutingKey      string `json:"routingKey,omitempty"`
	Ephemeral       bool   `json:"ephemeral,omitempty"`
}

// BrokerState holds expected broker-level state for all transports.
type BrokerState struct {
	AMQP AMQPBrokerState `json:"amqp"`
	NATS NATSBrokerState `json:"nats"`
}

// AMQPBrokerState holds expected AMQP broker declarations.
type AMQPBrokerState struct {
	Exchanges []AMQPExchange `json:"exchanges"`
	Queues    []AMQPQueue    `json:"queues"`
	Bindings  []AMQPBinding  `json:"bindings"`
}

// AMQPExchange describes an expected AMQP exchange declaration.
type AMQPExchange struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Durable    bool   `json:"durable"`
	AutoDelete bool   `json:"autoDelete"`
}

// AMQPQueue describes an expected AMQP queue declaration.
type AMQPQueue struct {
	Name       string         `json:"name,omitempty"`
	NamePrefix string         `json:"namePrefix,omitempty"`
	Durable    bool           `json:"durable"`
	AutoDelete bool           `json:"autoDelete"`
	Arguments  QueueArguments `json:"arguments"`
}

// QueueArguments holds expected AMQP queue arguments.
type QueueArguments struct {
	XQueueType string `json:"x-queue-type"`
	XExpires   int    `json:"x-expires"`
}

// AMQPBinding describes an expected AMQP queue-to-exchange binding.
type AMQPBinding struct {
	Source            string `json:"source"`
	Destination       string `json:"destination,omitempty"`
	DestinationPrefix string `json:"destinationPrefix,omitempty"`
	RoutingKey        string `json:"routingKey"`
}

// NATSBrokerState holds expected NATS broker state.
type NATSBrokerState struct {
	Streams   []NATSStream   `json:"streams"`
	Consumers []NATSConsumer `json:"consumers"`
}

// NATSStream describes an expected NATS JetStream stream.
type NATSStream struct {
	Name     string   `json:"name"`
	Subjects []string `json:"subjects"`
	Storage  string   `json:"storage"`
}

// NATSConsumer describes an expected NATS JetStream consumer.
type NATSConsumer struct {
	Stream         string   `json:"stream"`
	Durable        string   `json:"durable,omitempty"`
	FilterSubject  string   `json:"filterSubject,omitempty"`
	FilterSubjects []string `json:"filterSubjects,omitempty"`
	AckPolicy      string   `json:"ackPolicy"`
}

// Scenario represents a single topology conformance test scenario.
type Scenario struct {
	Name              string                        `json:"name"`
	ServiceName       string                        `json:"serviceName"`
	Setups            []SetupIntent                 `json:"setups"`
	ExpectedEndpoints map[string][]ExpectedEndpoint `json:"expectedEndpoints"`
	Broker            BrokerState                   `json:"broker"`
}

type fixtureFile struct {
	Scenarios []Scenario `json:"scenarios"`
}

// LoadScenarios loads topology conformance scenarios from the shared fixture file.
func LoadScenarios(t T, fixturePath string) []Scenario {
	t.Helper()
	data, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("failed to read topology fixtures: %v", err)
	}
	var f fixtureFile
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("failed to parse topology fixtures: %v", err)
	}
	if len(f.Scenarios) == 0 {
		t.Fatal("no scenarios found in fixture file")
	}
	return f.Scenarios
}

// AssertTopology verifies that actual topology endpoints match expected ones.
func AssertTopology(t T, expected []ExpectedEndpoint, actual spec.Topology) {
	t.Helper()
	if len(actual.Endpoints) != len(expected) {
		t.Errorf("endpoint count mismatch: got %d, want %d", len(actual.Endpoints), len(expected))
		return
	}
	for _, exp := range expected {
		RequireEndpointMatch(t, exp, actual.Endpoints)
	}
}

// RequireEndpointMatch finds and validates a matching endpoint in actuals.
func RequireEndpointMatch(t T, expected ExpectedEndpoint, actuals []spec.Endpoint) {
	t.Helper()
	for _, actual := range actuals {
		if spec.EndpointDirection(expected.Direction) != actual.Direction ||
			spec.Pattern(expected.Pattern) != actual.Pattern ||
			expected.ExchangeName != actual.ExchangeName ||
			expected.RoutingKey != actual.RoutingKey {
			continue
		}
		// When expected specifies a QueueName, use it as a match filter to
		// distinguish endpoints with the same direction/pattern/exchange/routingKey
		// but different queue names (e.g., suffix consumers).
		if expected.QueueName != "" && expected.QueueName != actual.QueueName {
			continue
		}
		if expected.QueueNamePrefix != "" && !strings.HasPrefix(actual.QueueName, expected.QueueNamePrefix) {
			continue
		}
		if spec.ExchangeKind(expected.ExchangeKind) != actual.ExchangeKind {
			t.Errorf("exchangeKind mismatch: got %q, want %q", actual.ExchangeKind, expected.ExchangeKind)
		}
		if expected.Ephemeral != actual.Ephemeral {
			t.Errorf("ephemeral mismatch: got %v, want %v", actual.Ephemeral, expected.Ephemeral)
		}
		return
	}
	t.Errorf("no matching endpoint found: direction=%s pattern=%s exchange=%s routingKey=%s queueName=%s",
		expected.Direction, expected.Pattern, expected.ExchangeName, expected.RoutingKey, expected.QueueName)
}

// AssertAMQPBrokerState verifies that actual AMQP broker state matches expected.
func AssertAMQPBrokerState(t T, expected, actual AMQPBrokerState) {
	t.Helper()

	// Exchanges: match by name.
	if len(actual.Exchanges) != len(expected.Exchanges) {
		t.Errorf("exchange count mismatch: got %d, want %d", len(actual.Exchanges), len(expected.Exchanges))
	} else {
		for _, exp := range expected.Exchanges {
			found := false
			for _, act := range actual.Exchanges {
				if act.Name == exp.Name {
					found = true
					if act.Type != exp.Type {
						t.Errorf("exchange %s type mismatch: got %q, want %q", exp.Name, act.Type, exp.Type)
					}
					if act.Durable != exp.Durable {
						t.Errorf("exchange %s durable mismatch: got %v, want %v", exp.Name, act.Durable, exp.Durable)
					}
					if act.AutoDelete != exp.AutoDelete {
						t.Errorf("exchange %s autoDelete mismatch: got %v, want %v", exp.Name, act.AutoDelete, exp.AutoDelete)
					}
					break
				}
			}
			if !found {
				t.Errorf("expected exchange %q not found", exp.Name)
			}
		}
	}

	// Queues: match by name (exact) or namePrefix.
	if len(actual.Queues) != len(expected.Queues) {
		t.Errorf("queue count mismatch: got %d, want %d", len(actual.Queues), len(expected.Queues))
	} else {
		for _, exp := range expected.Queues {
			found := false
			for _, act := range actual.Queues {
				if exp.Name != "" && act.Name == exp.Name {
					found = true
				} else if exp.NamePrefix != "" && strings.HasPrefix(act.Name, exp.NamePrefix) {
					found = true
				}
				if found {
					if act.Durable != exp.Durable {
						t.Errorf("queue %s durable mismatch: got %v, want %v", queueID(exp), act.Durable, exp.Durable)
					}
					if act.AutoDelete != exp.AutoDelete {
						t.Errorf("queue %s autoDelete mismatch: got %v, want %v", queueID(exp), act.AutoDelete, exp.AutoDelete)
					}
					if act.Arguments != exp.Arguments {
						t.Errorf("queue %s arguments mismatch: got %+v, want %+v", queueID(exp), act.Arguments, exp.Arguments)
					}
					break
				}
			}
			if !found {
				t.Errorf("expected queue %s not found", queueID(exp))
			}
		}
	}

	// Bindings: match by source + routingKey, then verify destination.
	if len(actual.Bindings) != len(expected.Bindings) {
		t.Errorf("binding count mismatch: got %d, want %d", len(actual.Bindings), len(expected.Bindings))
	} else {
		for _, exp := range expected.Bindings {
			found := false
			for _, act := range actual.Bindings {
				if act.Source != exp.Source || act.RoutingKey != exp.RoutingKey {
					continue
				}
				if exp.Destination != "" {
					if act.Destination != exp.Destination {
						continue
					}
				} else if exp.DestinationPrefix != "" {
					if !strings.HasPrefix(act.Destination, exp.DestinationPrefix) {
						continue
					}
				}
				found = true
				break
			}
			if !found {
				t.Errorf("expected binding not found: source=%s routingKey=%s dest=%s destPrefix=%s",
					exp.Source, exp.RoutingKey, exp.Destination, exp.DestinationPrefix)
			}
		}
	}
}

func queueID(q AMQPQueue) string {
	if q.Name != "" {
		return q.Name
	}
	return q.NamePrefix + "*"
}

// AssertNATSBrokerState verifies that actual NATS broker state matches expected.
func AssertNATSBrokerState(t T, expected, actual NATSBrokerState) {
	t.Helper()

	// Streams: match by name.
	if len(actual.Streams) != len(expected.Streams) {
		t.Errorf("stream count mismatch: got %d, want %d", len(actual.Streams), len(expected.Streams))
	} else {
		for _, exp := range expected.Streams {
			found := false
			for _, act := range actual.Streams {
				if act.Name == exp.Name {
					found = true
					actSubjects := make([]string, len(act.Subjects))
					copy(actSubjects, act.Subjects)
					expSubjects := make([]string, len(exp.Subjects))
					copy(expSubjects, exp.Subjects)
					sort.Strings(actSubjects)
					sort.Strings(expSubjects)
					if strings.Join(actSubjects, ",") != strings.Join(expSubjects, ",") {
						t.Errorf("stream %s subjects mismatch: got %v, want %v", exp.Name, act.Subjects, exp.Subjects)
					}
					if act.Storage != exp.Storage {
						t.Errorf("stream %s storage mismatch: got %q, want %q", exp.Name, act.Storage, exp.Storage)
					}
					break
				}
			}
			if !found {
				t.Errorf("expected stream %q not found", exp.Name)
			}
		}
	}

	// Consumers: match by stream+durable (durable) or stream+filterSubject (ephemeral).
	if len(actual.Consumers) != len(expected.Consumers) {
		t.Errorf("consumer count mismatch: got %d, want %d", len(actual.Consumers), len(expected.Consumers))
	} else {
		for _, exp := range expected.Consumers {
			found := false
			for _, act := range actual.Consumers {
				if act.Stream != exp.Stream {
					continue
				}
				if exp.Durable != "" {
					if act.Durable != exp.Durable {
						continue
					}
				} else {
					// Ephemeral: match by filterSubject.
					if act.FilterSubject != exp.FilterSubject {
						continue
					}
				}
				found = true
				if act.AckPolicy != exp.AckPolicy {
					t.Errorf("consumer %s/%s ackPolicy mismatch: got %q, want %q",
						exp.Stream, consumerID(exp), act.AckPolicy, exp.AckPolicy)
				}
				if exp.FilterSubject != "" && act.FilterSubject != exp.FilterSubject {
					t.Errorf("consumer %s/%s filterSubject mismatch: got %q, want %q",
						exp.Stream, consumerID(exp), act.FilterSubject, exp.FilterSubject)
				}
				if len(exp.FilterSubjects) > 0 {
					actFS := make([]string, len(act.FilterSubjects))
					copy(actFS, act.FilterSubjects)
					expFS := make([]string, len(exp.FilterSubjects))
					copy(expFS, exp.FilterSubjects)
					sort.Strings(actFS)
					sort.Strings(expFS)
					if strings.Join(actFS, ",") != strings.Join(expFS, ",") {
						t.Errorf("consumer %s/%s filterSubjects mismatch: got %v, want %v",
							exp.Stream, consumerID(exp), act.FilterSubjects, exp.FilterSubjects)
					}
				}
				break
			}
			if !found {
				t.Errorf("expected consumer not found: stream=%s durable=%s filterSubject=%s",
					exp.Stream, exp.Durable, exp.FilterSubject)
			}
		}
	}
}

func consumerID(c NATSConsumer) string {
	if c.Durable != "" {
		return c.Durable
	}
	return c.FilterSubject
}
