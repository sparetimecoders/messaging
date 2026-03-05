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

import "github.com/sparetimecoders/messaging/specification/spec/spectest"

// IntentKey uniquely identifies a messaging intent by pattern, direction, and
// whether the consumer is ephemeral.
type IntentKey struct {
	Pattern   string `json:"pattern"`
	Direction string `json:"direction"`
	Ephemeral bool   `json:"ephemeral,omitempty"`
}

// CoverageEntry records which scenarios exercise a particular intent.
type CoverageEntry struct {
	IntentKey
	Scenarios []string `json:"scenarios,omitempty"`
}

// CoverageMatrix summarises which intents are covered by the scenario suite.
type CoverageMatrix struct {
	Entries []CoverageEntry `json:"entries"`
	Missing []IntentKey     `json:"missing,omitempty"`
}

// AllSupportedIntents returns every intent combination the TCK tracks.
// Ephemeral variants only apply to consume-side event-stream and custom-stream.
func AllSupportedIntents() []IntentKey {
	return []IntentKey{
		{Pattern: "event-stream", Direction: "publish"},
		{Pattern: "event-stream", Direction: "consume"},
		{Pattern: "event-stream", Direction: "consume", Ephemeral: true},
		{Pattern: "custom-stream", Direction: "publish"},
		{Pattern: "custom-stream", Direction: "consume"},
		{Pattern: "custom-stream", Direction: "consume", Ephemeral: true},
		{Pattern: "service-request", Direction: "publish"},
		{Pattern: "service-request", Direction: "consume"},
		{Pattern: "service-response", Direction: "publish"},
		{Pattern: "service-response", Direction: "consume"},
		{Pattern: "queue-publish", Direction: "publish"},
	}
}

// ComputeCoverageMatrix builds a coverage matrix from the given scenarios.
func ComputeCoverageMatrix(scenarios []Scenario) CoverageMatrix {
	allIntents := AllSupportedIntents()

	covered := make(map[IntentKey][]string)
	for _, sc := range scenarios {
		for _, svc := range sc.Services {
			for _, intent := range svc.Setups {
				key := intentKeyFromSetup(intent)
				covered[key] = append(covered[key], sc.Name)
			}
		}
	}

	var entries []CoverageEntry
	var missing []IntentKey
	for _, k := range allIntents {
		entry := CoverageEntry{IntentKey: k, Scenarios: covered[k]}
		entries = append(entries, entry)
		if len(covered[k]) == 0 {
			missing = append(missing, k)
		}
	}

	return CoverageMatrix{Entries: entries, Missing: missing}
}

func intentKeyFromSetup(intent spectest.SetupIntent) IntentKey {
	return IntentKey{
		Pattern:   intent.Pattern,
		Direction: intent.Direction,
		Ephemeral: intent.Ephemeral,
	}
}
