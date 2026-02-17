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
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	natsgo "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/sparetimecoders/gomessaging/spec"
	"github.com/sparetimecoders/gomessaging/spec/spectest"
	"github.com/stretchr/testify/require"
)

type natsIntegrationAdapter struct {
	url string
}

func (a *natsIntegrationAdapter) TransportKey() string { return "nats" }

func (a *natsIntegrationAdapter) StartService(t *testing.T, serviceName string, intents []spectest.SetupIntent) *spectest.ServiceHandle {
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
			setups = append(setups, ServiceRequestConsumer(intent.RoutingKey, captureHandler))

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
	require.NoError(t, err)

	err = conn.Start(context.Background(), setups...)
	require.NoError(t, err)

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

func (a *natsIntegrationAdapter) QueryBrokerState(t *testing.T) spectest.BrokerState {
	t.Helper()

	nc, err := natsgo.Connect(a.url)
	require.NoError(t, err)
	defer nc.Close()

	js, err := jetstream.New(nc)
	require.NoError(t, err)

	ctx := context.Background()

	var streams []spectest.NATSStream
	streamLister := js.ListStreams(ctx)
	for si := range streamLister.Info() {
		subjects := make([]string, len(si.Config.Subjects))
		copy(subjects, si.Config.Subjects)
		streams = append(streams, spectest.NATSStream{
			Name:     si.Config.Name,
			Subjects: subjects,
			Storage:  strings.ToLower(si.Config.Storage.String()),
		})
	}
	require.NoError(t, streamLister.Err())

	var consumers []spectest.NATSConsumer
	for _, s := range streams {
		stream, err := js.Stream(ctx, s.Name)
		require.NoError(t, err)

		consLister := stream.ListConsumers(ctx)
		for ci := range consLister.Info() {
			nc := spectest.NATSConsumer{
				Stream:    s.Name,
				AckPolicy: ackPolicyString(ci.Config.AckPolicy),
			}
			if ci.Config.Durable != "" {
				nc.Durable = ci.Config.Durable
			}
			if ci.Config.FilterSubject != "" {
				nc.FilterSubject = ci.Config.FilterSubject
			}
			if len(ci.Config.FilterSubjects) > 0 {
				nc.FilterSubjects = ci.Config.FilterSubjects
			}
			consumers = append(consumers, nc)
		}
		require.NoError(t, consLister.Err())
	}

	return spectest.BrokerState{
		NATS: spectest.NATSBrokerState{
			Streams:   streams,
			Consumers: consumers,
		},
	}
}

func (a *natsIntegrationAdapter) PublishRaw(t *testing.T, target spectest.ProbeTarget, payload json.RawMessage, headers map[string]string) error {
	t.Helper()
	nc, err := natsgo.Connect(a.url)
	require.NoError(t, err)
	defer nc.Close()

	js, err := jetstream.New(nc)
	require.NoError(t, err)

	natsHeaders := natsgo.Header{}
	for attr, val := range headers {
		natsHeaders.Set("ce-"+attr, val)
	}
	natsHeaders.Set(spec.CEID, uuid.New().String())
	natsHeaders.Set(spec.CETime, time.Now().UTC().Format(time.RFC3339))

	msg := &natsgo.Msg{
		Subject: target.Subject,
		Data:    payload,
		Header:  natsHeaders,
	}
	_, err = js.PublishMsg(context.Background(), msg)
	return err
}

func (a *natsIntegrationAdapter) CreateProbeConsumer(t *testing.T, target spectest.ProbeTarget) *spectest.ProbeConsumer {
	t.Helper()
	nc, err := natsgo.Connect(a.url)
	require.NoError(t, err)

	sub, err := nc.SubscribeSync(target.Subject)
	require.NoError(t, err)
	require.NoError(t, nc.Flush())

	return &spectest.ProbeConsumer{
		Receive: func(timeout time.Duration) *spectest.RawMessage {
			msg, err := sub.NextMsg(timeout)
			if err != nil {
				return nil
			}
			hdrs := make(map[string]string)
			for k := range msg.Header {
				v := msg.Header.Get(k)
				if strings.HasPrefix(k, "ce-") {
					hdrs[strings.TrimPrefix(k, "ce-")] = v
				}
			}
			return &spectest.RawMessage{
				Payload: msg.Data,
				Headers: hdrs,
			}
		},
		Close: func() {
			_ = sub.Unsubscribe()
			nc.Close()
		},
	}
}

func TestIntegrationTCK(t *testing.T) {
	scenarios := spectest.LoadTCKScenarios(t, "../../specification/spec/testdata/tck.json")
	for _, scenario := range scenarios {
		t.Run(scenario.Name, func(t *testing.T) {
			s := startTestServer(t)
			adapter := &natsIntegrationAdapter{url: serverURL(s)}
			spectest.RunIntegrationTCKScenario(t, adapter, scenario)
		})
	}
}

func publishFunc(pub *Publisher) spectest.PublishFunc {
	return func(ctx context.Context, routingKey string, payload json.RawMessage) error {
		return pub.Publish(ctx, routingKey, payload)
	}
}

func ackPolicyString(p jetstream.AckPolicy) string {
	switch p {
	case jetstream.AckExplicitPolicy:
		return "explicit"
	case jetstream.AckNonePolicy:
		return "none"
	case jetstream.AckAllPolicy:
		return "all"
	default:
		return "unknown"
	}
}
