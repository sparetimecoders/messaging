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

package spectest

import (
	"fmt"
	"reflect"
	"testing"
	"time"
)

// T is the test interface used by specification and TCK code. It mirrors a
// subset of [testing.TB] but uses func(T) signatures so that it can be
// implemented outside the testing package (e.g. by a standalone CLI runner).
type T interface {
	Helper()
	Fatalf(format string, args ...any)
	Fatal(args ...any)
	Errorf(format string, args ...any)
	Logf(format string, args ...any)
	Log(args ...any)
	Cleanup(func())
	Run(name string, f func(T)) bool
	FailNow()
	Failed() bool
	Name() string
}

// WrapT wraps a [*testing.T] to satisfy the [T] interface.
func WrapT(t *testing.T) T {
	return &testingTWrapper{t: t}
}

type testingTWrapper struct{ t *testing.T }

func (w *testingTWrapper) Helper()                   { w.t.Helper() }
func (w *testingTWrapper) Fatalf(f string, a ...any) { w.t.Helper(); w.t.Fatalf(f, a...) }
func (w *testingTWrapper) Fatal(a ...any)            { w.t.Helper(); w.t.Fatal(a...) }
func (w *testingTWrapper) Errorf(f string, a ...any) { w.t.Helper(); w.t.Errorf(f, a...) }
func (w *testingTWrapper) Logf(f string, a ...any)   { w.t.Helper(); w.t.Logf(f, a...) }
func (w *testingTWrapper) Log(a ...any)              { w.t.Helper(); w.t.Log(a...) }
func (w *testingTWrapper) Cleanup(fn func())         { w.t.Cleanup(fn) }
func (w *testingTWrapper) FailNow()                  { w.t.FailNow() }
func (w *testingTWrapper) Failed() bool              { return w.t.Failed() }
func (w *testingTWrapper) Name() string              { return w.t.Name() }

func (w *testingTWrapper) Run(name string, f func(T)) bool {
	return w.t.Run(name, func(t *testing.T) { f(WrapT(t)) })
}

// --- Assertion helpers ---
//
// These replace testify's require.* and assert.* in non-test code, enabling
// the TCK to run without a *testing.T.

// RequireNoError calls t.Fatalf if err is non-nil.
func RequireNoError(t T, err error, msg ...any) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s\n  got: %v", failPrefix("unexpected error", msg), err)
	}
}

// RequireEqual calls t.Fatalf if expected != actual (using [reflect.DeepEqual]).
func RequireEqual(t T, expected, actual any, msg ...any) {
	t.Helper()
	if !reflect.DeepEqual(expected, actual) {
		t.Fatalf("%s\n  expected: %v\n  actual:   %v", failPrefix("not equal", msg), expected, actual)
	}
}

// RequireNotNil calls t.Fatalf if v is nil.
func RequireNotNil(t T, v any, msg ...any) {
	t.Helper()
	if isNil(v) {
		t.Fatalf("%s", failPrefix("expected non-nil value", msg))
	}
}

// RequireTrue calls t.Fatalf if cond is false.
func RequireTrue(t T, cond bool, msg ...any) {
	t.Helper()
	if !cond {
		t.Fatalf("%s", failPrefix("expected true", msg))
	}
}

// RequireNotEmpty calls t.Fatalf if v is empty (nil, zero length, or zero value).
func RequireNotEmpty(t T, v any, msg ...any) {
	t.Helper()
	if isEmpty(v) {
		t.Fatalf("%s", failPrefix("expected non-empty value", msg))
	}
}

// AssertEqual calls t.Errorf if expected != actual (using [reflect.DeepEqual]).
func AssertEqual(t T, expected, actual any, msg ...any) {
	t.Helper()
	if !reflect.DeepEqual(expected, actual) {
		t.Errorf("%s\n  expected: %v\n  actual:   %v", failPrefix("not equal", msg), expected, actual)
	}
}

// AssertNotEqual calls t.Errorf if a == b (using [reflect.DeepEqual]).
func AssertNotEqual(t T, a, b any, msg ...any) {
	t.Helper()
	if reflect.DeepEqual(a, b) {
		t.Errorf("%s\n  values are equal: %v", failPrefix("expected different values", msg), a)
	}
}

// AssertEventually polls fn until it returns true or timeout elapses.
func AssertEventually(t T, fn func() bool, timeout, interval time.Duration, msg ...any) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(interval)
	}
	t.Errorf("%s", failPrefix("condition not met within timeout", msg))
}

// --- internal helpers ---

func failPrefix(prefix string, msg []any) string {
	m := formatMsg(msg)
	if m != "" {
		return prefix + ": " + m
	}
	return prefix
}

func formatMsg(msg []any) string {
	if len(msg) == 0 {
		return ""
	}
	if format, ok := msg[0].(string); ok && len(msg) > 1 {
		return fmt.Sprintf(format, msg[1:]...)
	}
	return fmt.Sprint(msg...)
}

func isNil(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return rv.IsNil()
	}
	return false
}

func isEmpty(v any) bool {
	if isNil(v) {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Array, reflect.Chan, reflect.Map, reflect.Slice, reflect.String:
		return rv.Len() == 0
	}
	return false
}
