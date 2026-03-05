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

package tck

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"time"

	"github.com/sparetimecoders/messaging/specification/spec/spectest"
)

// Adapter provides transport-specific service startup for the TCK.
// All broker access (querying state, raw publish/consume) is handled by the
// TCK itself — implementors only need to start services.
type Adapter interface {
	TransportKey() string
	BrokerConfig() BrokerConfig
	StartService(t spectest.T, serviceName string, intents []spectest.SetupIntent) *spectest.ServiceHandle
}

// Scenario describes a multi-service integration test scenario.
// Unlike spectest.TCKScenario, this type does not include expectedEndpoints,
// broker state, or rawTarget — those are computed at runtime.
type Scenario struct {
	Name          string                            `json:"name"`
	Transports    []string                          `json:"transports,omitempty"`
	Services      map[string]spectest.ServiceConfig `json:"services"`
	Messages      []spectest.MessageSpec            `json:"messages"`
	ProbeMessages []ProbeMessage                    `json:"probeMessages,omitempty"`
}

// ProbeMessage describes a cross-validation probe. Unlike spectest.ProbeMessage,
// this type omits rawTarget since it is computed at runtime.
type ProbeMessage struct {
	Direction        string            `json:"direction"` // "outbound" or "inbound"
	PublishVia       string            `json:"publishVia,omitempty"`
	ExpectReceivedBy string            `json:"expectReceivedBy,omitempty"`
	RoutingKey       string            `json:"routingKey"`
	Payload          json.RawMessage   `json:"payload"`
	CEAttributes     map[string]string `json:"ceAttributes"`
	PayloadMatch     json.RawMessage   `json:"payloadMatch,omitempty"`
}

type scenarioFile struct {
	Scenarios []Scenario `json:"scenarios"`
}

// LoadScenariosFile loads TCK scenarios from a fixture file, returning an error
// instead of calling t.Fatal. This is used by the standalone tck-runner binary.
func LoadScenariosFile(fixturePath string) ([]Scenario, error) {
	data, err := os.ReadFile(fixturePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read TCK fixtures: %w", err)
	}
	var f scenarioFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("failed to parse TCK fixtures: %w", err)
	}
	if len(f.Scenarios) == 0 {
		return nil, fmt.Errorf("no scenarios in TCK fixture")
	}
	return f.Scenarios, nil
}

// LoadScenarios loads TCK scenarios from a fixture file.
func LoadScenarios(t spectest.T, fixturePath string) []Scenario {
	t.Helper()
	scenarios, err := LoadScenariosFile(fixturePath)
	spectest.RequireNoError(t, err)
	return scenarios
}

// RunTCK runs the full integration test suite. Each scenario shares the same
// adapter instance. For transports that need a fresh broker per scenario (e.g.
// NATS embedded server), use LoadScenarios + RunScenario directly.
func RunTCK(t spectest.T, fixturePath string, adapter Adapter) {
	t.Helper()
	scenarios := LoadScenarios(t, fixturePath)
	key := adapter.TransportKey()
	for _, scenario := range scenarios {
		if len(scenario.Transports) > 0 && !slices.Contains(scenario.Transports, key) {
			t.Logf("skipping scenario %q (transport %s not in %v)", scenario.Name, key, scenario.Transports)
			continue
		}
		t.Run(scenario.Name, func(t spectest.T) {
			RunScenario(t, adapter, scenario)
		})
	}
}

// RunScenario runs a single TCK scenario with randomized service names,
// nonce-injected payloads, and TCK-owned broker validation.
func RunScenario(t spectest.T, adapter Adapter, scenario Scenario) {
	t.Helper()
	key := adapter.TransportKey()

	if len(scenario.Transports) > 0 && !slices.Contains(scenario.Transports, key) {
		t.Logf("skipping scenario %q (transport %s not in %v)", scenario.Name, key, scenario.Transports)
		return
	}

	broker, mapper, handles := phaseSetup(t, adapter, scenario)
	phaseTopology(t, key, scenario, handles, mapper)
	phaseBrokerState(t, key, scenario, broker, mapper)
	phaseDelivery(t, scenario, handles, mapper)
	phaseProbes(t, key, scenario, handles, broker, mapper)
}

