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
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateFile(t *testing.T) {
	tmp := writeTemp(t, `{
		"serviceName": "orders",
		"endpoints": [{
			"direction": "publish",
			"pattern": "event-stream",
			"exchangeName": "events.topic.exchange",
			"exchangeKind": "topic",
			"routingKey": "Order.Created"
		}]
	}`)

	var stdout, stderr bytes.Buffer
	code := run([]string{"validate", tmp}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d: stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "OK") {
		t.Fatalf("expected OK in output, got %q", stdout.String())
	}
}

func TestValidateStdin(t *testing.T) {
	input := `{
		"serviceName": "svc",
		"endpoints": [{
			"direction": "publish",
			"pattern": "event-stream",
			"exchangeName": "events.topic.exchange",
			"exchangeKind": "topic",
			"routingKey": "X"
		}]
	}`

	var stdout, stderr bytes.Buffer
	code := run([]string{"validate", "-"}, strings.NewReader(input), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d: stderr=%s", code, stderr.String())
	}
}

func TestValidateInvalidTopology(t *testing.T) {
	tmp := writeTemp(t, `{
		"serviceName": "",
		"endpoints": []
	}`)

	var stdout, stderr bytes.Buffer
	code := run([]string{"validate", tmp}, nil, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}
	if !strings.Contains(stdout.String(), "service name must not be empty") {
		t.Fatalf("expected error about service name, got %q", stdout.String())
	}
}

func TestValidateInvalidJSON(t *testing.T) {
	tmp := writeTemp(t, `{not json}`)

	var stdout, stderr bytes.Buffer
	code := run([]string{"validate", tmp}, nil, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}
}

func TestValidateVerbose(t *testing.T) {
	tmp := writeTemp(t, `{
		"serviceName": "orders",
		"endpoints": [{
			"direction": "publish",
			"pattern": "event-stream",
			"exchangeName": "events.topic.exchange",
			"exchangeKind": "topic",
			"routingKey": "Order.Created"
		}]
	}`)

	var stdout, stderr bytes.Buffer
	code := run([]string{"validate", "--verbose", tmp}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d: stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "1 endpoints OK") {
		t.Fatalf("expected verbose output, got %q", stdout.String())
	}
}

func TestValidateJSONFormat(t *testing.T) {
	tmp := writeTemp(t, `{
		"serviceName": "orders",
		"endpoints": [{
			"direction": "publish",
			"pattern": "event-stream",
			"exchangeName": "events.topic.exchange",
			"exchangeKind": "topic",
			"routingKey": "Order.Created"
		}]
	}`)

	var stdout, stderr bytes.Buffer
	code := run([]string{"validate", "--format", "json", tmp}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d: stderr=%s", code, stderr.String())
	}

	var res result
	if err := json.Unmarshal(stdout.Bytes(), &res); err != nil {
		t.Fatalf("failed to parse JSON output: %s", err)
	}
	if !res.OK {
		t.Fatalf("expected ok=true, got %+v", res)
	}
}

func TestValidateJSONFormatError(t *testing.T) {
	tmp := writeTemp(t, `{"serviceName": "", "endpoints": []}`)

	var stdout, stderr bytes.Buffer
	code := run([]string{"validate", "--format", "json", tmp}, nil, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}

	var res result
	if err := json.Unmarshal(stdout.Bytes(), &res); err != nil {
		t.Fatalf("failed to parse JSON output: %s", err)
	}
	if res.OK {
		t.Fatalf("expected ok=false")
	}
	if len(res.Errors) == 0 {
		t.Fatalf("expected errors in JSON output")
	}
}

func TestValidateMissingFile(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"validate", "/nonexistent/file.json"}, nil, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}
}

func TestValidateTooManyArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"validate", "a", "b"}, nil, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}
}

func TestCrossValidateMatching(t *testing.T) {
	pub := writeTemp(t, `{
		"serviceName": "orders",
		"endpoints": [{
			"direction": "publish",
			"pattern": "event-stream",
			"exchangeName": "events.topic.exchange",
			"exchangeKind": "topic",
			"routingKey": "Order.Created"
		}]
	}`)
	con := writeTemp(t, `{
		"serviceName": "notifications",
		"endpoints": [{
			"direction": "consume",
			"pattern": "event-stream",
			"exchangeName": "events.topic.exchange",
			"exchangeKind": "topic",
			"queueName": "events.topic.exchange.queue.notifications",
			"routingKey": "Order.Created"
		}]
	}`)

	var stdout, stderr bytes.Buffer
	code := run([]string{"cross-validate", pub, con}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d: stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
}

func TestCrossValidateMissingPublisher(t *testing.T) {
	con := writeTemp(t, `{
		"serviceName": "notifications",
		"endpoints": [{
			"direction": "consume",
			"pattern": "event-stream",
			"exchangeName": "events.topic.exchange",
			"exchangeKind": "topic",
			"queueName": "events.topic.exchange.queue.notifications",
			"routingKey": "Order.Created"
		}]
	}`)

	var stdout, stderr bytes.Buffer
	code := run([]string{"cross-validate", con}, nil, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}
	if !strings.Contains(stdout.String(), "no service publishes it") {
		t.Fatalf("expected missing publisher error, got %q", stdout.String())
	}
}

func TestCrossValidateVerbose(t *testing.T) {
	pub := writeTemp(t, `{
		"serviceName": "orders",
		"endpoints": [{
			"direction": "publish",
			"pattern": "event-stream",
			"exchangeName": "events.topic.exchange",
			"exchangeKind": "topic",
			"routingKey": "Order.Created"
		}]
	}`)

	var stdout, stderr bytes.Buffer
	code := run([]string{"cross-validate", "--verbose", pub}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if !strings.Contains(stdout.String(), "1 services") {
		t.Fatalf("expected verbose output, got %q", stdout.String())
	}
}

func TestCrossValidateNoArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"cross-validate"}, nil, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}
}

