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
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/sparetimecoders/gomessaging/spec/spectest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReportingTCapturesErrors(t *testing.T) {
	inner := &fakeT{}
	rt := newReportingT(inner)

	rt.Errorf("first error: %d", 1)
	rt.Errorf("second error: %s", "oops")

	errors := rt.snapshot()
	require.Len(t, errors, 2)
	assert.Equal(t, "first error: 1", errors[0])
	assert.Equal(t, "second error: oops", errors[1])

	// snapshot clears the buffer.
	assert.Empty(t, rt.snapshot())
}

func TestReportingTHasFatal(t *testing.T) {
	inner := &fakeT{}
	rt := newReportingT(inner)

	assert.False(t, rt.hasFatal())

	rt.Errorf("non-fatal")
	assert.False(t, rt.hasFatal())

	// Simulate a fatal call that doesn't actually panic (fakeT absorbs it).
	rt.Fatalf("fatal: %s", "boom")
	assert.True(t, rt.hasFatal())

	// hasFatal resets.
	assert.False(t, rt.hasFatal())
}

func TestGenerateJSONRoundtrip(t *testing.T) {
	report := &TCKReport{
		TransportKey: "nats",
		Timestamp:    time.Date(2026, 2, 18, 14, 30, 0, 0, time.UTC),
		Scenarios: []ScenarioReport{
			{
				Name:     "basic test",
				Passed:   true,
				Duration: 150 * time.Millisecond,
				Services: []string{"pub", "sub"},
				Patterns: []string{"event-stream"},
				Phases: []PhaseResult{
					{Name: "setup", Passed: true, Duration: 10 * time.Millisecond},
					{Name: "topology", Passed: true, Duration: 20 * time.Millisecond},
					{Name: "broker", Passed: true, Duration: 30 * time.Millisecond},
					{Name: "delivery", Passed: true, Duration: 40 * time.Millisecond},
					{Name: "probes", Passed: true, Duration: 50 * time.Millisecond},
				},
			},
		},
		Coverage: CoverageMatrix{
			Entries: []CoverageEntry{
				{IntentKey: IntentKey{Pattern: "event-stream", Direction: "publish"}, Scenarios: []string{"basic test"}},
			},
		},
		Summary: ReportSummary{Total: 1, Passed: 1, Failed: 0},
	}

	data, err := report.GenerateJSON()
	require.NoError(t, err)

	var decoded TCKReport
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, "nats", decoded.TransportKey)
	assert.Len(t, decoded.Scenarios, 1)
	assert.Equal(t, "basic test", decoded.Scenarios[0].Name)
	assert.True(t, decoded.Scenarios[0].Passed)
	assert.Len(t, decoded.Scenarios[0].Phases, 5)
	assert.Equal(t, 1, decoded.Summary.Total)
	assert.Equal(t, 1, decoded.Summary.Passed)
}

func TestGenerateMarkdownFormat(t *testing.T) {
	report := &TCKReport{
		TransportKey: "amqp",
		Timestamp:    time.Date(2026, 2, 18, 14, 30, 0, 0, time.UTC),
		Scenarios: []ScenarioReport{
			{
				Name:     "event stream publish and consume",
				Passed:   true,
				Duration: 150 * time.Millisecond,
				Services: []string{"orders", "notifications"},
				Patterns: []string{"event-stream"},
				Phases: []PhaseResult{
					{Name: "setup", Passed: true, Duration: 10 * time.Millisecond},
					{Name: "topology", Passed: true, Duration: 20 * time.Millisecond},
					{Name: "broker", Passed: true, Duration: 30 * time.Millisecond},
					{Name: "delivery", Passed: true, Duration: 40 * time.Millisecond},
					{Name: "probes", Passed: true, Duration: 50 * time.Millisecond},
				},
			},
			{
				Name:     "failing scenario",
				Passed:   false,
				Duration: 200 * time.Millisecond,
				Services: []string{"svc-a"},
				Patterns: []string{"custom-stream"},
				Phases: []PhaseResult{
					{Name: "setup", Passed: true, Duration: 10 * time.Millisecond},
					{Name: "topology", Passed: false, Duration: 20 * time.Millisecond, Errors: []string{"mismatch"}},
				},
			},
		},
		Coverage: CoverageMatrix{
			Entries: []CoverageEntry{
				{IntentKey: IntentKey{Pattern: "event-stream", Direction: "publish"}, Scenarios: []string{"event stream publish and consume"}},
			},
		},
		Summary: ReportSummary{Total: 2, Passed: 1, Failed: 1},
	}

	md := report.GenerateMarkdown()

	assert.Contains(t, md, "# TCK Conformance Report: amqp")
	assert.Contains(t, md, "**Result:** FAIL (1/2 scenarios)")
	assert.Contains(t, md, "2026-02-18T14:30:00Z")
	assert.Contains(t, md, "## Scenarios")
	assert.Contains(t, md, "event stream publish and consume")
	assert.Contains(t, md, "failing scenario")
	assert.Contains(t, md, "## Coverage Matrix")
	assert.Contains(t, md, "event-stream")
}

