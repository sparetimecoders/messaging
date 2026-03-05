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

package amqp

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"

	"github.com/sparetimecoders/messaging/specification/spec"
	"github.com/sparetimecoders/messaging/specification/spec/spectest"
	"github.com/sparetimecoders/messaging/specification/tck"
	"github.com/stretchr/testify/require"
)

type amqpIntegrationAdapter struct {
	amqpURL       string
	managementURL string
}

func (a *amqpIntegrationAdapter) TransportKey() string { return "amqp" }

func (a *amqpIntegrationAdapter) BrokerConfig() tck.BrokerConfig {
	return tck.BrokerConfig{
		AMQPURL:       a.amqpURL,
		ManagementURL: a.managementURL,
	}
}

func (a *amqpIntegrationAdapter) StartService(t spectest.T, serviceName string, intents []spectest.SetupIntent) *spectest.ServiceHandle {
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

	var conn *Connection
	var setups []Setup
	for _, intent := range intents {
		switch {
		case intent.Pattern == "event-stream" && intent.Direction == "publish":
			pub := NewPublisher()
			setups = append(setups, EventStreamPublisher(pub))
			publishers[spectest.PublisherKey(intent)] = amqpPublishFunc(pub)

		case intent.Pattern == "event-stream" && intent.Direction == "consume" && !intent.Ephemeral && intent.QueueSuffix != "":
			setups = append(setups, EventStreamConsumer(intent.RoutingKey, captureHandler, AddQueueNameSuffix(intent.QueueSuffix)))

		case intent.Pattern == "event-stream" && intent.Direction == "consume" && !intent.Ephemeral:
			setups = append(setups, EventStreamConsumer(intent.RoutingKey, captureHandler))

		case intent.Pattern == "event-stream" && intent.Direction == "consume" && intent.Ephemeral:
			setups = append(setups, TransientEventStreamConsumer(intent.RoutingKey, captureHandler))

		case intent.Pattern == "custom-stream" && intent.Direction == "publish":
			pub := NewPublisher()
			setups = append(setups, StreamPublisher(intent.Exchange, pub))
			publishers[spectest.PublisherKey(intent)] = amqpPublishFunc(pub)

		case intent.Pattern == "custom-stream" && intent.Direction == "consume" && !intent.Ephemeral:
			setups = append(setups, StreamConsumer(intent.Exchange, intent.RoutingKey, captureHandler))

		case intent.Pattern == "custom-stream" && intent.Direction == "consume" && intent.Ephemeral:
			setups = append(setups, TransientStreamConsumer(intent.Exchange, intent.RoutingKey, captureHandler))

		case intent.Pattern == "service-request" && intent.Direction == "consume":
			setups = append(setups, ServiceRequestConsumer(intent.RoutingKey, captureHandler))

		case intent.Pattern == "service-request" && intent.Direction == "publish":
			pub := NewPublisher()
			setups = append(setups, ServicePublisher(intent.TargetService, pub))
			publishers[spectest.PublisherKey(intent)] = amqpPublishFunc(pub)

		case intent.Pattern == "service-response" && intent.Direction == "consume":
			setups = append(setups, ServiceResponseConsumer[json.RawMessage](intent.TargetService, intent.RoutingKey, captureHandler))

		case intent.Pattern == "service-response" && intent.Direction == "publish":
			// service-response publish is handled via conn.PublishServiceResponse.
			// Register a publisher that extracts the target from headers.
			pk := spectest.PublisherKey(intent)
			publishers[pk] = func(ctx context.Context, routingKey string, payload json.RawMessage, headers map[string]string) error {
				targetService := headers["_tckTargetService"]
				return conn.PublishServiceResponse(ctx, targetService, routingKey, payload)
			}

		case intent.Pattern == "queue-publish" && intent.Direction == "publish":
			pub := NewPublisher()
			setups = append(setups, QueuePublisher(pub, intent.DestinationQueue))
			publishers[spectest.PublisherKey(intent)] = amqpPublishFunc(pub)

		default:
			t.Fatalf("unsupported setup intent: pattern=%s direction=%s", intent.Pattern, intent.Direction)
		}
	}

	var err error
	conn, err = NewFromURL(serviceName, a.amqpURL)
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
	amqpURL := os.Getenv("RABBITMQ_URL")
	if amqpURL == "" {
		t.Skip("RABBITMQ_URL not set, skipping AMQP integration TCK")
	}
	managementURL := os.Getenv("RABBITMQ_MANAGEMENT_URL")
	if managementURL == "" {
		managementURL = "http://guest:guest@localhost:15672"
	}

	adapter := &amqpIntegrationAdapter{
		amqpURL:       amqpURL,
		managementURL: managementURL,
	}
	tck.RunTCK(spectest.WrapT(t), "../../specification/spec/testdata/tck.json", adapter)
}

func TestIntegrationTCKSubprocess(t *testing.T) {
	amqpURL := os.Getenv("RABBITMQ_URL")
	if amqpURL == "" {
		t.Skip("RABBITMQ_URL not set, skipping AMQP subprocess TCK")
	}

	// Build the amqp-adapter binary from the tck-adapters module.
	binDir := t.TempDir()
	binPath := filepath.Join(binDir, "amqp-adapter")
	build := exec.Command("go", "build", "-o", binPath, "github.com/sparetimecoders/messaging/golang/tck-adapters/cmd/amqp-adapter")
	out, err := build.CombinedOutput()
	require.NoError(t, err, "failed to build amqp-adapter: %s", out)

	wt := spectest.WrapT(t)
	scenarios := tck.LoadScenarios(wt, "../../specification/spec/testdata/tck.json")
	for _, scenario := range scenarios {
		t.Run(scenario.Name, func(t *testing.T) {
			adapter := tck.NewSubprocessAdapter(spectest.WrapT(t), binPath)
			tck.RunScenario(spectest.WrapT(t), adapter, scenario)
		})
	}
}

func amqpPublishFunc(pub *Publisher) spectest.PublishFunc {
	return func(ctx context.Context, routingKey string, payload json.RawMessage, headers map[string]string) error {
		var hdrs []Header
		for k, v := range headers {
			hdrs = append(hdrs, Header{Key: k, Value: v})
		}
		return pub.Publish(ctx, routingKey, payload, hdrs...)
	}
}
