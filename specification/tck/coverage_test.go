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
	"testing"

	"github.com/sparetimecoders/gomessaging/spec/spectest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAllSupportedIntents(t *testing.T) {
	intents := AllSupportedIntents()
	assert.Len(t, intents, 9)

	// Verify no duplicates.
	seen := make(map[IntentKey]bool)
	for _, k := range intents {
		assert.False(t, seen[k], "duplicate intent: %+v", k)
		seen[k] = true
	}
}

func TestComputeCoverageMatrixFullCoverage(t *testing.T) {
	scenarios, err := LoadScenariosFile("../spec/testdata/tck.json")
	require.NoError(t, err)

	matrix := ComputeCoverageMatrix(scenarios)
	assert.Empty(t, matrix.Missing, "expected all intents covered, missing: %v", matrix.Missing)
	assert.Len(t, matrix.Entries, 9)

	// Every entry should have at least one scenario.
	for _, entry := range matrix.Entries {
		assert.NotEmpty(t, entry.Scenarios, "intent %+v has no scenarios", entry.IntentKey)
	}
}

func TestComputeCoverageMatrixPartialCoverage(t *testing.T) {
	// Only event-stream publish + consume (non-ephemeral).
	scenarios := []Scenario{
		{
			Name: "basic",
			Services: map[string]spectest.ServiceConfig{
				"pub": {Setups: []spectest.SetupIntent{{Pattern: "event-stream", Direction: "publish"}}},
				"sub": {Setups: []spectest.SetupIntent{{Pattern: "event-stream", Direction: "consume"}}},
			},
		},
	}

	matrix := ComputeCoverageMatrix(scenarios)
	require.Len(t, matrix.Missing, 7)

	// event-stream publish and consume are covered.
	for _, entry := range matrix.Entries {
		if entry.Pattern == "event-stream" && entry.Direction == "publish" {
			assert.Equal(t, []string{"basic"}, entry.Scenarios)
		}
		if entry.Pattern == "event-stream" && entry.Direction == "consume" && !entry.Ephemeral {
			assert.Equal(t, []string{"basic"}, entry.Scenarios)
		}
	}
}

func TestComputeCoverageMatrixMultipleScenariosSameIntent(t *testing.T) {
	scenarios := []Scenario{
		{
			Name: "scenario-a",
			Services: map[string]spectest.ServiceConfig{
				"pub": {Setups: []spectest.SetupIntent{{Pattern: "event-stream", Direction: "publish"}}},
			},
		},
		{
			Name: "scenario-b",
			Services: map[string]spectest.ServiceConfig{
				"pub": {Setups: []spectest.SetupIntent{{Pattern: "event-stream", Direction: "publish"}}},
			},
		},
	}

	matrix := ComputeCoverageMatrix(scenarios)

	for _, entry := range matrix.Entries {
		if entry.Pattern == "event-stream" && entry.Direction == "publish" {
			assert.Equal(t, []string{"scenario-a", "scenario-b"}, entry.Scenarios)
		}
	}
}
