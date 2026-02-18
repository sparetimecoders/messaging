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

package adapterutil

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/sparetimecoders/gomessaging/spec"
	"github.com/sparetimecoders/gomessaging/spec/spectest"
	"github.com/sparetimecoders/gomessaging/tck"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockManager struct{}

func (m *mockManager) StartService(serviceName string, intents []spectest.SetupIntent) (*ServiceState, error) {
	var pubKeys []string
	for _, intent := range intents {
		if intent.Direction == "publish" {
			pubKeys = append(pubKeys, spectest.PublisherKey(intent))
		}
	}

	var received []tck.ReceivedMessageWire

	return &ServiceState{
		Topology: spec.Topology{
			Transport:   spec.TransportNATS,
			ServiceName: serviceName,
			Endpoints: []spec.Endpoint{{
				Direction:    spec.DirectionPublish,
				Pattern:      spec.PatternEventStream,
				ExchangeName: "events",
				ExchangeKind: spec.ExchangeTopic,
			}},
		},
		PublisherKeys: pubKeys,
		Publish: func(publisherKey, routingKey string, payload json.RawMessage, headers map[string]string) error {
			received = append(received, tck.ReceivedMessageWire{
				RoutingKey: routingKey,
				Payload:    payload,
				Metadata:   spec.Metadata{Type: routingKey, Source: serviceName},
			})
			return nil
		},
		Received: func() []tck.ReceivedMessageWire {
			result := make([]tck.ReceivedMessageWire, len(received))
			copy(result, received)
			return result
		},
		Close: func() error { return nil },
	}, nil
}

func TestServeFullSession(t *testing.T) {
	requests := []string{
		`{"id":1,"method":"hello","params":{"protocolVersion":1}}`,
		`{"id":2,"method":"start_service","params":{"serviceName":"svc-1","intents":[{"pattern":"event-stream","direction":"publish"}]}}`,
		`{"id":3,"method":"publish","params":{"serviceName":"svc-1","publisherKey":"event-stream","routingKey":"Evt.One","payload":{"k":"v"}}}`,
		`{"id":4,"method":"received","params":{"serviceName":"svc-1"}}`,
		`{"id":5,"method":"close_service","params":{"serviceName":"svc-1"}}`,
		`{"id":6,"method":"shutdown","params":{}}`,
	}
	input := strings.Join(requests, "\n") + "\n"

	inputR := strings.NewReader(input)
	outputR, outputW := io.Pipe()

	mgr := &mockManager{}
	done := make(chan error, 1)
	go func() {
		done <- serve(inputR, outputW, "nats", tck.BrokerConfig{NATSURL: "nats://mock:4222"}, mgr)
		outputW.Close()
	}()

	scanner := bufio.NewScanner(outputR)
	var responses []tck.Response

	for scanner.Scan() {
		var resp tck.Response
		require.NoError(t, json.Unmarshal(scanner.Bytes(), &resp))
		responses = append(responses, resp)
	}

	require.NoError(t, <-done)
	require.Len(t, responses, 6)

	// hello
	assert.Equal(t, 1, responses[0].ID)
	assert.Nil(t, responses[0].Error)
	var helloResult tck.HelloResult
	require.NoError(t, json.Unmarshal(responses[0].Result, &helloResult))
	assert.Equal(t, tck.ProtocolVersion, helloResult.ProtocolVersion)
	assert.Equal(t, "nats", helloResult.TransportKey)
	assert.Equal(t, "nats://mock:4222", helloResult.BrokerConfig.NATSURL)

	// start_service
	assert.Equal(t, 2, responses[1].ID)
	assert.Nil(t, responses[1].Error)
	var startResult tck.StartServiceResult
	require.NoError(t, json.Unmarshal(responses[1].Result, &startResult))
	assert.Equal(t, []string{"event-stream"}, startResult.PublisherKeys)
	assert.Equal(t, "svc-1", startResult.Topology.ServiceName)

	// publish
	assert.Equal(t, 3, responses[2].ID)
	assert.Nil(t, responses[2].Error)

	// received
	assert.Equal(t, 4, responses[3].ID)
	assert.Nil(t, responses[3].Error)
	var recvResult tck.ReceivedResult
	require.NoError(t, json.Unmarshal(responses[3].Result, &recvResult))
	require.Len(t, recvResult.Messages, 1)
	assert.Equal(t, "Evt.One", recvResult.Messages[0].RoutingKey)

	// close_service
	assert.Equal(t, 5, responses[4].ID)
	assert.Nil(t, responses[4].Error)

	// shutdown
	assert.Equal(t, 6, responses[5].ID)
	assert.Nil(t, responses[5].Error)
}

