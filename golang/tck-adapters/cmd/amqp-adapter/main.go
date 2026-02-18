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

// Binary amqp-adapter is the reference AMQP adapter for the TCK subprocess protocol.
// It reads RABBITMQ_URL and RABBITMQ_MANAGEMENT_URL from the environment and serves
// JSON-RPC via stdin/fd 3.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"

	"github.com/sparetimecoders/gomessaging/amqp"
	"github.com/sparetimecoders/gomessaging/spec"
	"github.com/sparetimecoders/gomessaging/spec/spectest"
	"github.com/sparetimecoders/gomessaging/tck"
	"github.com/sparetimecoders/gomessaging/tck/adapterutil"
)

func main() {
	amqpURL := os.Getenv("RABBITMQ_URL")
	if amqpURL == "" {
		log.Fatal("RABBITMQ_URL environment variable is required")
	}

	managementURL := os.Getenv("RABBITMQ_MANAGEMENT_URL")
	if managementURL == "" {
		managementURL = "http://guest:guest@localhost:15672"
	}

	brokerConfig := tck.BrokerConfig{
		AMQPURL:       amqpURL,
		ManagementURL: managementURL,
	}
	mgr := &amqpServiceManager{url: amqpURL}

	if err := adapterutil.Serve("amqp", brokerConfig, mgr); err != nil {
		log.Fatalf("adapter: %v", err)
	}
}

type amqpServiceManager struct {
	url string
}

func (m *amqpServiceManager) StartService(serviceName string, intents []spectest.SetupIntent) (*adapterutil.ServiceState, error) {
	var mu sync.Mutex
	var received []tck.ReceivedMessageWire
	publishers := make(map[string]*amqp.Publisher)
	var publisherKeys []string

	captureHandler := func(_ context.Context, event spec.ConsumableEvent[json.RawMessage]) error {
		mu.Lock()
		defer mu.Unlock()
		received = append(received, tck.ReceivedMessageWire{
			RoutingKey:   event.DeliveryInfo.Key,
			Payload:      event.Payload,
			Metadata:     event.Metadata,
			DeliveryInfo: event.DeliveryInfo,
		})
		return nil
	}

	var setups []amqp.Setup
	for _, intent := range intents {
		switch {
		case intent.Pattern == "event-stream" && intent.Direction == "publish":
			pub := amqp.NewPublisher()
			setups = append(setups, amqp.EventStreamPublisher(pub))
			pk := spectest.PublisherKey(intent)
			publishers[pk] = pub
			publisherKeys = append(publisherKeys, pk)

		case intent.Pattern == "event-stream" && intent.Direction == "consume" && !intent.Ephemeral:
			setups = append(setups, amqp.EventStreamConsumer(intent.RoutingKey, captureHandler))

		case intent.Pattern == "event-stream" && intent.Direction == "consume" && intent.Ephemeral:
			setups = append(setups, amqp.TransientEventStreamConsumer(intent.RoutingKey, captureHandler))

		case intent.Pattern == "custom-stream" && intent.Direction == "publish":
			pub := amqp.NewPublisher()
			setups = append(setups, amqp.StreamPublisher(intent.Exchange, pub))
			pk := spectest.PublisherKey(intent)
			publishers[pk] = pub
			publisherKeys = append(publisherKeys, pk)

		case intent.Pattern == "custom-stream" && intent.Direction == "consume" && !intent.Ephemeral:
			setups = append(setups, amqp.StreamConsumer(intent.Exchange, intent.RoutingKey, captureHandler))

		case intent.Pattern == "custom-stream" && intent.Direction == "consume" && intent.Ephemeral:
			setups = append(setups, amqp.TransientStreamConsumer(intent.Exchange, intent.RoutingKey, captureHandler))

		case intent.Pattern == "service-request" && intent.Direction == "consume":
			setups = append(setups, amqp.ServiceRequestConsumer(intent.RoutingKey, captureHandler))

		case intent.Pattern == "service-request" && intent.Direction == "publish":
			pub := amqp.NewPublisher()
			setups = append(setups, amqp.ServicePublisher(intent.TargetService, pub))
			pk := spectest.PublisherKey(intent)
			publishers[pk] = pub
			publisherKeys = append(publisherKeys, pk)

		case intent.Pattern == "service-response" && intent.Direction == "consume":
			setups = append(setups, amqp.ServiceResponseConsumer[json.RawMessage](intent.TargetService, intent.RoutingKey, captureHandler))

		default:
			return nil, fmt.Errorf("unsupported intent: pattern=%s direction=%s", intent.Pattern, intent.Direction)
		}
	}

	conn, err := amqp.NewFromURL(serviceName, m.url)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}

	if err := conn.Start(context.Background(), setups...); err != nil {
		return nil, fmt.Errorf("start: %w", err)
	}

	return &adapterutil.ServiceState{
		Topology:      conn.Topology(),
		PublisherKeys: publisherKeys,
		Publish: func(publisherKey, routingKey string, payload json.RawMessage) error {
			pub, ok := publishers[publisherKey]
			if !ok {
				return fmt.Errorf("unknown publisher key: %s", publisherKey)
			}
			return pub.Publish(context.Background(), routingKey, payload)
		},
		Received: func() []tck.ReceivedMessageWire {
			mu.Lock()
			defer mu.Unlock()
			result := make([]tck.ReceivedMessageWire, len(received))
			copy(result, received)
			return result
		},
		Close: conn.Close,
	}, nil
}
