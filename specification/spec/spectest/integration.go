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

package spectest

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/sparetimecoders/gomessaging/spec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TCKScenario describes a multi-service integration test scenario.
type TCKScenario struct {
	Name              string                                   `json:"name"`
	Services          map[string]ServiceConfig                 `json:"services"`
	ExpectedEndpoints map[string]map[string][]ExpectedEndpoint `json:"expectedEndpoints"`
	Broker            BrokerState                              `json:"broker"`
	Messages          []MessageSpec                            `json:"messages"`
}

// ServiceConfig describes the setup intents for a single service.
type ServiceConfig struct {
	Setups []SetupIntent `json:"setups"`
}

// MessageSpec describes a message to publish and its expected deliveries.
type MessageSpec struct {
	From               string             `json:"from"`
	RoutingKey         string             `json:"routingKey"`
	Payload            json.RawMessage    `json:"payload"`
	ExpectedDeliveries []ExpectedDelivery `json:"expectedDeliveries"`
}

// ExpectedDelivery describes where a message should be delivered and what to assert.
type ExpectedDelivery struct {
	To           string             `json:"to"`
	Metadata     MetadataAssertions `json:"metadata,omitempty"`
	PayloadMatch json.RawMessage    `json:"payloadMatch,omitempty"`
}

// MetadataAssertions holds optional metadata fields to assert on received messages.
type MetadataAssertions struct {
	Type            string `json:"type,omitempty"`
	Source          string `json:"source,omitempty"`
	SpecVersion     string `json:"specVersion,omitempty"`
	DataContentType string `json:"dataContentType,omitempty"`
}

// ReceivedMessage captures a message received by a consumer.
type ReceivedMessage struct {
	RoutingKey string
	Payload    json.RawMessage
	Metadata   spec.Metadata
	Info       spec.DeliveryInfo
}

// PublishFunc publishes a message with the given routing key and payload.
type PublishFunc func(ctx context.Context, routingKey string, payload json.RawMessage) error

// ServiceHandle provides access to a running service's topology, publishers,
// and received messages.
type ServiceHandle struct {
	Topology   func() spec.Topology
	Publishers map[string]PublishFunc
	Received   func() []ReceivedMessage
	Close      func() error
}

// IntegrationAdapter provides transport-specific operations for the integration TCK.
type IntegrationAdapter interface {
	TransportKey() string
	StartService(t *testing.T, serviceName string, intents []SetupIntent) *ServiceHandle
	QueryBrokerState(t *testing.T) BrokerState
}

type tckFixtureFile struct {
	Scenarios []TCKScenario `json:"scenarios"`
}

// LoadTCKScenarios loads integration TCK scenarios from a fixture file.
func LoadTCKScenarios(t *testing.T, fixturePath string) []TCKScenario {
	t.Helper()
	data, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("failed to read TCK fixtures: %v", err)
	}
	var f tckFixtureFile
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("failed to parse TCK fixtures: %v", err)
	}
	if len(f.Scenarios) == 0 {
		t.Fatal("no scenarios found in TCK fixture file")
	}
	return f.Scenarios
}

// PublisherKey derives the publisher map key from a SetupIntent.
func PublisherKey(intent SetupIntent) string {
	switch intent.Pattern {
	case "event-stream":
		return "event-stream"
	case "custom-stream":
		return fmt.Sprintf("custom-stream:%s", intent.Exchange)
	case "service-request":
		return fmt.Sprintf("service-request:%s", intent.TargetService)
	default:
		return intent.Pattern
	}
}

// RunIntegrationTCK runs the integration test suite for a given transport adapter.
// All scenarios share the same adapter (and thus the same broker). For transports
// that need a fresh broker per scenario, use RunIntegrationTCKScenario directly.
func RunIntegrationTCK(t *testing.T, fixturePath string, adapter IntegrationAdapter) {
	t.Helper()
	scenarios := LoadTCKScenarios(t, fixturePath)
	key := adapter.TransportKey()

	for _, scenario := range scenarios {
		if _, ok := scenario.ExpectedEndpoints[key]; !ok {
			continue
		}

		t.Run(scenario.Name, func(t *testing.T) {
			RunIntegrationTCKScenario(t, adapter, scenario)
		})
	}
}

