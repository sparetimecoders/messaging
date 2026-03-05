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

package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/sparetimecoders/messaging/specification/spec/spectest"
)

// failNowSentinel is the panic value used by FailNow/Fatalf to abort the
// current test function. It is recovered in Run().
type failNowSentinel struct{}

// cliRunner implements [spectest.T] for standalone (non-test) execution.
// It uses panic/recover for FailNow semantics and prints output that mirrors
// "go test -v".
type cliRunner struct {
	name     string
	depth    int
	failed   bool
	cleanups []func()
	verbose  bool
}

// Verify interface compliance.
var _ spectest.T = (*cliRunner)(nil)

func newCLIRunner(name string, verbose bool) *cliRunner {
	return &cliRunner{name: name, verbose: verbose}
}

func (r *cliRunner) Helper() {} // no-op outside testing

func (r *cliRunner) Fatalf(format string, args ...any) {
	r.logf(format, args...)
	r.failed = true
	panic(failNowSentinel{})
}

func (r *cliRunner) Fatal(args ...any) {
	r.logf("%s", fmt.Sprint(args...))
	r.failed = true
	panic(failNowSentinel{})
}

func (r *cliRunner) Errorf(format string, args ...any) {
	r.logf(format, args...)
	r.failed = true
}

func (r *cliRunner) Logf(format string, args ...any) {
	if r.verbose {
		r.logf(format, args...)
	}
}

func (r *cliRunner) Log(args ...any) {
	if r.verbose {
		r.logf("%s", fmt.Sprint(args...))
	}
}

func (r *cliRunner) Cleanup(fn func()) {
	r.cleanups = append(r.cleanups, fn)
}

func (r *cliRunner) FailNow() {
	r.failed = true
	panic(failNowSentinel{})
}

func (r *cliRunner) Failed() bool { return r.failed }

func (r *cliRunner) Name() string { return r.name }

func (r *cliRunner) Run(name string, f func(spectest.T)) bool {
	child := &cliRunner{
		name:    r.name + "/" + name,
		depth:   r.depth + 1,
		verbose: r.verbose,
	}

	fmt.Fprintf(os.Stderr, "%s=== RUN   %s\n", indent(r.depth), name)

	start := time.Now()

	// Run the test function, catching failNow panics.
	func() {
		defer func() {
			if p := recover(); p != nil {
				if _, ok := p.(failNowSentinel); !ok {
					panic(p) // re-panic non-sentinel
				}
				// child.failed already set by Fatalf/FailNow
			}
		}()
		f(child)
	}()

	// Run cleanups in LIFO order.
	for i := len(child.cleanups) - 1; i >= 0; i-- {
		child.cleanups[i]()
	}

	elapsed := time.Since(start)
	if child.failed {
		r.failed = true
		fmt.Fprintf(os.Stderr, "%s--- FAIL: %s (%.2fs)\n", indent(r.depth), name, elapsed.Seconds())
	} else {
		fmt.Fprintf(os.Stderr, "%s--- PASS: %s (%.2fs)\n", indent(r.depth), name, elapsed.Seconds())
	}

	return !child.failed
}

// RunCleanups executes all registered cleanup functions in LIFO order.
// This is called explicitly on the root runner at program exit.
func (r *cliRunner) RunCleanups() {
	for i := len(r.cleanups) - 1; i >= 0; i-- {
		r.cleanups[i]()
	}
}

func (r *cliRunner) logf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	prefix := indent(r.depth) + "    "
	for _, line := range strings.Split(msg, "\n") {
		fmt.Fprintf(os.Stderr, "%s%s\n", prefix, line)
	}
}

func indent(depth int) string {
	if depth <= 0 {
		return ""
	}
	return strings.Repeat("    ", depth)
}
