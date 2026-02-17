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
	"io"
	"log/slog"
	"testing"

	"github.com/sparetimecoders/gomessaging/spec"
	"github.com/sparetimecoders/gomessaging/spec/spectest"
	"github.com/stretchr/testify/require"
)

func TestTopologyConformance(t *testing.T) {
	scenarios := spectest.LoadScenarios(t, "../../specification/spec/testdata/topology.json")

	for _, scenario := range scenarios {
		expected, ok := scenario.ExpectedEndpoints["nats"]
		if !ok {
			continue
		}
		t.Run(scenario.Name, func(t *testing.T) {
			c := &Connection{
				serviceName: scenario.ServiceName,
				logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
				topology:    spec.Topology{Transport: spec.TransportNATS, ServiceName: scenario.ServiceName},
				collectMode: true,
			}

			setups := natsIntentsToSetups(t, scenario.Setups)
			for _, s := range setups {
				require.NoError(t, s(c))
			}

			topo := c.Topology()
			require.Equal(t, scenario.ServiceName, topo.ServiceName)
			spectest.AssertTopology(t, expected, topo)

			actualBroker := deriveNATSBrokerState(c)
			spectest.AssertNATSBrokerState(t, scenario.Broker.NATS, actualBroker)
		})
	}
}

// deriveNATSBrokerState builds NATSBrokerState from Connection's collected data,
// mirroring the grouping logic of startPendingJSConsumers.
func deriveNATSBrokerState(c *Connection) spectest.NATSBrokerState {
	// Collect all unique stream names from publishers and consumers.
	seen := make(map[string]bool)
	var streamNames []string
	for _, name := range c.pendingStreams {
		if !seen[name] {
			seen[name] = true
			streamNames = append(streamNames, name)
		}
	}
	for _, cfg := range c.pendingJSConsumers {
		if !seen[cfg.stream] {
			seen[cfg.stream] = true
			streamNames = append(streamNames, cfg.stream)
		}
	}

	streams := make([]spectest.NATSStream, 0, len(streamNames))
	for _, name := range streamNames {
		streams = append(streams, spectest.NATSStream{
			Name:     name,
			Subjects: []string{streamSubjects(name)},
			Storage:  "file",
		})
	}

	// Group consumer configs by stream+consumerName (same key as startPendingJSConsumers).
	type group struct {
		configs []*consumerConfig
	}
	groups := make(map[string]*group)
	var order []string
	for _, cfg := range c.pendingJSConsumers {
		key := consumerGroupKey(cfg)
		g, exists := groups[key]
		if !exists {
			g = &group{}
			groups[key] = g
			order = append(order, key)
		}
		g.configs = append(g.configs, cfg)
	}

	consumers := make([]spectest.NATSConsumer, 0, len(order))
	for _, key := range order {
		g := groups[key]
		first := g.configs[0]

		var filterSubjects []string
		for _, cfg := range g.configs {
			filterSubjects = append(filterSubjects, filterSubject(cfg.stream, cfg.routingKey))
		}

		nc := spectest.NATSConsumer{
			Stream:    first.stream,
			AckPolicy: "explicit",
		}
		if !first.ephemeral {
			nc.Durable = first.consumerName
		}
		if len(filterSubjects) == 1 {
			nc.FilterSubject = filterSubjects[0]
		} else {
			nc.FilterSubjects = filterSubjects
		}
		consumers = append(consumers, nc)
	}

	return spectest.NATSBrokerState{
		Streams:   streams,
		Consumers: consumers,
	}
}

func natsIntentsToSetups(t *testing.T, intents []spectest.SetupIntent) []Setup {
	t.Helper()
	var setups []Setup
	for _, intent := range intents {
		setups = append(setups, natsIntentToSetup(t, intent))
	}
	return setups
}

func natsIntentToSetup(t *testing.T, intent spectest.SetupIntent) Setup {
	t.Helper()
	switch {
	case intent.Pattern == "event-stream" && intent.Direction == "publish":
		return EventStreamPublisher(NewPublisher())

	case intent.Pattern == "event-stream" && intent.Direction == "consume" && !intent.Ephemeral:
		return EventStreamConsumer(intent.RoutingKey, func(ctx context.Context, msg spec.ConsumableEvent[json.RawMessage]) error {
			return nil
		})

	case intent.Pattern == "event-stream" && intent.Direction == "consume" && intent.Ephemeral:
		return TransientEventStreamConsumer(intent.RoutingKey, func(ctx context.Context, msg spec.ConsumableEvent[json.RawMessage]) error {
			return nil
		})

	case intent.Pattern == "custom-stream" && intent.Direction == "publish":
		return StreamPublisher(intent.Exchange, NewPublisher())

	case intent.Pattern == "custom-stream" && intent.Direction == "consume" && !intent.Ephemeral:
		return StreamConsumer(intent.Exchange, intent.RoutingKey, func(ctx context.Context, msg spec.ConsumableEvent[json.RawMessage]) error {
			return nil
		})

	case intent.Pattern == "custom-stream" && intent.Direction == "consume" && intent.Ephemeral:
		return TransientStreamConsumer(intent.Exchange, intent.RoutingKey, func(ctx context.Context, msg spec.ConsumableEvent[json.RawMessage]) error {
			return nil
		})

	case intent.Pattern == "service-request" && intent.Direction == "consume":
		return ServiceRequestConsumer(intent.RoutingKey, func(ctx context.Context, msg spec.ConsumableEvent[json.RawMessage]) error {
			return nil
		})

	case intent.Pattern == "service-request" && intent.Direction == "publish":
		return ServicePublisher(intent.TargetService, NewPublisher())

	case intent.Pattern == "service-response" && intent.Direction == "consume":
		return ServiceResponseConsumer[json.RawMessage](intent.TargetService, intent.RoutingKey, func(ctx context.Context, msg spec.ConsumableEvent[json.RawMessage]) error {
			return nil
		})

	default:
		t.Fatalf("unsupported setup intent: pattern=%s direction=%s", intent.Pattern, intent.Direction)
		return nil
	}
}