// RunIntegrationTCKScenario runs a single TCK scenario against the given adapter.
func RunIntegrationTCKScenario(t *testing.T, adapter IntegrationAdapter, scenario TCKScenario) {
	t.Helper()
	key := adapter.TransportKey()

	if _, ok := scenario.ExpectedEndpoints[key]; !ok {
		t.Skipf("no expected endpoints for transport %q", key)
	}

	// Phase 1: Start all services.
	handles := make(map[string]*ServiceHandle)
	for svcName, svcCfg := range scenario.Services {
		h := adapter.StartService(t, svcName, svcCfg.Setups)
		t.Cleanup(func() { _ = h.Close() })
		handles[svcName] = h
	}

	// Phase 2: Assert topology for each service.
	for svcName, h := range handles {
		expected, ok := scenario.ExpectedEndpoints[key][svcName]
		if !ok {
			continue
		}
		topo := h.Topology()
		require.Equal(t, svcName, topo.ServiceName, "service name mismatch for %s", svcName)
		AssertTopology(t, expected, topo)
	}

	// Phase 3: Assert broker state.
	actualBroker := adapter.QueryBrokerState(t)
	switch key {
	case "amqp":
		AssertAMQPBrokerState(t, scenario.Broker.AMQP, actualBroker.AMQP)
	case "nats":
		AssertNATSBrokerState(t, scenario.Broker.NATS, actualBroker.NATS)
	}

	// Phase 4: Publish messages and assert delivery.
	for _, msg := range scenario.Messages {
		publishAndAssert(t, msg, scenario.Services, handles)
	}
}

func publishAndAssert(t *testing.T, msg MessageSpec, services map[string]ServiceConfig, handles map[string]*ServiceHandle) {
	t.Helper()

	h, ok := handles[msg.From]
	require.True(t, ok, "service %q not found in handles", msg.From)
	require.NotEmpty(t, h.Publishers, "service %q has no publishers", msg.From)

	pub := findPublisher(t, msg.From, services[msg.From], h)
	err := pub(context.Background(), msg.RoutingKey, msg.Payload)
	require.NoError(t, err, "publish failed for %s/%s", msg.From, msg.RoutingKey)

	for _, delivery := range msg.ExpectedDeliveries {
		assertDelivery(t, delivery, handles)
	}
}

func findPublisher(t *testing.T, svcName string, svcCfg ServiceConfig, h *ServiceHandle) PublishFunc {
	t.Helper()
	if len(h.Publishers) == 1 {
		for _, p := range h.Publishers {
			return p
		}
	}
	for _, intent := range svcCfg.Setups {
		if intent.Direction == "publish" {
			pk := PublisherKey(intent)
			if p, exists := h.Publishers[pk]; exists {
				return p
			}
		}
	}
	t.Fatalf("no publisher found for service %q", svcName)
	return nil
}

func assertDelivery(t *testing.T, delivery ExpectedDelivery, handles map[string]*ServiceHandle) {
	t.Helper()

	th, ok := handles[delivery.To]
	require.True(t, ok, "target service %q not found", delivery.To)

	assert.Eventually(t, func() bool {
		for _, rm := range th.Received() {
			if rm.Metadata.Type == delivery.Metadata.Type {
				return true
			}
		}
		return false
	}, 5*time.Second, 50*time.Millisecond,
		"expected delivery to %s with type %s", delivery.To, delivery.Metadata.Type)

	var matched *ReceivedMessage
	for _, rm := range th.Received() {
		if rm.Metadata.Type == delivery.Metadata.Type {
			matched = &rm
			break
		}
	}
	if matched == nil {
		return
	}

	if delivery.Metadata.Type != "" {
		assert.Equal(t, delivery.Metadata.Type, matched.Metadata.Type,
			"metadata.type mismatch for delivery to %s", delivery.To)
	}
	if delivery.Metadata.Source != "" {
		assert.Equal(t, delivery.Metadata.Source, matched.Metadata.Source,
			"metadata.source mismatch for delivery to %s", delivery.To)
	}
	if delivery.Metadata.SpecVersion != "" {
		assert.Equal(t, delivery.Metadata.SpecVersion, matched.Metadata.SpecVersion,
			"metadata.specVersion mismatch for delivery to %s", delivery.To)
	}
	if delivery.Metadata.DataContentType != "" {
		assert.Equal(t, delivery.Metadata.DataContentType, matched.Metadata.DataContentType,
			"metadata.dataContentType mismatch for delivery to %s", delivery.To)
	}

	if len(delivery.PayloadMatch) > 0 {
		assertPayloadMatch(t, delivery.To, matched.Payload, delivery.PayloadMatch)
	}
}

func assertPayloadMatch(t *testing.T, target string, actual, expected json.RawMessage) {
	t.Helper()
	var actualMap, expectedMap map[string]any
	require.NoError(t, json.Unmarshal(actual, &actualMap), "failed to unmarshal actual payload for %s", target)
	require.NoError(t, json.Unmarshal(expected, &expectedMap), "failed to unmarshal expected payload for %s", target)
	for k, v := range expectedMap {
		assert.Equal(t, v, actualMap[k], "payload field %q mismatch for delivery to %s", k, target)
	}
}