// RunScenarioWithReport runs a single scenario and returns a structured report
// with per-phase timing and error capture.
func RunScenarioWithReport(t spectest.T, adapter Adapter, scenario Scenario) ScenarioReport {
	t.Helper()
	start := time.Now()
	key := adapter.TransportKey()

	report := ScenarioReport{
		Name:     scenario.Name,
		Services: scenarioServiceNames(scenario.Services),
		Patterns: scenarioPatterns(scenario.Services),
		Passed:   true,
	}

	rt := newReportingT(t)

	// Phase: setup.
	phaseStart := time.Now()
	broker, mapper, handles := phaseSetup(rt, adapter, scenario)
	setupErrors := rt.snapshot()
	setupFatal := rt.hasFatal()
	report.Phases = append(report.Phases, PhaseResult{
		Name: "setup", Passed: len(setupErrors) == 0, Duration: time.Since(phaseStart), Errors: setupErrors,
	})
	if setupFatal || len(setupErrors) > 0 {
		report.Passed = false
		report.Duration = time.Since(start)
		return report
	}

	// Phase: topology.
	phaseStart = time.Now()
	phaseTopology(rt, key, scenario, handles, mapper)
	topoErrors := rt.snapshot()
	report.Phases = append(report.Phases, PhaseResult{
		Name: "topology", Passed: len(topoErrors) == 0, Duration: time.Since(phaseStart), Errors: topoErrors,
	})
	if len(topoErrors) > 0 {
		report.Passed = false
	}

	// Phase: broker.
	phaseStart = time.Now()
	phaseBrokerState(rt, key, scenario, broker, mapper)
	brokerErrors := rt.snapshot()
	report.Phases = append(report.Phases, PhaseResult{
		Name: "broker", Passed: len(brokerErrors) == 0, Duration: time.Since(phaseStart), Errors: brokerErrors,
	})
	if len(brokerErrors) > 0 {
		report.Passed = false
	}

	// Phase: delivery.
	phaseStart = time.Now()
	phaseDelivery(rt, scenario, handles, mapper)
	deliveryErrors := rt.snapshot()
	report.Phases = append(report.Phases, PhaseResult{
		Name: "delivery", Passed: len(deliveryErrors) == 0, Duration: time.Since(phaseStart), Errors: deliveryErrors,
	})
	if len(deliveryErrors) > 0 {
		report.Passed = false
	}

	// Phase: probes.
	phaseStart = time.Now()
	phaseProbes(rt, key, scenario, handles, broker, mapper)
	probeErrors := rt.snapshot()
	report.Phases = append(report.Phases, PhaseResult{
		Name: "probes", Passed: len(probeErrors) == 0, Duration: time.Since(phaseStart), Errors: probeErrors,
	})
	if len(probeErrors) > 0 {
		report.Passed = false
	}

	report.Duration = time.Since(start)
	return report
}

// RunTCKWithReport runs all scenarios and produces a full conformance report.
func RunTCKWithReport(t spectest.T, fixturePath string, adapter Adapter) *TCKReport {
	t.Helper()
	scenarios := LoadScenarios(t, fixturePath)
	key := adapter.TransportKey()

	report := &TCKReport{
		TransportKey: key,
		Timestamp:    time.Now(),
	}

	var runScenarios []Scenario
	for _, scenario := range scenarios {
		if len(scenario.Transports) > 0 && !slices.Contains(scenario.Transports, key) {
			t.Logf("skipping scenario %q (transport %s not in %v)", scenario.Name, key, scenario.Transports)
			continue
		}
		runScenarios = append(runScenarios, scenario)
		s := scenario
		t.Run(s.Name, func(t spectest.T) {
			sr := RunScenarioWithReport(t, adapter, s)
			report.Scenarios = append(report.Scenarios, sr)
			if sr.Passed {
				report.Summary.Passed++
			} else {
				report.Summary.Failed++
			}
			report.Summary.Total++
		})
	}

	report.Coverage = ComputeCoverageMatrix(runScenarios)
	return report
}

