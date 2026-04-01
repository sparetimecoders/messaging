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
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/sparetimecoders/messaging"
	"github.com/sparetimecoders/messaging/spectest"
)

const (
	startupTimeout  = 10 * time.Second
	commandTimeout  = 30 * time.Second
	shutdownTimeout = 5 * time.Second
)

// SubprocessAdapter implements [Adapter] by spawning an external process and
// communicating via a newline-delimited JSON-RPC protocol. Requests are sent
// on stdin; responses are read from fd 3. stdout/stderr are forwarded to t.Log.
type SubprocessAdapter struct {
	cmd          *exec.Cmd
	stdin        io.WriteCloser
	responsePipe *os.File // read end of fd 3 pipe
	scanner      *bufio.Scanner
	nextID       int
	transportKey string
	brokerCfg    BrokerConfig
	mu           sync.Mutex
}

// NewSubprocessAdapter starts the adapter process, performs the hello handshake,
// and returns a ready-to-use Adapter.
func NewSubprocessAdapter(t spectest.T, command string, args ...string) *SubprocessAdapter {
	t.Helper()

	pipeR, pipeW, err := os.Pipe()
	if err != nil {
		t.Fatalf("tck subprocess: failed to create pipe: %v", err)
	}

	cmd := exec.Command(command, args...)
	cmd.ExtraFiles = []*os.File{pipeW} // fd 3 in the child
	cmd.Env = append(os.Environ(), cmd.Env...)

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		pipeR.Close()
		pipeW.Close()
		t.Fatalf("tck subprocess: failed to create stdin pipe: %v", err)
	}

	// Capture stdout and stderr for diagnostics.
	stdoutR, stdoutW := io.Pipe()
	stderrR, stderrW := io.Pipe()
	cmd.Stdout = stdoutW
	cmd.Stderr = stderrW

	if err := cmd.Start(); err != nil {
		pipeR.Close()
		pipeW.Close()
		t.Fatalf("tck subprocess: failed to start %q: %v", command, err)
	}

	// Close the write end of the pipe in the parent — only the child writes to it.
	pipeW.Close()

	// Forward subprocess stdout/stderr to t.Log in background goroutines.
	go forwardOutput(t, "stdout", stdoutR)
	go forwardOutput(t, "stderr", stderrR)

	a := &SubprocessAdapter{
		cmd:          cmd,
		stdin:        stdinPipe,
		responsePipe: pipeR,
		scanner:      bufio.NewScanner(pipeR),
		nextID:       1,
	}

	// Ensure cleanup on test end.
	t.Cleanup(func() {
		a.shutdown(t)
		stdoutW.Close()
		stderrW.Close()
	})

	// Perform hello handshake.
	a.hello(t)
	return a
}

func (a *SubprocessAdapter) TransportKey() string { return a.transportKey }

func (a *SubprocessAdapter) BrokerConfig() BrokerConfig { return a.brokerCfg }

func (a *SubprocessAdapter) StartService(t spectest.T, serviceName string, intents []spectest.SetupIntent) *spectest.ServiceHandle {
	t.Helper()

	params := StartServiceParams{
		ServiceName: serviceName,
		Intents:     intents,
	}
	var result StartServiceResult
	a.call(t, "start_service", params, &result, commandTimeout)

	publisherKeys := make(map[string]spectest.PublishFunc, len(result.PublisherKeys))
	for _, key := range result.PublisherKeys {
		pk := key // capture
		publisherKeys[pk] = func(_ context.Context, routingKey string, payload json.RawMessage, headers map[string]string) error {
			return a.publish(t, serviceName, pk, routingKey, payload, headers)
		}
	}

	return &spectest.ServiceHandle{
		Topology: func() messaging.Topology {
			return result.Topology
		},
		Publishers: publisherKeys,
		Received: func() []spectest.ReceivedMessage {
			return a.received(t, serviceName)
		},
		Close: func() error {
			a.closeService(t, serviceName)
			return nil
		},
	}
}

func (a *SubprocessAdapter) hello(t spectest.T) {
	t.Helper()
	params := HelloParams{ProtocolVersion: ProtocolVersion}
	var result HelloResult
	a.call(t, "hello", params, &result, startupTimeout)

	if result.ProtocolVersion != ProtocolVersion {
		t.Fatalf("tck subprocess: protocol version mismatch: got %d, want %d", result.ProtocolVersion, ProtocolVersion)
	}
	a.transportKey = result.TransportKey
	a.brokerCfg = result.BrokerConfig
}

