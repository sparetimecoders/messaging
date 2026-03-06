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

	"github.com/sparetimecoders/messaging"
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
	ProbeMessages     []ProbeMessage                           `json:"probeMessages,omitempty"`
}

// ServiceConfig describes the setup intents for a single service.
type ServiceConfig struct {
	Setups []SetupIntent `json:"setups"`
}

// MessageSpec describes a message to publish and its expected deliveries.
type MessageSpec struct {
	From                 string               `json:"from"`
	RoutingKey           string               `json:"routingKey"`
	Payload              json.RawMessage      `json:"payload"`
	CustomHeaders        map[string]string    `json:"customHeaders,omitempty"`
	ExpectedDeliveries   []ExpectedDelivery   `json:"expectedDeliveries"`
	UnexpectedDeliveries []UnexpectedDelivery `json:"unexpectedDeliveries,omitempty"`
	ResponseMessages     []ResponseMessage    `json:"responseMessages,omitempty"`
}

// ResponseMessage describes a response to publish after a request is delivered.
// The TCK triggers this after asserting the initial delivery.
type ResponseMessage struct {
	From               string             `json:"from"`
	TargetService      string             `json:"targetService"`
	RoutingKey         string             `json:"routingKey"`
	Payload            json.RawMessage    `json:"payload"`
	ExpectedDeliveries []ExpectedDelivery `json:"expectedDeliveries"`
}

// UnexpectedDelivery describes a service that should NOT receive a message.
type UnexpectedDelivery struct {
	To           string `json:"to"`
	MetadataType string `json:"metadataType"`
}

// ExpectedDelivery describes where a message should be delivered and what to assert.
type ExpectedDelivery struct {
	To           string             `json:"to"`
	Metadata     MetadataAssertions `json:"metadata,omitempty"`
	PayloadMatch json.RawMessage    `json:"payloadMatch,omitempty"`
	HeaderMatch  map[string]string  `json:"headerMatch,omitempty"`
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

// PublishFunc publishes a message with the given routing key, payload, and optional headers.
type PublishFunc func(ctx context.Context, routingKey string, payload json.RawMessage, headers map[string]string) error

// ServiceHandle provides access to a running service's topology, publishers,
// and received messages.
type ServiceHandle struct {
	Topology   func() spec.Topology
	Publishers map[string]PublishFunc
	Received   func() []ReceivedMessage
	Close      func() error
}

// Deprecated: IntegrationAdapter is superseded by tck.Adapter in the
// github.com/sparetimecoders/messaging/tck module, which provides
// randomized service names, nonce injection, and TCK-owned broker validation.
type IntegrationAdapter interface {
	TransportKey() string
	StartService(t *testing.T, serviceName string, intents []SetupIntent) *ServiceHandle
	QueryBrokerState(t *testing.T) BrokerState
}

// ProbeMessage describes a cross-validation probe where the TCK itself acts as
// an independent publisher or consumer using raw broker access. This prevents
// implementations from passing the TCK by hardcoding responses.
type ProbeMessage struct {
	Direction        string                 `json:"direction"` // "outbound" or "inbound"
	PublishVia       string                 `json:"publishVia,omitempty"`
	ExpectReceivedBy string                 `json:"expectReceivedBy,omitempty"`
	RoutingKey       string                 `json:"routingKey"`
	Payload          json.RawMessage        `json:"payload"`
	RawTarget        map[string]ProbeTarget `json:"rawTarget"`
	CEAttributes     map[string]string      `json:"ceAttributes"`
	PayloadMatch     json.RawMessage        `json:"payloadMatch,omitempty"`
}

// ProbeTarget describes a transport-specific raw publish/consume target.
type ProbeTarget struct {
	Stream     string `json:"stream,omitempty"`     // NATS
	Subject    string `json:"subject,omitempty"`    // NATS
	Exchange   string `json:"exchange,omitempty"`   // AMQP
	RoutingKey string `json:"routingKey,omitempty"` // AMQP
}

// RawMessage is a message received directly from the broker.
type RawMessage struct {
	Payload json.RawMessage
	Headers map[string]string // bare CE attribute names → values
}

// ProbeConsumer reads raw messages from the broker.
type ProbeConsumer struct {
	Receive func(timeout time.Duration) *RawMessage
	Close   func()
}

// ProbeAdapter provides raw broker access for cross-validation probes.
// Adapters implementing this interface enable Phase 5 of the TCK: the runner
// independently verifies that messages actually flow through the broker,
// preventing implementations from passing by hardcoding responses.
type ProbeAdapter interface {
	// PublishRaw publishes a raw message to the broker, bypassing the implementation.
	// Header keys are bare CE attribute names ("type", "source", etc.).
	PublishRaw(t *testing.T, target ProbeTarget, payload json.RawMessage, headers map[string]string) error
	// CreateProbeConsumer sets up a raw consumer on the broker. Must be called
	// before the message is published so the consumer is ready to receive.
	CreateProbeConsumer(t *testing.T, target ProbeTarget) *ProbeConsumer
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
	case "service-response":
		return fmt.Sprintf("service-response:%s", intent.TargetService)
	case "queue-publish":
		return fmt.Sprintf("queue-publish:%s", intent.DestinationQueue)
	default:
		return intent.Pattern
	}
}

