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

// Binary tck-runner is the standalone Transport Conformance Kit runner.
// It spawns an adapter binary via the TCK subprocess protocol and runs
// all scenarios from a tck.json fixture file.
//
// Usage:
//
//	tck-runner -adapter ./my-adapter -fixtures ./tck.json [-scenario NAME] [-v]
//	tck-runner --check-coverage -fixtures ./tck.json
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/sparetimecoders/gomessaging/spec/spectest"
	"github.com/sparetimecoders/gomessaging/tck"
)

func main() {
	adapterPath := flag.String("adapter", "", "path to adapter binary (required unless --check-coverage)")
	fixturesPath := flag.String("fixtures", "", "path to tck.json fixture file (required)")
	scenarioFilter := flag.String("scenario", "", "run only scenarios matching substring")
	verbose := flag.Bool("v", false, "verbose logging")
	reportJSON := flag.String("report-json", "", "write JSON report to file (- for stdout)")
	reportMD := flag.String("report-md", "", "write Markdown report to file (- for stdout)")
	checkCoverage := flag.Bool("check-coverage", false, "check coverage matrix and exit (no adapter needed)")
	flag.Parse()

	if *fixturesPath == "" {
		fmt.Fprintln(os.Stderr, "usage: tck-runner -adapter PATH -fixtures PATH [-scenario NAME] [-v]")
		fmt.Fprintln(os.Stderr, "       tck-runner --check-coverage -fixtures PATH")
		flag.PrintDefaults()
		os.Exit(2)
	}

	scenarios, err := tck.LoadScenariosFile(*fixturesPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// --check-coverage: compute and print coverage matrix, then exit.
	if *checkCoverage {
		runCheckCoverage(scenarios)
		return
	}

	if *adapterPath == "" {
		fmt.Fprintln(os.Stderr, "error: -adapter is required when not using --check-coverage")
		os.Exit(2)
	}

	root := newCLIRunner("TCK", *verbose)
	defer root.RunCleanups()

	adapter := tck.NewSubprocessAdapter(root, *adapterPath)

	wantReport := *reportJSON != "" || *reportMD != ""

	if wantReport {
		runWithReport(root, adapter, *fixturesPath, *scenarioFilter, *reportJSON, *reportMD)
		return
	}

	// Default: run scenarios without report generation.
	var passed, failed int
	for _, scenario := range scenarios {
		if *scenarioFilter != "" && !strings.Contains(scenario.Name, *scenarioFilter) {
			continue
		}

		ok := root.Run(scenario.Name, func(t spectest.T) {
			tck.RunScenario(t, adapter, scenario)
		})
		if ok {
			passed++
		} else {
			failed++
		}
	}

	total := passed + failed
	fmt.Fprintln(os.Stderr)
	if failed > 0 {
		fmt.Fprintf(os.Stderr, "FAIL (%d/%d scenarios)\n", passed, total)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "PASS (%d/%d scenarios)\n", passed, total)
}

func runCheckCoverage(scenarios []tck.Scenario) {
	matrix := tck.ComputeCoverageMatrix(scenarios)

	fmt.Println("Coverage Matrix")
	fmt.Println("===============")
	fmt.Printf("%-20s %-12s %-12s %s\n", "Pattern", "Direction", "Ephemeral", "Covered By")
	fmt.Println(strings.Repeat("-", 80))

	for _, entry := range matrix.Entries {
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
		fmt.Printf("%-20s %-12s %-12s %s\n", entry.Pattern, entry.Direction, ephStr, coveredBy)
	}

	fmt.Println()
	if len(matrix.Missing) == 0 {
		fmt.Printf("All %d intent combinations covered.\n", len(matrix.Entries))
	} else {
		fmt.Printf("%d intent combination(s) missing coverage:\n", len(matrix.Missing))
		for _, m := range matrix.Missing {
			fmt.Printf("  - %s / %s / ephemeral=%v\n", m.Pattern, m.Direction, m.Ephemeral)
		}
		os.Exit(1)
	}
}

func runWithReport(root *cliRunner, adapter tck.Adapter, fixturesPath, scenarioFilter, jsonPath, mdPath string) {
	report := tck.RunTCKWithReport(root, fixturesPath, adapter)

	if jsonPath != "" {
		data, err := report.GenerateJSON()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error generating JSON report: %v\n", err)
			os.Exit(1)
		}
		if err := writeOutput(jsonPath, data); err != nil {
			fmt.Fprintf(os.Stderr, "error writing JSON report: %v\n", err)
			os.Exit(1)
		}
	}

	if mdPath != "" {
		md := report.GenerateMarkdown()
		if err := writeOutput(mdPath, []byte(md)); err != nil {
			fmt.Fprintf(os.Stderr, "error writing Markdown report: %v\n", err)
			os.Exit(1)
		}
	}

	fmt.Fprintln(os.Stderr)
	if report.Summary.Failed > 0 {
		fmt.Fprintf(os.Stderr, "FAIL (%d/%d scenarios)\n", report.Summary.Passed, report.Summary.Total)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "PASS (%d/%d scenarios)\n", report.Summary.Passed, report.Summary.Total)
}

func writeOutput(path string, data []byte) error {
	if path == "-" {
		_, err := os.Stdout.Write(data)
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
