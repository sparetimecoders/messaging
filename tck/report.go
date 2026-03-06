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
	"fmt"
	"strings"
	"time"

	"github.com/sparetimecoders/messaging/spectest"
)

// PhaseResult records the outcome of a single TCK phase.
type PhaseResult struct {
	Name     string        `json:"name"`
	Passed   bool          `json:"passed"`
	Duration time.Duration `json:"duration"`
	Errors   []string      `json:"errors,omitempty"`
}

// ScenarioReport records the outcome of a single TCK scenario.
type ScenarioReport struct {
	Name     string        `json:"name"`
	Passed   bool          `json:"passed"`
	Duration time.Duration `json:"duration"`
	Services []string      `json:"services"`
	Patterns []string      `json:"patterns"`
	Phases   []PhaseResult `json:"phases"`
}

// ReportSummary holds aggregate counts.
type ReportSummary struct {
	Total  int `json:"total"`
	Passed int `json:"passed"`
	Failed int `json:"failed"`
}

// TCKReport is the top-level conformance report.
type TCKReport struct {
	TransportKey string           `json:"transportKey"`
	Timestamp    time.Time        `json:"timestamp"`
	Scenarios    []ScenarioReport `json:"scenarios"`
	Coverage     CoverageMatrix   `json:"coverage"`
	Summary      ReportSummary    `json:"summary"`
}

// GenerateJSON marshals the report to indented JSON.
func (r *TCKReport) GenerateJSON() ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}

// GenerateMarkdown renders the report as a Markdown document.
func (r *TCKReport) GenerateMarkdown() string {
	var b strings.Builder

	result := "PASS"
	if r.Summary.Failed > 0 {
		result = "FAIL"
	}

	fmt.Fprintf(&b, "# TCK Conformance Report: %s\n\n", r.TransportKey)
	fmt.Fprintf(&b, "**Generated:** %s\n", r.Timestamp.UTC().Format(time.RFC3339))
	fmt.Fprintf(&b, "**Result:** %s (%d/%d scenarios)\n\n", result, r.Summary.Passed, r.Summary.Total)

	// Scenarios table.
	b.WriteString("## Scenarios\n\n")
	b.WriteString("| # | Scenario | Result | Duration | Setup | Topology | Broker | Delivery | Probes |\n")
	b.WriteString("|---|----------|--------|----------|-------|----------|--------|----------|--------|\n")

	for i, s := range r.Scenarios {
		sResult := "PASS"
		if !s.Passed {
			sResult = "FAIL"
		}
		phases := make([]string, 5)
		phaseNames := []string{"setup", "topology", "broker", "delivery", "probes"}
		for j, name := range phaseNames {
			phases[j] = "-"
			for _, p := range s.Phases {
				if p.Name == name {
					if p.Passed {
						phases[j] = "PASS"
					} else {
						phases[j] = "FAIL"
					}
					break
				}
			}
		}
		fmt.Fprintf(&b, "| %d | %s | %s | %.2fs | %s | %s | %s | %s | %s |\n",
			i+1, s.Name, sResult, s.Duration.Seconds(),
			phases[0], phases[1], phases[2], phases[3], phases[4])
	}

	// Coverage matrix.
	b.WriteString("\n## Coverage Matrix\n\n")
	b.WriteString("| Pattern | Direction | Ephemeral | Covered By |\n")
	b.WriteString("|---------|-----------|-----------|------------|\n")

	for _, entry := range r.Coverage.Entries {
		ephStr := "-"
		if entry.Direction == "consume" && entry.Pattern != "service-request" && entry.Pattern != "service-response" {
			if entry.Ephemeral {
				ephStr = "yes"
			} else {
				ephStr = "no"
			}
		}

		coveredBy := "-"
		if len(entry.Scenarios) > 0 {
			coveredBy = strings.Join(entry.Scenarios, ", ")
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s |\n",
			entry.Pattern, entry.Direction, ephStr, coveredBy)
	}

	b.WriteString("\n")
	totalIntents := len(r.Coverage.Entries)
	if len(r.Coverage.Missing) == 0 {
		fmt.Fprintf(&b, "**All %d intent combinations covered.**\n", totalIntents)
	} else {
		fmt.Fprintf(&b, "**%d intent combination(s) missing coverage.**\n", len(r.Coverage.Missing))
	}

	return b.String()
}

// reportingT wraps a spectest.T to intercept errors for report generation.
type reportingT struct {
	spectest.T
	errors []string
	fatal  bool
}

func newReportingT(inner spectest.T) *reportingT {
	return &reportingT{T: inner}
}

func (r *reportingT) Errorf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	r.errors = append(r.errors, msg)
	r.T.Errorf(format, args...)
}

func (r *reportingT) Fatalf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	r.errors = append(r.errors, msg)
	r.fatal = true
	r.T.Fatalf(format, args...)
}

func (r *reportingT) Fatal(args ...any) {
	msg := fmt.Sprint(args...)
	r.errors = append(r.errors, msg)
	r.fatal = true
	r.T.Fatal(args...)
}

func (r *reportingT) FailNow() {
	r.fatal = true
	r.T.FailNow()
}

// snapshot returns errors collected since the last reset and clears the buffer.
func (r *reportingT) snapshot() []string {
	errs := r.errors
	r.errors = nil
	return errs
}

// hasFatal returns whether a fatal error was recorded since the last reset.
func (r *reportingT) hasFatal() bool {
	f := r.fatal
	r.fatal = false
	return f
}

// scenarioPatterns returns unique patterns from a scenario's service configs.
func scenarioPatterns(services map[string]spectest.ServiceConfig) []string {
	seen := make(map[string]bool)
	var patterns []string
	for _, svc := range services {
		for _, intent := range svc.Setups {
			if !seen[intent.Pattern] {
				seen[intent.Pattern] = true
				patterns = append(patterns, intent.Pattern)
			}
		}
	}
	return patterns
}

// scenarioServiceNames returns sorted template service names.
func scenarioServiceNames(services map[string]spectest.ServiceConfig) []string {
	names := make([]string, 0, len(services))
	for name := range services {
		names = append(names, name)
	}
	return names
}