func (a *SubprocessAdapter) publish(t spectest.T, serviceName, publisherKey, routingKey string, payload json.RawMessage, headers map[string]string) error {
	t.Helper()
	params := PublishParams{
		ServiceName:  serviceName,
		PublisherKey: publisherKey,
		RoutingKey:   routingKey,
		Payload:      payload,
		Headers:      headers,
	}
	a.call(t, "publish", params, nil, commandTimeout)
	return nil
}

func (a *SubprocessAdapter) received(t spectest.T, serviceName string) []spectest.ReceivedMessage {
	t.Helper()
	params := ReceivedParams{ServiceName: serviceName}
	var result ReceivedResult
	a.call(t, "received", params, &result, commandTimeout)

	messages := make([]spectest.ReceivedMessage, len(result.Messages))
	for i, m := range result.Messages {
		messages[i] = spectest.ReceivedMessage{
			RoutingKey: m.RoutingKey,
			Payload:    m.Payload,
			Metadata:   m.Metadata,
			Info:       m.DeliveryInfo,
		}
	}
	return messages
}

func (a *SubprocessAdapter) closeService(t spectest.T, serviceName string) {
	t.Helper()
	params := CloseServiceParams{ServiceName: serviceName}
	a.call(t, "close_service", params, nil, commandTimeout)
}

func (a *SubprocessAdapter) shutdown(t spectest.T) {
	t.Helper()
	a.mu.Lock()
	defer a.mu.Unlock()

	// Send shutdown request (best-effort).
	id := a.nextID
	a.nextID++
	req := Request{ID: id, Method: "shutdown", Params: mustMarshal(ShutdownParams{})}
	_ = a.writeRequest(req)

	// Wait for process to exit.
	done := make(chan error, 1)
	go func() { done <- a.cmd.Wait() }()

	select {
	case <-done:
		// Clean exit.
	case <-time.After(shutdownTimeout):
		t.Logf("tck subprocess: shutdown timeout, sending SIGKILL")
		_ = a.cmd.Process.Kill()
		<-done
	}
	a.responsePipe.Close()
}

// call sends a JSON-RPC request and waits for the matching response.
func (a *SubprocessAdapter) call(t spectest.T, method string, params any, result any, timeout time.Duration) {
	t.Helper()
	a.mu.Lock()
	defer a.mu.Unlock()

	id := a.nextID
	a.nextID++

	req := Request{
		ID:     id,
		Method: method,
		Params: mustMarshal(params),
	}

	if err := a.writeRequest(req); err != nil {
		t.Fatalf("tck subprocess: failed to write %s request: %v", method, err)
	}

	resp := a.readResponse(t, id, timeout)
	if resp.Error != nil {
		t.Fatalf("tck subprocess: %s returned error: [%d] %s", method, resp.Error.Code, resp.Error.Message)
	}

	if result != nil && len(resp.Result) > 0 {
		if err := json.Unmarshal(resp.Result, result); err != nil {
			t.Fatalf("tck subprocess: failed to unmarshal %s result: %v", method, err)
		}
	}
}

func (a *SubprocessAdapter) writeRequest(req Request) error {
	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}
	data = append(data, '\n')
	_, err = a.stdin.Write(data)
	return err
}

func (a *SubprocessAdapter) readResponse(t spectest.T, expectedID int, timeout time.Duration) Response {
	t.Helper()

	type scanResult struct {
		line string
		ok   bool
	}
	ch := make(chan scanResult, 1)
	// Deliberate leak: this goroutine may outlive the timeout because bufio.Scanner.Scan
	// blocks on I/O with no cancellation support. The subprocess is killed in shutdown(),
	// which closes the pipe and unblocks the read, so the goroutine exits shortly after.
	go func() {
		ok := a.scanner.Scan()
		ch <- scanResult{line: a.scanner.Text(), ok: ok}
	}()

	select {
	case sr := <-ch:
		if !sr.ok {
			if err := a.scanner.Err(); err != nil {
				t.Fatalf("tck subprocess: read error: %v", err)
			}
			t.Fatalf("tck subprocess: unexpected EOF while waiting for response id=%d", expectedID)
		}
		var resp Response
		if err := json.Unmarshal([]byte(sr.line), &resp); err != nil {
			t.Fatalf("tck subprocess: invalid JSON response: %v\nraw: %s", err, sr.line)
		}
		if resp.ID != expectedID {
			t.Fatalf("tck subprocess: response id mismatch: got %d, want %d", resp.ID, expectedID)
		}
		return resp
	case <-time.After(timeout):
		t.Fatalf("tck subprocess: timeout waiting for response id=%d after %v", expectedID, timeout)
		return Response{} // unreachable
	}
}

func forwardOutput(t spectest.T, label string, r io.Reader) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		t.Logf("adapter %s: %s", label, scanner.Text())
	}
}

func mustMarshal(v any) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("tck: marshal failed: %v", err))
	}
	return data
}
