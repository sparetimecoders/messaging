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

package nats

import (
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"

	"github.com/sparetimecoders/gomessaging/spec"
	"github.com/sparetimecoders/gomessaging/spec/spectest"
	"github.com/sparetimecoders/gomessaging/tck"
	"github.com/stretchr/testify/require"
)

type natsIntegrationAdapter struct {
	url string
}

func (a *natsIntegrationAdapter) TransportKey() string { return "nats" }

func (a *natsIntegrationAdapter) BrokerConfig() tck.BrokerConfig {
	return tck.BrokerConfig{
		NATSURL: a.url,
	}
}

func (a *natsIntegrationAdapter) StartService(t spectest.T, serviceName string, intents []spectest.SetupIntent) *spectest.ServiceHandle {
	t.Helper()

	var mu sync.Mutex
	var received []spectest.ReceivedMessage
	publishers := make(map[string]spectest.PublishFunc)

	captureHandler := func(_ context.Context, event spec.ConsumableEvent[json.RawMessage]) error {
		mu.Lock()
		defer mu.Unlock()
		received = append(received, spectest.ReceivedMessage{
			RoutingKey: event.DeliveryInfo.Key,
			Payload:    event.Payload,
			Metadata:   event.Metadata,
			Info:       event.DeliveryInfo,
		})
		return nil
	}

	var setups []Setup
	for _, intent := range intents {
		switch {
		case intent.Pattern == "event-stream" && intent.Direction == "publish":
			pub := NewPublisher()
			setups = append(setups, EventStreamPublisher(pub))
			publishers[spectest.PublisherKey(intent)] = publishFunc(pub)

		case intent.Pattern == "event-stream" && intent.Direction == "consume" && !intent.Ephemeral:
			setups = append(setups, EventStreamConsumer(intent.RoutingKey, captureHandler))

		case intent.Pattern == "event-stream" && intent.Direction == "consume" && intent.Ephemeral:
			setups = append(setups, TransientEventStreamConsumer(intent.RoutingKey, captureHandler))

		case intent.Pattern == "custom-stream" && intent.Direction == "publish":
			pub := NewPublisher()
			setups = append(setups, StreamPublisher(intent.Exchange, pub))
			publishers[spectest.PublisherKey(intent)] = publishFunc(pub)

		case intent.Pattern == "custom-stream" && intent.Direction == "consume" && !intent.Ephemeral:
			setups = append(setups, StreamConsumer(intent.Exchange, intent.RoutingKey, captureHandler))

		case intent.Pattern == "custom-stream" && intent.Direction == "consume" && intent.Ephemeral:
			setups = append(setups, TransientStreamConsumer(intent.Exchange, intent.RoutingKey, captureHandler))

		case intent.Pattern == "service-request" && intent.Direction == "consume":
			rrHandler := func(_ context.Context, event spec.ConsumableEvent[json.RawMessage]) (json.RawMessage, error) {
				mu.Lock()
				defer mu.Unlock()
				received = append(received, spectest.ReceivedMessage{
					RoutingKey: event.DeliveryInfo.Key,
					Payload:    event.Payload,
					Metadata:   event.Metadata,
					Info:       event.DeliveryInfo,
				})
				return json.RawMessage(`{"ok":true}`), nil
			}
			setups = append(setups, RequestResponseHandler[json.RawMessage, json.RawMessage](intent.RoutingKey, rrHandler))

		case intent.Pattern == "service-request" && intent.Direction == "publish":
			pub := NewPublisher()
			setups = append(setups, ServicePublisher(intent.TargetService, pub))
			publishers[spectest.PublisherKey(intent)] = publishFunc(pub)

		case intent.Pattern == "service-response" && intent.Direction == "consume":
			setups = append(setups, ServiceResponseConsumer[json.RawMessage](intent.TargetService, intent.RoutingKey, captureHandler))

		default:
			t.Fatalf("unsupported setup intent: pattern=%s direction=%s", intent.Pattern, intent.Direction)
		}
	}

	conn, err := NewConnection(serviceName, a.url)
	spectest.RequireNoError(t, err)

	err = conn.Start(context.Background(), setups...)
	spectest.RequireNoError(t, err)

	return &spectest.ServiceHandle{
		Topology:   conn.Topology,
		Publishers: publishers,
		Received: func() []spectest.ReceivedMessage {
			mu.Lock()
			defer mu.Unlock()
			result := make([]spectest.ReceivedMessage, len(received))
			copy(result, received)
			return result
		},
		Close: conn.Close,
	}
}

func TestIntegrationTCK(t *testing.T) {
	wt := spectest.WrapT(t)
	scenarios := tck.LoadScenarios(wt, "../../specification/spec/testdata/tck.json")
	for _, scenario := range scenarios {
		t.Run(scenario.Name, func(t *testing.T) {
			s := startTestServer(t)
			adapter := &natsIntegrationAdapter{url: serverURL(s)}
			tck.RunScenario(spectest.WrapT(t), adapter, scenario)
		})
	}
}

func TestIntegrationTCKSubprocess(t *testing.T) {
	// Build the tck-adapter binary.
	binDir := t.TempDir()
	binPath := filepath.Join(binDir, "tck-adapter")
	build := exec.Command("go", "build", "-o", binPath, "./cmd/tck-adapter")
	build.Dir = "."
	out, err := build.CombinedOutput()
	require.NoError(t, err, "failed to build tck-adapter: %s", out)

	wt := spectest.WrapT(t)
	scenarios := tck.LoadScenarios(wt, "../../specification/spec/testdata/tck.json")
	for _, scenario := range scenarios {
		t.Run(scenario.Name, func(t *testing.T) {
			s := startTestServer(t)
			url := serverURL(s)
			t.Setenv("NATS_URL", url)

			adapter := tck.NewSubprocessAdapter(spectest.WrapT(t), binPath)
			tck.RunScenario(spectest.WrapT(t), adapter, scenario)
		})
	}
}

func publishFunc(pub *Publisher) spectest.PublishFunc {
	return func(ctx context.Context, routingKey string, payload json.RawMessage) error {
		return pub.Publish(ctx, routingKey, payload)
	}
}
