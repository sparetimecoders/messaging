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
	"testing"

	"github.com/sparetimecoders/gomessaging/spec"
	"github.com/sparetimecoders/gomessaging/spec/spectest"
	"github.com/stretchr/testify/require"
)

func TestTopologyConformance(t *testing.T) {
	scenarios := spectest.LoadScenarios(t, "../../specification/spec/testdata/topology.json")

	for _, scenario := range scenarios {
		expected, ok := scenario.ExpectedEndpoints["amqp"]
		if !ok {
			continue
		}
		t.Run(scenario.Name, func(t *testing.T) {
			channel := NewMockAmqpChannel()
			conn := mockConnection(channel)
			conn.serviceName = scenario.ServiceName
			conn.topology = spec.Topology{Transport: spec.TransportAMQP, ServiceName: scenario.ServiceName}

			setups := mapIntentsToSetups(t, scenario.Setups)
			err := conn.Start(context.Background(), setups...)
			require.NoError(t, err)

			topo := conn.Topology()
			require.Equal(t, scenario.ServiceName, topo.ServiceName)
			spectest.AssertTopology(t, expected, topo)
		})
	}
}

func mapIntentsToSetups(t *testing.T, intents []spectest.SetupIntent) []Setup {
	t.Helper()
	var setups []Setup
	for _, intent := range intents {
		setups = append(setups, intentToSetup(t, intent))
	}
	return setups
}

func intentToSetup(t *testing.T, intent spectest.SetupIntent) Setup {
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
