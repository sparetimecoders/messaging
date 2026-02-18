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

// Binary nats-adapter is the reference NATS adapter for the TCK subprocess protocol.
// It reads NATS_URL from the environment and serves JSON-RPC via stdin/fd 3.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"

	"github.com/sparetimecoders/gomessaging/nats"
	"github.com/sparetimecoders/gomessaging/spec"
	"github.com/sparetimecoders/gomessaging/spec/spectest"
	"github.com/sparetimecoders/gomessaging/tck"
	"github.com/sparetimecoders/gomessaging/tck/adapterutil"
)

func main() {
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		log.Fatal("NATS_URL environment variable is required")
	}

	brokerConfig := tck.BrokerConfig{NATSURL: natsURL}
	mgr := &natsServiceManager{url: natsURL}

	if err := adapterutil.Serve("nats", brokerConfig, mgr); err != nil {
		log.Fatalf("adapter: %v", err)
	}
}

type natsServiceManager struct {
	url string
}

func (m *natsServiceManager) StartService(serviceName string, intents []spectest.SetupIntent) (*adapterutil.ServiceState, error) {
	var mu sync.Mutex
	var received []tck.ReceivedMessageWire
	publishers := make(map[string]*nats.Publisher)
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

	var setups []nats.Setup
	for _, intent := range intents {
		switch {
		case intent.Pattern == "event-stream" && intent.Direction == "publish":
			pub := nats.NewPublisher()
			setups = append(setups, nats.EventStreamPublisher(pub))
			pk := spectest.PublisherKey(intent)
			publishers[pk] = pub
			publisherKeys = append(publisherKeys, pk)

		case intent.Pattern == "event-stream" && intent.Direction == "consume" && !intent.Ephemeral && intent.QueueSuffix != "":
			setups = append(setups, nats.EventStreamConsumer(intent.RoutingKey, captureHandler, nats.AddConsumerNameSuffix(intent.QueueSuffix)))

		case intent.Pattern == "event-stream" && intent.Direction == "consume" && !intent.Ephemeral:
			setups = append(setups, nats.EventStreamConsumer(intent.RoutingKey, captureHandler))

		case intent.Pattern == "event-stream" && intent.Direction == "consume" && intent.Ephemeral:
			setups = append(setups, nats.TransientEventStreamConsumer(intent.RoutingKey, captureHandler))

		case intent.Pattern == "custom-stream" && intent.Direction == "publish":
			pub := nats.NewPublisher()
			setups = append(setups, nats.StreamPublisher(intent.Exchange, pub))
			pk := spectest.PublisherKey(intent)
			publishers[pk] = pub
			publisherKeys = append(publisherKeys, pk)

		case intent.Pattern == "custom-stream" && intent.Direction == "consume" && !intent.Ephemeral:
			setups = append(setups, nats.StreamConsumer(intent.Exchange, intent.RoutingKey, captureHandler))

		case intent.Pattern == "custom-stream" && intent.Direction == "consume" && intent.Ephemeral:
			setups = append(setups, nats.TransientStreamConsumer(intent.Exchange, intent.RoutingKey, captureHandler))

		case intent.Pattern == "service-request" && intent.Direction == "consume":
			rrHandler := func(_ context.Context, event spec.ConsumableEvent[json.RawMessage]) (json.RawMessage, error) {
				mu.Lock()
				defer mu.Unlock()
				received = append(received, tck.ReceivedMessageWire{
					RoutingKey:   event.DeliveryInfo.Key,
					Payload:      event.Payload,
					Metadata:     event.Metadata,
					DeliveryInfo: event.DeliveryInfo,
				})
				return json.RawMessage(`{"ok":true}`), nil
			}
			setups = append(setups, nats.RequestResponseHandler[json.RawMessage, json.RawMessage](intent.RoutingKey, rrHandler))

		case intent.Pattern == "service-request" && intent.Direction == "publish":
			pub := nats.NewPublisher()
			setups = append(setups, nats.ServicePublisher(intent.TargetService, pub))
			pk := spectest.PublisherKey(intent)
			publishers[pk] = pub
			publisherKeys = append(publisherKeys, pk)

		case intent.Pattern == "service-response" && intent.Direction == "consume":
			setups = append(setups, nats.ServiceResponseConsumer[json.RawMessage](intent.TargetService, intent.RoutingKey, captureHandler))

		case intent.Pattern == "service-response" && intent.Direction == "publish":
			// NATS service responses are sent via core NATS reply subject — no setup needed.
			pk := spectest.PublisherKey(intent)
			publisherKeys = append(publisherKeys, pk)

		default:
			return nil, fmt.Errorf("unsupported intent: pattern=%s direction=%s", intent.Pattern, intent.Direction)
		}
	}

	conn, err := nats.NewConnection(serviceName, m.url)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}

	if err := conn.Start(context.Background(), setups...); err != nil {
		return nil, fmt.Errorf("start: %w", err)
	}

	return &adapterutil.ServiceState{
		Topology:      conn.Topology(),
		PublisherKeys: publisherKeys,
		Publish: func(publisherKey, routingKey string, payload json.RawMessage, headers map[string]string) error {
			// service-response publish: NATS handles via reply subject, no-op.
			if strings.HasPrefix(publisherKey, "service-response:") {
				return nil
			}
			pub, ok := publishers[publisherKey]
			if !ok {
				return fmt.Errorf("unknown publisher key: %s", publisherKey)
			}
			var hdrs []nats.Header
			for k, v := range headers {
				hdrs = append(hdrs, nats.Header{Key: k, Value: v})
			}
			return pub.Publish(context.Background(), routingKey, payload, hdrs...)
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