// --- Phase functions ---

func phaseSetup(t spectest.T, adapter Adapter, scenario Scenario) (BrokerClient, *NameMapper, map[string]*spectest.ServiceHandle) {
	t.Helper()
	key := adapter.TransportKey()

	broker := newBrokerClient(key, adapter.BrokerConfig())
	spectest.RequireNotNil(t, broker, "no broker client for transport %q", key)
	broker.Cleanup(t)
	t.Cleanup(func() { broker.Cleanup(t) })

	templateNames := make([]string, 0, len(scenario.Services))
	for name := range scenario.Services {
		templateNames = append(templateNames, name)
	}
	mapper := NewNameMapper(templateNames)
	t.Logf("TCK name mapper: suffix=%s", mapper.Suffix())
	for _, name := range templateNames {
		t.Logf("  %s -> %s", name, mapper.Runtime(name))
	}

	handles := make(map[string]*spectest.ServiceHandle)
	for templateName, svcCfg := range scenario.Services {
		runtimeName := mapper.Runtime(templateName)
		mappedIntents := mapper.MapIntents(svcCfg.Setups)
		h := adapter.StartService(t, runtimeName, mappedIntents)
		t.Cleanup(func() { _ = h.Close() })
		handles[templateName] = h
	}

	return broker, mapper, handles
}

func phaseTopology(t spectest.T, key string, scenario Scenario, handles map[string]*spectest.ServiceHandle, mapper *NameMapper) {
	t.Helper()
	expectedEndpoints := ComputeExpectedEndpoints(key, scenario.Services, mapper)
	for templateName, h := range handles {
		expected, ok := expectedEndpoints[templateName]
		if !ok {
			continue
		}
		topo := h.Topology()
		runtimeName := mapper.Runtime(templateName)
		spectest.RequireEqual(t, runtimeName, topo.ServiceName, "service name mismatch for %s", templateName)
		spectest.AssertTopology(t, expected, topo)
	}
}

func phaseBrokerState(t spectest.T, key string, scenario Scenario, broker BrokerClient, mapper *NameMapper) {
	t.Helper()
	expectedBroker := ComputeExpectedBrokerState(key, scenario.Services, mapper)
	actualBroker := broker.QueryState(t)
	switch key {
	case "amqp":
		spectest.AssertAMQPBrokerState(t, expectedBroker.AMQP, actualBroker.AMQP)
	case "nats":
		spectest.AssertNATSBrokerState(t, expectedBroker.NATS, actualBroker.NATS)
	}
}

func phaseDelivery(t spectest.T, scenario Scenario, handles map[string]*spectest.ServiceHandle, mapper *NameMapper) {
	t.Helper()
	for _, msg := range scenario.Messages {
		publishAndAssert(t, msg, scenario.Services, handles, mapper)
	}
}

func phaseProbes(t spectest.T, key string, scenario Scenario, handles map[string]*spectest.ServiceHandle, broker BrokerClient, mapper *NameMapper) {
	t.Helper()
	for _, probe := range scenario.ProbeMessages {
		target := ComputeProbeTarget(key, probe, scenario.Services, mapper)
		switch probe.Direction {
		case "outbound":
			runOutboundProbe(t, probe, target, handles, scenario.Services, broker, mapper)
		case "inbound":
			runInboundProbe(t, probe, target, handles, broker, mapper)
		}
	}
}

