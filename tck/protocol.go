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

	"github.com/sparetimecoders/messaging/specification/spec"
	"github.com/sparetimecoders/messaging/specification/spec/spectest"
)

// ProtocolVersion is the current version of the TCK subprocess protocol.
const ProtocolVersion = 1

// Request is a JSON-RPC request sent from the TCK runner to the adapter (via stdin).
type Request struct {
	ID     int             `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

// Response is a JSON-RPC response sent from the adapter to the TCK runner (via fd 3).
type Response struct {
	ID     int             `json:"id"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *RPCError       `json:"error,omitempty"`
}

// RPCError describes a JSON-RPC error.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *RPCError) Error() string { return e.Message }

// --- hello ---

// HelloParams is sent as the first request to establish the protocol version.
type HelloParams struct {
	ProtocolVersion int `json:"protocolVersion"`
}

// HelloResult is the response to hello, providing transport info and broker config.
type HelloResult struct {
	ProtocolVersion int          `json:"protocolVersion"`
	TransportKey    string       `json:"transportKey"`
	BrokerConfig    BrokerConfig `json:"brokerConfig"`
}

// --- start_service ---

// StartServiceParams requests the adapter to start a service with the given intents.
type StartServiceParams struct {
	ServiceName string              `json:"serviceName"`
	Intents     []spectest.SetupIntent `json:"intents"`
}

// StartServiceResult is the response after starting a service.
type StartServiceResult struct {
	PublisherKeys []string      `json:"publisherKeys"`
	Topology      spec.Topology `json:"topology"`
}

// --- publish ---

// PublishParams requests the adapter to publish a message.
type PublishParams struct {
	ServiceName  string            `json:"serviceName"`
	PublisherKey string            `json:"publisherKey"`
	RoutingKey   string            `json:"routingKey"`
	Payload      json.RawMessage   `json:"payload"`
	Headers      map[string]string `json:"headers,omitempty"`
}

// --- received ---

// ReceivedParams requests the list of messages received by a service.
type ReceivedParams struct {
	ServiceName string `json:"serviceName"`
}

// ReceivedResult contains the messages received by a service.
type ReceivedResult struct {
	Messages []ReceivedMessageWire `json:"messages"`
}

// ReceivedMessageWire is the wire format of a received message.
type ReceivedMessageWire struct {
	RoutingKey   string          `json:"routingKey"`
	Payload      json.RawMessage `json:"payload"`
	Metadata     spec.Metadata   `json:"metadata"`
	DeliveryInfo spec.DeliveryInfo `json:"deliveryInfo"`
}

// --- close_service ---

// CloseServiceParams requests the adapter to close a single service.
type CloseServiceParams struct {
	ServiceName string `json:"serviceName"`
}

// --- shutdown ---

// ShutdownParams requests the adapter to exit cleanly.
type ShutdownParams struct{}