// Deprecated: RunIntegrationTCK is superseded by tck.RunTCK in the
// github.com/sparetimecoders/messaging/tck module.
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

// Deprecated: RunIntegrationTCKScenario is superseded by tck.RunScenario in the
// github.com/sparetimecoders/messaging/tck module.
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
	wt := WrapT(t)
	for svcName, h := range handles {
		expected, ok := scenario.ExpectedEndpoints[key][svcName]
		if !ok {
			continue
		}
		topo := h.Topology()
		require.Equal(t, svcName, topo.ServiceName, "service name mismatch for %s", svcName)
		AssertTopology(wt, expected, topo)
	}

	// Phase 3: Assert broker state.
	actualBroker := adapter.QueryBrokerState(t)
	switch key {
	case "amqp":
		AssertAMQPBrokerState(wt, scenario.Broker.AMQP, actualBroker.AMQP)
	case "nats":
		AssertNATSBrokerState(wt, scenario.Broker.NATS, actualBroker.NATS)
	}

	// Phase 4: Publish messages and assert delivery.
	for _, msg := range scenario.Messages {
		publishAndAssert(t, msg, scenario.Services, handles)
	}

	// Phase 5: Cross-validation probes.
	if probeAdapter, ok := adapter.(ProbeAdapter); ok {
		for _, probe := range scenario.ProbeMessages {
			target, ok := probe.RawTarget[key]
			if !ok {
				continue
			}
			switch probe.Direction {
			case "outbound":
				runOutboundProbe(t, probe, target, handles, scenario.Services, probeAdapter)
			case "inbound":
				runInboundProbe(t, probe, target, handles, probeAdapter)
			}
		}
	}
}