func publishAndAssert(t spectest.T, msg spectest.MessageSpec, services map[string]spectest.ServiceConfig, handles map[string]*spectest.ServiceHandle, mapper *NameMapper) {
	t.Helper()

	h, ok := handles[msg.From]
	spectest.RequireTrue(t, ok, "service %q not found in handles", msg.From)
	spectest.RequireNotEmpty(t, h.Publishers, "service %q has no publishers", msg.From)

	// Inject nonce into payload.
	payload, nonce := InjectNonce(msg.Payload)

	pub := findPublisher(t, msg.From, services[msg.From], h)
	err := pub(context.Background(), msg.RoutingKey, payload, msg.CustomHeaders)
	spectest.RequireNoError(t, err, "publish failed for %s/%s", msg.From, msg.RoutingKey)

	// Track matched message indices per service to support multi-delivery.
	matchedIndices := make(map[string]map[int]bool)
	for _, delivery := range msg.ExpectedDeliveries {
		assertDelivery(t, delivery, handles, mapper, nonce, matchedIndices)
	}

	for _, unexpected := range msg.UnexpectedDeliveries {
		assertNoDelivery(t, unexpected, handles)
	}

	// Process response messages (round-trip flow).
	for _, resp := range msg.ResponseMessages {
		publishAndAssertResponse(t, resp, services, handles, mapper)
	}
}

func findPublisher(t spectest.T, svcName string, svcCfg spectest.ServiceConfig, h *spectest.ServiceHandle) spectest.PublishFunc {
	t.Helper()
	if len(h.Publishers) == 1 {
		for _, p := range h.Publishers {
			return p
		}
	}
	for _, intent := range svcCfg.Setups {
		if intent.Direction == "publish" {
			pk := spectest.PublisherKey(intent)
			if p, exists := h.Publishers[pk]; exists {
				return p
			}
		}
	}
	t.Fatalf("no publisher found for service %q", svcName)
	return nil
}

func assertDelivery(t spectest.T, delivery spectest.ExpectedDelivery, handles map[string]*spectest.ServiceHandle, mapper *NameMapper, nonce string, matchedIndices map[string]map[int]bool) {
	t.Helper()

	th, ok := handles[delivery.To]
	spectest.RequireTrue(t, ok, "target service %q not found", delivery.To)

	if matchedIndices[delivery.To] == nil {
		matchedIndices[delivery.To] = make(map[int]bool)
	}
	excluded := matchedIndices[delivery.To]

	// Wait for delivery with matching nonce that hasn't been matched yet.
	spectest.AssertEventually(t, func() bool {
		for i, rm := range th.Received() {
			if excluded[i] {
				continue
			}
			if rm.Metadata.Type == delivery.Metadata.Type && hasNonce(rm.Payload, nonce) {
				return true
			}
		}
		return false
	}, 5*time.Second, 50*time.Millisecond,
		"expected delivery to %s with type %s", delivery.To, delivery.Metadata.Type)

	var matched *spectest.ReceivedMessage
	var matchedIdx int
	for i, rm := range th.Received() {
		if excluded[i] {
			continue
		}
		if rm.Metadata.Type == delivery.Metadata.Type && hasNonce(rm.Payload, nonce) {
			matched = &rm
			matchedIdx = i
			break
		}
	}
	if matched == nil {
		return
	}
	excluded[matchedIdx] = true

	// Assert metadata with name mapping.
	if delivery.Metadata.Type != "" {
		spectest.AssertEqual(t, delivery.Metadata.Type, matched.Metadata.Type,
			"metadata.type mismatch for delivery to %s", delivery.To)
	}
	if delivery.Metadata.Source != "" {
		expectedSource := mapper.Runtime(delivery.Metadata.Source)
		spectest.AssertEqual(t, expectedSource, matched.Metadata.Source,
			"metadata.source mismatch for delivery to %s", delivery.To)
	}
	if delivery.Metadata.SpecVersion != "" {
		spectest.AssertEqual(t, delivery.Metadata.SpecVersion, matched.Metadata.SpecVersion,
			"metadata.specVersion mismatch for delivery to %s", delivery.To)
	}
	if delivery.Metadata.DataContentType != "" {
		spectest.AssertEqual(t, delivery.Metadata.DataContentType, matched.Metadata.DataContentType,
			"metadata.dataContentType mismatch for delivery to %s", delivery.To)
	}

	// Assert original payload fields.
	if len(delivery.PayloadMatch) > 0 {
		assertPayloadMatch(t, delivery.To, matched.Payload, delivery.PayloadMatch)
	}

	// Assert custom headers.
	if len(delivery.HeaderMatch) > 0 {
		assertHeaderMatch(t, delivery.To, matched.Info.Headers, delivery.HeaderMatch)
	}

	// Assert nonce.
	assertNonce(t, delivery.To, matched.Payload, nonce)
}

