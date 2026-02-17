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
	"errors"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
)

type failingRegisterer struct{}

func (f failingRegisterer) Register(prometheus.Collector) error {
	return errors.New("registration failed")
}
func (f failingRegisterer) MustRegister(...prometheus.Collector) {}
func (f failingRegisterer) Unregister(prometheus.Collector) bool { return false }

func Test_InitMetrics_RegistrationError(t *testing.T) {
	err := InitMetrics(failingRegisterer{})
	require.EqualError(t, err, "registration failed")
}

func Test_InitMetrics_Idempotent(t *testing.T) {
	registry := prometheus.NewRegistry()
	require.NoError(t, InitMetrics(registry))
	require.NoError(t, InitMetrics(registry))
}

func Test_Metrics(t *testing.T) {
	registry := prometheus.NewRegistry()
	require.NoError(t, InitMetrics(registry))
	channel := NewMockAmqpChannel()

	err := publishMessage(context.Background(), nil, nil, nil, channel, Message{true}, "key", "exchange", "test-service", nil)
	require.NoError(t, err)
	metricFamilies, err := registry.Gather()
	require.NoError(t, err)
	var publishedSuccessfully float64
	for _, metric := range metricFamilies {
		if *metric.Name == "amqp_events_publish_succeed" {
			publishedSuccessfully = *metric.GetMetric()[0].GetCounter().Value
		}
	}
	require.Equal(t, 1.0, publishedSuccessfully)
}

func Test_InitMetrics_DefaultMapper(t *testing.T) {
	t.Cleanup(func() { routingKeyMapperVal.Store(func(s string) string { return s }) })
	registry := prometheus.NewRegistry()
	require.NoError(t, InitMetrics(registry)) // no options — identity mapper
	eventReceived("queue", "Order.Created")
	families, err := registry.Gather()
	require.NoError(t, err)
	found := false
	for _, f := range families {
		if f.GetName() == "amqp_events_received" {
			for _, m := range f.GetMetric() {
				for _, lp := range m.GetLabel() {
					if *lp.Name == "routing_key" {
						require.Equal(t, "Order.Created", *lp.Value)
						found = true
					}
				}
			}
		}
	}
	require.True(t, found, "routing_key label not found in gathered metrics")
}

func Test_InitMetrics_WithRoutingKeyMapper(t *testing.T) {
	t.Cleanup(func() { routingKeyMapperVal.Store(func(s string) string { return s }) })
	registry := prometheus.NewRegistry()
	require.NoError(t, InitMetrics(registry, WithRoutingKeyMapper(func(k string) string {
		return "mapped." + k
	})))

	channel := NewMockAmqpChannel()
	err := publishMessage(context.Background(), nil, nil, nil, channel, Message{true}, "Order.Created", "exchange", "svc", nil)
	require.NoError(t, err)

	families, err := registry.Gather()
	require.NoError(t, err)
	found := false
	for _, f := range families {
		if f.GetName() == "amqp_events_publish_succeed" {
			for _, m := range f.GetMetric() {
				for _, lp := range m.GetLabel() {
					if *lp.Name == "routing_key" {
						require.Equal(t, "mapped.Order.Created", *lp.Value)
						found = true
					}
				}
			}
		}
	}
	require.True(t, found, "routing_key label not found in gathered metrics")
}

func Test_InitMetrics_EmptyMapperReturnsFallback(t *testing.T) {
	t.Cleanup(func() { routingKeyMapperVal.Store(func(s string) string { return s }) })
	registry := prometheus.NewRegistry()
	require.NoError(t, InitMetrics(registry, WithRoutingKeyMapper(func(string) string {
		return ""
	})))

	eventReceived("queue", "Order.Created")
	families, err := registry.Gather()
	require.NoError(t, err)
	found := false
	for _, f := range families {
		if f.GetName() == "amqp_events_received" {
			for _, m := range f.GetMetric() {
				for _, lp := range m.GetLabel() {
					if *lp.Name == "routing_key" {
						require.Equal(t, "unknown", *lp.Value)
						found = true
					}
				}
			}
		}
	}
	require.True(t, found, "routing_key label not found in gathered metrics")
}