func TestCrossValidateInvalidJSON(t *testing.T) {
	tmp := writeTemp(t, `{bad}`)

	var stdout, stderr bytes.Buffer
	code := run([]string{"cross-validate", tmp}, nil, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}
}

func TestCheckFixtures(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "valid.json"), []byte(`{
		"serviceName": "svc",
		"endpoints": [{
			"direction": "publish",
			"pattern": "event-stream",
			"exchangeName": "events.topic.exchange",
			"exchangeKind": "topic",
			"routingKey": "X"
		}]
	}`), 0644)

	var stdout, stderr bytes.Buffer
	code := run([]string{"check-fixtures", dir}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d: stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
}

func TestCheckFixturesVerbose(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.json"), []byte(`{"key":"value"}`), 0644)
	os.WriteFile(filepath.Join(dir, "b.json"), []byte(`{"key":"value"}`), 0644)

	var stdout, stderr bytes.Buffer
	code := run([]string{"check-fixtures", "--verbose", dir}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if !strings.Contains(stdout.String(), "2 JSON files") {
		t.Fatalf("expected verbose output with count, got %q", stdout.String())
	}
}

func TestCheckFixturesInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "bad.json"), []byte(`{not json}`), 0644)

	var stdout, stderr bytes.Buffer
	code := run([]string{"check-fixtures", dir}, nil, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}
	if !strings.Contains(stdout.String(), "invalid JSON") {
		t.Fatalf("expected invalid JSON error, got %q", stdout.String())
	}
}

func TestCheckFixturesInvalidTopology(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "bad_topo.json"), []byte(`{
		"serviceName": "svc",
		"endpoints": [{
			"direction": "publish",
			"pattern": "event-stream",
			"exchangeName": "",
			"exchangeKind": "topic",
			"routingKey": "X"
		}]
	}`), 0644)

	var stdout, stderr bytes.Buffer
	code := run([]string{"check-fixtures", dir}, nil, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}
}

func TestCheckFixturesBadDir(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"check-fixtures", "/nonexistent/dir"}, nil, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}
}

func TestCheckFixturesSkipsSubdirs(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "subdir"), 0755)
	os.WriteFile(filepath.Join(dir, "ok.json"), []byte(`{"key":"val"}`), 0644)

	var stdout, stderr bytes.Buffer
	code := run([]string{"check-fixtures", dir}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
}

func TestCheckFixturesNonTopologyJSON(t *testing.T) {
	dir := t.TempDir()
	// constants.json-like file (valid JSON, no serviceName)
	os.WriteFile(filepath.Join(dir, "constants.json"), []byte(`{"defaultEventExchangeName": "events"}`), 0644)

	var stdout, stderr bytes.Buffer
	code := run([]string{"check-fixtures", "--verbose", dir}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d: stdout=%s", code, stdout.String())
	}
}

func TestNoArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(nil, nil, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}
}

func TestUnknownCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"bogus"}, nil, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}
}

func TestInvalidFormat(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"validate", "--format", "xml"}, strings.NewReader(`{"serviceName":"s","endpoints":[]}`), &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}
}

func TestCheckFixturesWithSpecTestdata(t *testing.T) {
	// Run against the actual spec testdata if available
	dir := filepath.Join("..", "..", "spec", "testdata")
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Skip("spec/testdata not found, skipping")
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"check-fixtures", "--verbose", dir}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit 0 for spec testdata, got %d: stdout=%s", code, stdout.String())
	}
}

func TestVisualizeBasic(t *testing.T) {
	pub := writeTemp(t, `{
		"serviceName": "orders",
		"endpoints": [{
			"direction": "publish",
			"pattern": "event-stream",
			"exchangeName": "events.topic.exchange",
			"exchangeKind": "topic",
			"routingKey": "Order.Created"
		}]
	}`)
	con := writeTemp(t, `{
		"serviceName": "notifications",
		"endpoints": [{
			"direction": "consume",
			"pattern": "event-stream",
			"exchangeName": "events.topic.exchange",
			"exchangeKind": "topic",
			"queueName": "events.topic.exchange.queue.notifications",
			"routingKey": "Order.Created"
		}]
	}`)

	var stdout, stderr bytes.Buffer
	code := run([]string{"visualize", pub, con}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d: stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "graph LR") {
		t.Fatalf("expected graph LR in output, got %q", out)
	}
	if !strings.Contains(out, "orders") {
		t.Fatalf("expected orders in output, got %q", out)
	}
	if !strings.Contains(out, "notifications") {
		t.Fatalf("expected notifications in output, got %q", out)
	}
}