func assertNoDelivery(t spectest.T, unexpected spectest.UnexpectedDelivery, handles map[string]*spectest.ServiceHandle) {
	t.Helper()

	th, ok := handles[unexpected.To]
	spectest.RequireTrue(t, ok, "target service %q not found for unexpected delivery", unexpected.To)

	// Expected deliveries have already been asserted (with up to 5s polling),
	// so messages have had plenty of time to arrive.
	time.Sleep(200 * time.Millisecond)

	for _, rm := range th.Received() {
		spectest.AssertNotEqual(t, unexpected.MetadataType, rm.Metadata.Type,
			"unexpected delivery to %s: message with type %s should not have been received",
			unexpected.To, unexpected.MetadataType)
	}
}

func assertPayloadMatch(t spectest.T, target string, actual, expected json.RawMessage) {
	t.Helper()
	var actualMap, expectedMap map[string]any
	spectest.RequireNoError(t, json.Unmarshal(actual, &actualMap), "failed to unmarshal actual payload for %s", target)
	spectest.RequireNoError(t, json.Unmarshal(expected, &expectedMap), "failed to unmarshal expected payload for %s", target)
	for k, v := range expectedMap {
		spectest.AssertEqual(t, v, actualMap[k], "payload field %q mismatch for delivery to %s", k, target)
	}
}

func publishAndAssertResponse(t spectest.T, resp spectest.ResponseMessage, services map[string]spectest.ServiceConfig, handles map[string]*spectest.ServiceHandle, mapper *NameMapper) {
	t.Helper()

	h, ok := handles[resp.From]
	spectest.RequireTrue(t, ok, "response service %q not found in handles", resp.From)
	spectest.RequireNotEmpty(t, h.Publishers, "response service %q has no publishers", resp.From)

	payload, nonce := InjectNonce(resp.Payload)

	pub := findPublisher(t, resp.From, services[resp.From], h)

	// Pass the runtime target service name so the adapter knows where to
	// route the response (e.g., AMQP headers exchange routing).
	var headers map[string]string
	if resp.TargetService != "" {
		headers = map[string]string{
			"_tckTargetService": mapper.Runtime(resp.TargetService),
		}
	}

	err := pub(context.Background(), resp.RoutingKey, payload, headers)
	spectest.RequireNoError(t, err, "response publish failed for %s/%s", resp.From, resp.RoutingKey)

	matchedIndices := make(map[string]map[int]bool)
	for _, delivery := range resp.ExpectedDeliveries {
		assertDelivery(t, delivery, handles, mapper, nonce, matchedIndices)
	}
}

func assertHeaderMatch(t spectest.T, target string, headers map[string]any, expected map[string]string) {
	t.Helper()
	for k, v := range expected {
		actual, ok := headers[k]
		spectest.RequireTrue(t, ok, "header %q not found in delivery to %s", k, target)
		spectest.AssertEqual(t, v, fmt.Sprintf("%v", actual),
			"header %q mismatch for delivery to %s", k, target)
	}
}

func assertNonce(t spectest.T, target string, payload json.RawMessage, expectedNonce string) {
	t.Helper()
	var obj map[string]any
	spectest.RequireNoError(t, json.Unmarshal(payload, &obj), "failed to unmarshal payload for nonce check on %s", target)
	actualNonce, ok := obj["_tckNonce"]
	spectest.RequireTrue(t, ok, "payload missing _tckNonce field for delivery to %s", target)
	spectest.AssertEqual(t, expectedNonce, actualNonce, "_tckNonce mismatch for delivery to %s", target)
}

// --- Probe handlers ---