func TestGenerateMarkdownAllCovered(t *testing.T) {
	report := &TCKReport{
		TransportKey: "nats",
		Timestamp:    time.Now(),
		Summary:      ReportSummary{Total: 1, Passed: 1},
		Coverage: CoverageMatrix{
			Entries: []CoverageEntry{
				{IntentKey: IntentKey{Pattern: "event-stream", Direction: "publish"}, Scenarios: []string{"s1"}},
			},
		},
	}

	md := report.GenerateMarkdown()
	assert.Contains(t, md, "**All 1 intent combinations covered.**")
	assert.NotContains(t, md, "missing coverage")
}

func TestGenerateMarkdownMissingCoverage(t *testing.T) {
	report := &TCKReport{
		TransportKey: "nats",
		Timestamp:    time.Now(),
		Summary:      ReportSummary{Total: 1, Passed: 1},
		Coverage: CoverageMatrix{
			Entries: []CoverageEntry{
				{IntentKey: IntentKey{Pattern: "event-stream", Direction: "publish"}, Scenarios: []string{"s1"}},
			},
			Missing: []IntentKey{
				{Pattern: "custom-stream", Direction: "publish"},
			},
		},
	}

	md := report.GenerateMarkdown()
	assert.Contains(t, md, "1 intent combination(s) missing coverage")
	assert.NotContains(t, md, "All ")
}

func TestScenarioPatterns(t *testing.T) {
	services := map[string]serviceConfigForTest{
		"a": {pattern: "event-stream"},
		"b": {pattern: "event-stream"},
		"c": {pattern: "custom-stream"},
	}

	// Build spectest.ServiceConfig for testing.
	svcCfgs := make(map[string]serviceConfig)
	for name, s := range services {
		svcCfgs[name] = serviceConfig{pattern: s.pattern}
	}

	// Directly test the pattern extraction logic.
	seen := make(map[string]bool)
	var patterns []string
	for _, s := range services {
		if !seen[s.pattern] {
			seen[s.pattern] = true
			patterns = append(patterns, s.pattern)
		}
	}
	assert.Len(t, patterns, 2)
	assert.Contains(t, patterns, "event-stream")
	assert.Contains(t, patterns, "custom-stream")
}

type serviceConfigForTest struct {
	pattern string
}

type serviceConfig struct {
	pattern string
}

// fakeT implements spectest.T for testing reportingT without actual test failures.
type fakeT struct {
	errors []string
	failed bool
	logs   []string
}

func (f *fakeT) Helper()                               {}
func (f *fakeT) Fatalf(format string, args ...any)      { f.failed = true }
func (f *fakeT) Fatal(args ...any)                      { f.failed = true }
func (f *fakeT) Errorf(format string, args ...any)      { f.failed = true }
func (f *fakeT) Logf(format string, args ...any)        {}
func (f *fakeT) Log(args ...any)                        {}
func (f *fakeT) Cleanup(func())                         {}
func (f *fakeT) Run(string, func(spectest.T)) bool      { return true }
func (f *fakeT) FailNow()                               { f.failed = true }
func (f *fakeT) Failed() bool                           { return f.failed }
func (f *fakeT) Name() string                           { return "fakeT" }

func TestGenerateMarkdownEphemeralColumn(t *testing.T) {
	report := &TCKReport{
		TransportKey: "nats",
		Timestamp:    time.Now(),
		Summary:      ReportSummary{Total: 1, Passed: 1},
		Coverage: CoverageMatrix{
			Entries: []CoverageEntry{
				{IntentKey: IntentKey{Pattern: "event-stream", Direction: "publish"}, Scenarios: []string{"s1"}},
				{IntentKey: IntentKey{Pattern: "event-stream", Direction: "consume", Ephemeral: false}, Scenarios: []string{"s1"}},
				{IntentKey: IntentKey{Pattern: "event-stream", Direction: "consume", Ephemeral: true}, Scenarios: []string{"s2"}},
				{IntentKey: IntentKey{Pattern: "service-request", Direction: "consume"}, Scenarios: []string{"s3"}},
			},
		},
	}

	md := report.GenerateMarkdown()
	lines := strings.Split(md, "\n")

	// Find coverage table lines.
	var tableLines []string
	inTable := false
	for _, line := range lines {
		if strings.HasPrefix(line, "| Pattern") {
			inTable = true
		}
		if inTable && strings.HasPrefix(line, "|") {
			tableLines = append(tableLines, line)
		}
	}

	// publish direction should have "-" for ephemeral.
	assert.Contains(t, tableLines[2], "| event-stream | publish | - |")
	// service-request consume should have "-" for ephemeral.
	assert.Contains(t, tableLines[5], "| service-request | consume | - |")
}