func publishAndAssert(t *testing.T, msg MessageSpec, services map[string]ServiceConfig, handles map[string]*ServiceHandle) {
	t.Helper()

	h, ok := handles[msg.From]
	require.True(t, ok, "service %q not found in handles", msg.From)
	require.NotEmpty(t, h.Publishers, "service %q has no publishers", msg.From)

	pub := findPublisher(t, msg.From, services[msg.From], h)
	err := pub(context.Background(), msg.RoutingKey, msg.Payload, msg.CustomHeaders)
	require.NoError(t, err, "publish failed for %s/%s", msg.From, msg.RoutingKey)

	for _, delivery := range msg.ExpectedDeliveries {
		assertDelivery(t, delivery, handles)
	}

	for _, unexpected := range msg.UnexpectedDeliveries {
		assertNoDelivery(t, unexpected, handles)
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

func assertNoDelivery(t *testing.T, unexpected UnexpectedDelivery, handles map[string]*ServiceHandle) {
	t.Helper()

	th, ok := handles[unexpected.To]
	require.True(t, ok, "target service %q not found for unexpected delivery", unexpected.To)

	// Expected deliveries have already been asserted (with up to 5s polling),
	// so messages have had plenty of time to arrive. A short additional wait
	// guards against slow routing.
	time.Sleep(200 * time.Millisecond)

	for _, rm := range th.Received() {
		assert.NotEqual(t, unexpected.MetadataType, rm.Metadata.Type,
			"unexpected delivery to %s: message with type %s should not have been received",
			unexpected.To, unexpected.MetadataType)
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

// runOutboundProbe verifies that a message published by the implementation
// actually arrives on the broker by consuming it with a raw broker client.
func runOutboundProbe(t *testing.T, probe ProbeMessage, target ProbeTarget, handles map[string]*ServiceHandle, services map[string]ServiceConfig, adapter ProbeAdapter) {
	t.Helper()

	// Set up raw consumer before publishing so it is ready to receive.
	consumer := adapter.CreateProbeConsumer(t, target)
	t.Cleanup(consumer.Close)

	// Publish via the implementation.
	h, ok := handles[probe.PublishVia]
	require.True(t, ok, "outbound probe: service %q not in handles", probe.PublishVia)
	pub := findPublisher(t, probe.PublishVia, services[probe.PublishVia], h)
	err := pub(context.Background(), probe.RoutingKey, probe.Payload, nil)
	require.NoError(t, err, "outbound probe: publish failed for %s/%s", probe.PublishVia, probe.RoutingKey)

	// Read raw message from broker.
	raw := consumer.Receive(5 * time.Second)
	require.NotNil(t, raw, "outbound probe: no raw message received from broker for %s", probe.RoutingKey)

	// Assert CE attributes.
	for attr, expected := range probe.CEAttributes {
		assert.Equal(t, expected, raw.Headers[attr],
			"outbound probe: CE attribute %q mismatch", attr)
	}

	// Assert payload.
	if len(probe.PayloadMatch) > 0 {
		assertPayloadMatch(t, "outbound-probe", raw.Payload, probe.PayloadMatch)
	}
}

// runInboundProbe verifies that the implementation's consumer actually reads
// from the broker by injecting a raw message and checking Received().
func runInboundProbe(t *testing.T, probe ProbeMessage, target ProbeTarget, handles map[string]*ServiceHandle, adapter ProbeAdapter) {
	t.Helper()

	// Publish raw message to broker.
	err := adapter.PublishRaw(t, target, probe.Payload, probe.CEAttributes)
	require.NoError(t, err, "inbound probe: raw publish failed")

	// Wait for the implementation's consumer to receive it.
	h, ok := handles[probe.ExpectReceivedBy]
	require.True(t, ok, "inbound probe: service %q not in handles", probe.ExpectReceivedBy)

	// Match by ce-source since inbound probes use a unique source ("tck-probe").
	expectedSource := probe.CEAttributes["source"]
	require.NotEmpty(t, expectedSource, "inbound probe must specify ceAttributes.source for matching")

	assert.Eventually(t, func() bool {
		for _, rm := range h.Received() {
			if rm.Metadata.Source == expectedSource {
				return true
			}
		}
		return false
	}, 5*time.Second, 50*time.Millisecond,
		"inbound probe: expected delivery to %s with source %s", probe.ExpectReceivedBy, expectedSource)

	var matched *ReceivedMessage
	for _, rm := range h.Received() {
		if rm.Metadata.Source == expectedSource {
			matched = &rm
			break
		}
	}
	if matched == nil {
		return
	}

	// Assert metadata.
	if v, ok := probe.CEAttributes["type"]; ok {
		assert.Equal(t, v, matched.Metadata.Type, "inbound probe: metadata.type mismatch")
	}
	if v, ok := probe.CEAttributes["specversion"]; ok {
		assert.Equal(t, v, matched.Metadata.SpecVersion, "inbound probe: metadata.specVersion mismatch")
	}

	// Assert payload.
	if len(probe.PayloadMatch) > 0 {
		assertPayloadMatch(t, "inbound-probe", matched.Payload, probe.PayloadMatch)
	}
}
