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
	adapterPath := flag.String("adapter", "", "path to adapter binary (required)")
	fixturesPath := flag.String("fixtures", "", "path to tck.json fixture file (required)")
	scenarioFilter := flag.String("scenario", "", "run only scenarios matching substring")
	verbose := flag.Bool("v", false, "verbose logging")
	flag.Parse()

	if *adapterPath == "" || *fixturesPath == "" {
		fmt.Fprintln(os.Stderr, "usage: tck-runner -adapter PATH -fixtures PATH [-scenario NAME] [-v]")
		flag.PrintDefaults()
		os.Exit(2)
	}

	scenarios, err := tck.LoadScenariosFile(*fixturesPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	root := newCLIRunner("TCK", *verbose)
	defer root.RunCleanups()

	adapter := tck.NewSubprocessAdapter(root, *adapterPath)

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
