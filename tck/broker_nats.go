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
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	natsgo "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/sparetimecoders/messaging"
	"github.com/sparetimecoders/messaging/spectest"
)

// natsBrokerClient implements BrokerClient for NATS JetStream.
type natsBrokerClient struct {
	url string
}

func (c *natsBrokerClient) QueryState(t spectest.T) spectest.BrokerState {
	t.Helper()

	nc, err := natsgo.Connect(c.url)
	spectest.RequireNoError(t, err)
	defer nc.Close()

	js, err := jetstream.New(nc)
	spectest.RequireNoError(t, err)

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
	spectest.RequireNoError(t, streamLister.Err())

	var consumers []spectest.NATSConsumer
	for _, s := range streams {
		stream, err := js.Stream(ctx, s.Name)
		spectest.RequireNoError(t, err)

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
		spectest.RequireNoError(t, consLister.Err())
	}

	return spectest.BrokerState{
		NATS: spectest.NATSBrokerState{
			Streams:   streams,
			Consumers: consumers,
		},
	}
}

func (c *natsBrokerClient) PublishRaw(t spectest.T, target spectest.ProbeTarget, payload json.RawMessage, headers map[string]string) error {
	t.Helper()
	nc, err := natsgo.Connect(c.url)
	spectest.RequireNoError(t, err)
	defer nc.Close()

	js, err := jetstream.New(nc)
	spectest.RequireNoError(t, err)

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

func (c *natsBrokerClient) CreateProbeConsumer(t spectest.T, target spectest.ProbeTarget) *spectest.ProbeConsumer {
	t.Helper()
	nc, err := natsgo.Connect(c.url)
	spectest.RequireNoError(t, err)

	sub, err := nc.SubscribeSync(target.Subject)
	spectest.RequireNoError(t, err)
	spectest.RequireNoError(t, nc.Flush())

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

func (c *natsBrokerClient) Cleanup(t spectest.T) {
	t.Helper()

	nc, err := natsgo.Connect(c.url)
	spectest.RequireNoError(t, err)
	defer nc.Close()

	js, err := jetstream.New(nc)
	spectest.RequireNoError(t, err)

	ctx := context.Background()
	streamLister := js.ListStreams(ctx)
	for si := range streamLister.Info() {
		err := js.DeleteStream(ctx, si.Config.Name)
		spectest.RequireNoError(t, err)
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
