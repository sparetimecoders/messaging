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
	"strings"
	"testing"

	"github.com/sparetimecoders/gomessaging/spec"
)

// SetupIntent describes a transport-agnostic setup action from the fixture file.
type SetupIntent struct {
	Pattern       string `json:"pattern"`
	Direction     string `json:"direction"`
	RoutingKey    string `json:"routingKey,omitempty"`
	Exchange      string `json:"exchange,omitempty"`
	TargetService string `json:"targetService,omitempty"`
	Ephemeral     bool   `json:"ephemeral,omitempty"`
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

// Scenario represents a single topology conformance test scenario.
type Scenario struct {
	Name              string                        `json:"name"`
	ServiceName       string                        `json:"serviceName"`
	Setups            []SetupIntent                 `json:"setups"`
	ExpectedEndpoints map[string][]ExpectedEndpoint `json:"expectedEndpoints"`
}

type fixtureFile struct {
	Scenarios []Scenario `json:"scenarios"`
}

// LoadScenarios loads topology conformance scenarios from the shared fixture file.
func LoadScenarios(t *testing.T, fixturePath string) []Scenario {
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
func AssertTopology(t *testing.T, expected []ExpectedEndpoint, actual spec.Topology) {
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
func RequireEndpointMatch(t *testing.T, expected ExpectedEndpoint, actuals []spec.Endpoint) {
	t.Helper()
	for _, actual := range actuals {
		if spec.EndpointDirection(expected.Direction) != actual.Direction ||
			spec.Pattern(expected.Pattern) != actual.Pattern ||
			expected.ExchangeName != actual.ExchangeName ||
			expected.RoutingKey != actual.RoutingKey {
			continue
		}
		if spec.ExchangeKind(expected.ExchangeKind) != actual.ExchangeKind {
			t.Errorf("exchangeKind mismatch: got %q, want %q", actual.ExchangeKind, expected.ExchangeKind)
		}
		if expected.Ephemeral != actual.Ephemeral {
			t.Errorf("ephemeral mismatch: got %v, want %v", actual.Ephemeral, expected.Ephemeral)
		}
		if expected.QueueName != "" {
			if expected.QueueName != actual.QueueName {
				t.Errorf("queueName mismatch: got %q, want %q", actual.QueueName, expected.QueueName)
			}
		} else if expected.QueueNamePrefix != "" {
			if !strings.HasPrefix(actual.QueueName, expected.QueueNamePrefix) {
				t.Errorf("queueName %q should have prefix %q", actual.QueueName, expected.QueueNamePrefix)
			}
		}
		return
	}
	t.Errorf("no matching endpoint found: direction=%s pattern=%s exchange=%s routingKey=%s",
		expected.Direction, expected.Pattern, expected.ExchangeName, expected.RoutingKey)
}