func TestServeUnknownMethod(t *testing.T) {
	input := `{"id":1,"method":"bogus","params":{}}` + "\n" + `{"id":2,"method":"shutdown","params":{}}` + "\n"
	inputR := strings.NewReader(input)
	outputR, outputW := io.Pipe()

	done := make(chan error, 1)
	go func() {
		done <- serve(inputR, outputW, "nats", tck.BrokerConfig{}, &mockManager{})
		outputW.Close()
	}()

	scanner := bufio.NewScanner(outputR)
	var responses []tck.Response
	for scanner.Scan() {
		var resp tck.Response
		require.NoError(t, json.Unmarshal(scanner.Bytes(), &resp))
		responses = append(responses, resp)
	}
	require.NoError(t, <-done)
	require.Len(t, responses, 2)

	assert.NotNil(t, responses[0].Error)
	assert.Contains(t, responses[0].Error.Message, "unknown method")
}

func TestServeUnknownServicePublish(t *testing.T) {
	input := strings.Join([]string{
		`{"id":1,"method":"hello","params":{"protocolVersion":1}}`,
		`{"id":2,"method":"publish","params":{"serviceName":"nonexistent","publisherKey":"x","routingKey":"y","payload":{}}}`,
		`{"id":3,"method":"shutdown","params":{}}`,
	}, "\n") + "\n"

	inputR := strings.NewReader(input)
	outputR, outputW := io.Pipe()

	done := make(chan error, 1)
	go func() {
		done <- serve(inputR, outputW, "nats", tck.BrokerConfig{}, &mockManager{})
		outputW.Close()
	}()

	scanner := bufio.NewScanner(outputR)
	var responses []tck.Response
	for scanner.Scan() {
		var resp tck.Response
		require.NoError(t, json.Unmarshal(scanner.Bytes(), &resp))
		responses = append(responses, resp)
	}
	require.NoError(t, <-done)
	require.Len(t, responses, 3)

	// publish to unknown service should error
	assert.NotNil(t, responses[1].Error)
	assert.Contains(t, responses[1].Error.Message, "unknown service")
}

func TestServeShutdownClosesRemainingServices(t *testing.T) {
	closeCalled := false
	mgr := &closingManager{onClose: func() { closeCalled = true }}

	input := strings.Join([]string{
		`{"id":1,"method":"hello","params":{"protocolVersion":1}}`,
		`{"id":2,"method":"start_service","params":{"serviceName":"svc-1","intents":[]}}`,
		`{"id":3,"method":"shutdown","params":{}}`,
	}, "\n") + "\n"

	inputR := strings.NewReader(input)
	outputR, outputW := io.Pipe()

	done := make(chan error, 1)
	go func() {
		done <- serve(inputR, outputW, "nats", tck.BrokerConfig{}, mgr)
		outputW.Close()
	}()

	scanner := bufio.NewScanner(outputR)
	for scanner.Scan() {
		// drain
	}
	require.NoError(t, <-done)
	assert.True(t, closeCalled, "shutdown should close remaining services")
}

type closingManager struct {
	onClose func()
}

func (m *closingManager) StartService(serviceName string, intents []spectest.SetupIntent) (*ServiceState, error) {
	return &ServiceState{
		Topology:      spec.Topology{ServiceName: serviceName},
		PublisherKeys: nil,
		Publish:       func(_, _ string, _ json.RawMessage, _ map[string]string) error { return nil },
		Received:      func() []tck.ReceivedMessageWire { return nil },
		Close: func() error {
			m.onClose()
			return nil
		},
	}, nil
}
