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

// Package adapterutil provides a reusable adapter-side protocol handler for the
// TCK subprocess protocol. Transport adapter binaries implement [ServiceManager]
// and call [Serve] to handle the JSON-RPC communication automatically.
package adapterutil

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/sparetimecoders/gomessaging/spec"
	"github.com/sparetimecoders/gomessaging/spec/spectest"
	"github.com/sparetimecoders/gomessaging/tck"
)

// ServiceManager is implemented by transport-specific adapter binaries.
type ServiceManager interface {
	StartService(serviceName string, intents []spectest.SetupIntent) (*ServiceState, error)
}

// ServiceState holds the runtime state of a started service.
type ServiceState struct {
	Topology      spec.Topology
	PublisherKeys []string
	Publish       func(publisherKey, routingKey string, payload json.RawMessage, headers map[string]string) error
	Received      func() []tck.ReceivedMessageWire
	Close         func() error
}

// Serve reads JSON-RPC requests from stdin, dispatches to the ServiceManager,
// and writes responses to fd 3. stdout and stderr remain available for
// diagnostic logging by the adapter.
func Serve(transportKey string, brokerConfig tck.BrokerConfig, mgr ServiceManager) error {
	protocolW := os.NewFile(3, "tck-protocol")
	if protocolW == nil {
		return fmt.Errorf("adapterutil: fd 3 not available")
	}
	defer protocolW.Close()

	return serve(os.Stdin, protocolW, transportKey, brokerConfig, mgr)
}

func serve(input io.Reader, output io.Writer, transportKey string, brokerConfig tck.BrokerConfig, mgr ServiceManager) error {
	scanner := bufio.NewScanner(input)
	services := make(map[string]*ServiceState)

	for scanner.Scan() {
		line := scanner.Bytes()
		var req tck.Request
		if err := json.Unmarshal(line, &req); err != nil {
			writeError(output, 0, -1, fmt.Sprintf("invalid request JSON: %v", err))
			continue
		}

		switch req.Method {
		case "hello":
			handleHello(output, req, transportKey, brokerConfig)
		case "start_service":
			handleStartService(output, req, mgr, services)
		case "publish":
			handlePublish(output, req, services)
		case "received":
			handleReceived(output, req, services)
		case "close_service":
			handleCloseService(output, req, services)
		case "shutdown":
			handleShutdown(output, req, services)
			return nil
		default:
			writeError(output, req.ID, -1, fmt.Sprintf("unknown method: %s", req.Method))
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("adapterutil: stdin read error: %w", err)
	}
	return nil // stdin closed
}

func handleHello(w io.Writer, req tck.Request, transportKey string, brokerConfig tck.BrokerConfig) {
	var params tck.HelloParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		writeError(w, req.ID, -1, fmt.Sprintf("invalid hello params: %v", err))
		return
	}
	writeResult(w, req.ID, tck.HelloResult{
		ProtocolVersion: tck.ProtocolVersion,
		TransportKey:    transportKey,
		BrokerConfig:    brokerConfig,
	})
}

func handleStartService(w io.Writer, req tck.Request, mgr ServiceManager, services map[string]*ServiceState) {
	var params tck.StartServiceParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		writeError(w, req.ID, -1, fmt.Sprintf("invalid start_service params: %v", err))
		return
	}
	state, err := mgr.StartService(params.ServiceName, params.Intents)
	if err != nil {
		writeError(w, req.ID, -1, fmt.Sprintf("start_service failed: %v", err))
		return
	}
	services[params.ServiceName] = state
	writeResult(w, req.ID, tck.StartServiceResult{
		PublisherKeys: state.PublisherKeys,
		Topology:      state.Topology,
	})
}

func handlePublish(w io.Writer, req tck.Request, services map[string]*ServiceState) {
	var params tck.PublishParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		writeError(w, req.ID, -1, fmt.Sprintf("invalid publish params: %v", err))
		return
	}
	svc, ok := services[params.ServiceName]
	if !ok {
		writeError(w, req.ID, -1, fmt.Sprintf("unknown service: %s", params.ServiceName))
		return
	}
	if err := svc.Publish(params.PublisherKey, params.RoutingKey, params.Payload, params.Headers); err != nil {
		writeError(w, req.ID, -1, fmt.Sprintf("publish failed: %v", err))
		return
	}
	writeResult(w, req.ID, struct{}{})
}

func handleReceived(w io.Writer, req tck.Request, services map[string]*ServiceState) {
	var params tck.ReceivedParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		writeError(w, req.ID, -1, fmt.Sprintf("invalid received params: %v", err))
		return
	}
	svc, ok := services[params.ServiceName]
	if !ok {
		writeError(w, req.ID, -1, fmt.Sprintf("unknown service: %s", params.ServiceName))
		return
	}
	writeResult(w, req.ID, tck.ReceivedResult{Messages: svc.Received()})
}

func handleCloseService(w io.Writer, req tck.Request, services map[string]*ServiceState) {
	var params tck.CloseServiceParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		writeError(w, req.ID, -1, fmt.Sprintf("invalid close_service params: %v", err))
		return
	}
	svc, ok := services[params.ServiceName]
	if !ok {
		writeError(w, req.ID, -1, fmt.Sprintf("unknown service: %s", params.ServiceName))
		return
	}
	if err := svc.Close(); err != nil {
		writeError(w, req.ID, -1, fmt.Sprintf("close_service failed: %v", err))
		return
	}
	delete(services, params.ServiceName)
	writeResult(w, req.ID, struct{}{})
}

func handleShutdown(w io.Writer, req tck.Request, services map[string]*ServiceState) {
	// Close all remaining services.
	for name, svc := range services {
		_ = svc.Close()
		delete(services, name)
	}
	writeResult(w, req.ID, struct{}{})
}

func writeResult(w io.Writer, id int, result any) {
	data, err := json.Marshal(result)
	if err != nil {
		writeError(w, id, -1, fmt.Sprintf("failed to marshal result: %v", err))
		return
	}
	resp := tck.Response{ID: id, Result: data}
	line, _ := json.Marshal(resp)
	line = append(line, '\n')
	_, _ = w.Write(line)
}

func writeError(w io.Writer, id int, code int, message string) {
	resp := tck.Response{ID: id, Error: &tck.RPCError{Code: code, Message: message}}
	line, _ := json.Marshal(resp)
	line = append(line, '\n')
	_, _ = w.Write(line)
}
