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

// IntentKey identifies a unique pattern+direction+ephemeral combination.
type IntentKey struct {
	Pattern   string
	Direction string
	Ephemeral bool
}

// CoverageEntry records which scenarios exercise a particular intent.
type CoverageEntry struct {
	IntentKey
	Scenarios []string
}

// CoverageMatrix holds the full coverage analysis.
type CoverageMatrix struct {
	Entries []CoverageEntry
	Missing []IntentKey
}

// AllSupportedIntents returns the 11 known intent combinations that the TCK
// should exercise.
func AllSupportedIntents() []IntentKey {
	return []IntentKey{
		{Pattern: "event-stream", Direction: "publish"},
		{Pattern: "event-stream", Direction: "consume", Ephemeral: false},
		{Pattern: "event-stream", Direction: "consume", Ephemeral: true},
		{Pattern: "custom-stream", Direction: "publish"},
		{Pattern: "custom-stream", Direction: "consume", Ephemeral: false},
		{Pattern: "custom-stream", Direction: "consume", Ephemeral: true},
		{Pattern: "service-request", Direction: "publish"},
		{Pattern: "service-request", Direction: "consume"},
		{Pattern: "service-response", Direction: "publish"},
		{Pattern: "service-response", Direction: "consume"},
		{Pattern: "queue-publish", Direction: "publish"},
	}
}

// ComputeCoverageMatrix scans all scenario intents and reports which of the
// 11 supported combinations are covered and which are missing.
func ComputeCoverageMatrix(scenarios []Scenario) CoverageMatrix {
	// Build a map from IntentKey to list of scenario names.
	covered := make(map[IntentKey][]string)
	for _, s := range scenarios {
		seen := make(map[IntentKey]bool)
		for _, svc := range s.Services {
			for _, intent := range svc.Setups {
				key := IntentKey{
					Pattern:   intent.Pattern,
					Direction: intent.Direction,
					Ephemeral: intent.Ephemeral,
				}
				if !seen[key] {
					covered[key] = append(covered[key], s.Name)
					seen[key] = true
				}
			}
		}
	}

	all := AllSupportedIntents()
	entries := make([]CoverageEntry, 0, len(all))
	var missing []IntentKey

	for _, key := range all {
		scenarios, ok := covered[key]
		if ok {
			entries = append(entries, CoverageEntry{IntentKey: key, Scenarios: scenarios})
		} else {
			entries = append(entries, CoverageEntry{IntentKey: key})
			missing = append(missing, key)
		}
	}

	return CoverageMatrix{Entries: entries, Missing: missing}
}
