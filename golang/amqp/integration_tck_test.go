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
	"sync"
	"testing"

	"github.com/sparetimecoders/gomessaging/spec"
	"github.com/sparetimecoders/gomessaging/spec/spectest"
	"github.com/sparetimecoders/gomessaging/tck"
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

	var setups []Setup
	for _, intent := range intents {
		switch {
		case intent.Pattern == "event-stream" && intent.Direction == "publish":
			pub := NewPublisher()
			setups = append(setups, EventStreamPublisher(pub))
			publishers[spectest.PublisherKey(intent)] = amqpPublishFunc(pub)

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

		default:
			t.Fatalf("unsupported setup intent: pattern=%s direction=%s", intent.Pattern, intent.Direction)
		}
	}

	conn, err := NewFromURL(serviceName, a.amqpURL)
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

func amqpPublishFunc(pub *Publisher) spectest.PublishFunc {
	return func(ctx context.Context, routingKey string, payload json.RawMessage) error {
		return pub.Publish(ctx, routingKey, payload)
	}
}
