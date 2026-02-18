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

// Package tck provides a tamper-resistant Transport Conformance Kit for the
// gomessaging framework. It randomizes service names, injects nonces into
// message payloads, and performs all broker validation directly — preventing
// implementations from passing the TCK by hardcoding responses.
package tck

import (
	"encoding/json"
	"time"

	"github.com/sparetimecoders/gomessaging/spec/spectest"
)

// BrokerConfig holds connection details for direct broker access.
type BrokerConfig struct {
	AMQPURL       string // e.g. amqp://guest:guest@localhost:5672
	ManagementURL string // e.g. http://guest:guest@localhost:15672
	NATSURL       string // e.g. nats://localhost:4222
}

// BrokerClient provides direct broker access for TCK validation.
// Implementations are TCK-owned and not delegated to the adapter.
type BrokerClient interface {
	QueryState(t spectest.T) spectest.BrokerState
	PublishRaw(t spectest.T, target spectest.ProbeTarget, payload json.RawMessage, headers map[string]string) error
	CreateProbeConsumer(t spectest.T, target spectest.ProbeTarget) *spectest.ProbeConsumer
	Cleanup(t spectest.T)
}

// newBrokerClient creates a BrokerClient for the given transport key.
func newBrokerClient(transportKey string, cfg BrokerConfig) BrokerClient {
	switch transportKey {
	case "amqp":
		return &amqpBrokerClient{url: cfg.AMQPURL, managementURL: cfg.ManagementURL}
	case "nats":
		return &natsBrokerClient{url: cfg.NATSURL}
	default:
		return nil
	}
}

// probeTimeout is the default timeout for waiting on probe messages.
const probeTimeout = 5 * time.Second