func runOutboundProbe(t spectest.T, probe ProbeMessage, target spectest.ProbeTarget, handles map[string]*spectest.ServiceHandle, services map[string]spectest.ServiceConfig, broker BrokerClient, mapper *NameMapper) {
	t.Helper()

	// Set up raw consumer before publishing so it is ready to receive.
	consumer := broker.CreateProbeConsumer(t, target)
	t.Cleanup(consumer.Close)

	// Inject nonce.
	payload, nonce := InjectNonce(probe.Payload)

	// Publish via the implementation.
	h, ok := handles[probe.PublishVia]
	spectest.RequireTrue(t, ok, "outbound probe: service %q not in handles", probe.PublishVia)
	pub := findPublisher(t, probe.PublishVia, services[probe.PublishVia], h)
	err := pub(context.Background(), probe.RoutingKey, payload, nil)
	spectest.RequireNoError(t, err, "outbound probe: publish failed for %s/%s", probe.PublishVia, probe.RoutingKey)

	// Read raw message from broker.
	raw := consumer.Receive(probeTimeout)
	spectest.RequireNotNil(t, raw, "outbound probe: no raw message received from broker for %s", probe.RoutingKey)

	// Assert CE attributes with name mapping.
	for attr, expected := range probe.CEAttributes {
		expectedVal := expected
		if attr == "source" {
			expectedVal = mapper.Runtime(expected)
		}
		spectest.AssertEqual(t, expectedVal, raw.Headers[attr],
			"outbound probe: CE attribute %q mismatch", attr)
	}

	// Assert original payload fields.
	if len(probe.PayloadMatch) > 0 {
		assertPayloadMatch(t, "outbound-probe", raw.Payload, probe.PayloadMatch)
	}

	// Assert nonce.
	assertNonce(t, "outbound-probe", raw.Payload, nonce)
}

func runInboundProbe(t spectest.T, probe ProbeMessage, target spectest.ProbeTarget, handles map[string]*spectest.ServiceHandle, broker BrokerClient, mapper *NameMapper) {
	t.Helper()

	// Inject nonce.
	payload, nonce := InjectNonce(probe.Payload)

	// Publish raw message to broker.
	err := broker.PublishRaw(t, target, payload, probe.CEAttributes)
	spectest.RequireNoError(t, err, "inbound probe: raw publish failed")

	// Wait for the implementation's consumer to receive it.
	h, ok := handles[probe.ExpectReceivedBy]
	spectest.RequireTrue(t, ok, "inbound probe: service %q not in handles", probe.ExpectReceivedBy)

	// Match by ce-source since inbound probes use a unique source ("tck-probe").
	expectedSource := probe.CEAttributes["source"]
	spectest.RequireNotEmpty(t, expectedSource, "inbound probe must specify ceAttributes.source")

	spectest.AssertEventually(t, func() bool {
		for _, rm := range h.Received() {
			if rm.Metadata.Source == expectedSource && hasNonce(rm.Payload, nonce) {
				return true
			}
		}
		return false
	}, 5*time.Second, 50*time.Millisecond,
		"inbound probe: expected delivery to %s with source %s", probe.ExpectReceivedBy, expectedSource)

	var matched *spectest.ReceivedMessage
	for _, rm := range h.Received() {
		if rm.Metadata.Source == expectedSource && hasNonce(rm.Payload, nonce) {
			matched = &rm
			break
		}
	}
	if matched == nil {
		return
	}

	// Assert metadata.
	if v, ok := probe.CEAttributes["type"]; ok {
		spectest.AssertEqual(t, v, matched.Metadata.Type, "inbound probe: metadata.type mismatch")
	}
	if v, ok := probe.CEAttributes["specversion"]; ok {
		spectest.AssertEqual(t, v, matched.Metadata.SpecVersion, "inbound probe: metadata.specVersion mismatch")
	}

	// Assert original payload fields.
	if len(probe.PayloadMatch) > 0 {
		assertPayloadMatch(t, "inbound-probe", matched.Payload, probe.PayloadMatch)
	}

	// Assert nonce.
	assertNonce(t, "inbound-probe", matched.Payload, nonce)
}

// hasNonce checks if a payload contains the given nonce value.
func hasNonce(payload json.RawMessage, nonce string) bool {
	var obj map[string]any
	if err := json.Unmarshal(payload, &obj); err != nil {
		return false
	}
	v, ok := obj["_tckNonce"]
	if !ok {
		return false
	}
	return v == nonce
}