func TestVisualizeInvalidTopology(t *testing.T) {
	tmp := writeTemp(t, `{
		"serviceName": "",
		"endpoints": []
	}`)

	var stdout, stderr bytes.Buffer
	code := run([]string{"visualize", tmp}, nil, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}
}

func TestVisualizeNoValidateSkipsValidation(t *testing.T) {
	tmp := writeTemp(t, `{
		"serviceName": "",
		"endpoints": []
	}`)

	var stdout, stderr bytes.Buffer
	code := run([]string{"visualize", "--no-validate", tmp}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit 0 with --no-validate, got %d: stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "graph LR") {
		t.Fatalf("expected graph LR in output, got %q", stdout.String())
	}
}

func TestVisualizeJSONFormat(t *testing.T) {
	tmp := writeTemp(t, `{
		"serviceName": "orders",
		"endpoints": [{
			"direction": "publish",
			"pattern": "event-stream",
			"exchangeName": "events.topic.exchange",
			"exchangeKind": "topic",
			"routingKey": "Order.Created"
		}]
	}`)

	var stdout, stderr bytes.Buffer
	code := run([]string{"visualize", "--format", "json", tmp}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d: stderr=%s", code, stderr.String())
	}

	var res struct {
		OK      bool   `json:"ok"`
		Diagram string `json:"diagram"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &res); err != nil {
		t.Fatalf("failed to parse JSON output: %s\nraw: %s", err, stdout.String())
	}
	if !res.OK {
		t.Fatalf("expected ok=true")
	}
	if !strings.Contains(res.Diagram, "graph LR") {
		t.Fatalf("expected graph LR in diagram, got %q", res.Diagram)
	}
}

func TestVisualizeNoArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"visualize"}, nil, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}
}

func TestVisualizeInvalidJSON(t *testing.T) {
	tmp := writeTemp(t, `{bad}`)

	var stdout, stderr bytes.Buffer
	code := run([]string{"visualize", tmp}, nil, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}
}

func TestVisualizeMissingFile(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"visualize", "/nonexistent/file.json"}, nil, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}
}

func TestDiscoverMermaid(t *testing.T) {
	srv := newBrokerTestServer(t)
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	code := run([]string{"discover", "--url", srv.URL}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d: stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "graph LR") {
		t.Fatalf("expected graph LR in output, got %q", out)
	}
	if !strings.Contains(out, "notifications") {
		t.Fatalf("expected notifications in output, got %q", out)
	}
}

func TestDiscoverJSON(t *testing.T) {
	srv := newBrokerTestServer(t)
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	code := run([]string{"discover", "--url", srv.URL, "--output", "json"}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d: stderr=%s", code, stderr.String())
	}

	var topos []struct {
		ServiceName string `json:"serviceName"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &topos); err != nil {
		t.Fatalf("failed to parse JSON: %s\nraw: %s", err, stdout.String())
	}
	if len(topos) == 0 {
		t.Fatal("expected at least one topology")
	}
}

func TestDiscoverTopologyDir(t *testing.T) {
	srv := newBrokerTestServer(t)
	defer srv.Close()

	dir := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := run([]string{"discover", "--url", srv.URL, "--output", "topology-dir", dir}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d: stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("expected topology files in directory")
	}
}

func TestDiscoverTopologyDirNoArgs(t *testing.T) {
	srv := newBrokerTestServer(t)
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	code := run([]string{"discover", "--url", srv.URL, "--output", "topology-dir"}, nil, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}
}

func TestDiscoverAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	code := run([]string{"discover", "--url", srv.URL}, nil, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}
}

func TestDiscoverMermaidJSON(t *testing.T) {
	srv := newBrokerTestServer(t)
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	code := run([]string{"discover", "--url", srv.URL, "--format", "json"}, nil, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d: stderr=%s", code, stderr.String())
	}

	var res struct {
		OK      bool   `json:"ok"`
		Diagram string `json:"diagram"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &res); err != nil {
		t.Fatalf("failed to parse JSON: %s\nraw: %s", err, stdout.String())
	}
	if !res.OK {
		t.Fatal("expected ok=true")
	}
	if !strings.Contains(res.Diagram, "graph LR") {
		t.Fatalf("expected graph LR in diagram, got %q", res.Diagram)
	}
}

func newBrokerTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	exchanges := `[
		{"name":"events.topic.exchange","type":"topic"},
		{"name":"amq.topic","type":"topic"}
	]`
	bindings := `[
		{"source":"events.topic.exchange","destination":"events.topic.exchange.queue.notifications","destination_type":"queue","routing_key":"Order.Created"}
	]`
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/api/exchanges/"):
			w.Write([]byte(exchanges)) //nolint:errcheck
		case strings.Contains(r.URL.Path, "/api/bindings/"):
			w.Write([]byte(bindings)) //nolint:errcheck
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "*.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	f.Close()
	return f.Name()
}
