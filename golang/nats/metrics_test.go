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
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
)

func TestInitMetrics(t *testing.T) {
	registry := prometheus.NewRegistry()
	err := InitMetrics(registry)
	assert.NoError(t, err)

	// Trigger all metrics so they appear in Gather
	eventReceived("test", "key")
	eventAck("test", "key", 10)
	eventNak("test", "key", 10)
	eventWithoutHandler("test", "key")
	eventNotParsable("test", "key")
	eventPublishSucceed("test", "key", 10)
	eventPublishFailed("test", "key", 10)

	families, err := registry.Gather()
	assert.NoError(t, err)

	names := make(map[string]bool)
	for _, f := range families {
		names[f.GetName()] = true
	}

	assert.True(t, names["nats_events_received"])
	assert.True(t, names["nats_events_ack"])
	assert.True(t, names["nats_events_nak"])
	assert.True(t, names["nats_events_without_handler"])
	assert.True(t, names["nats_events_not_parsable"])
	assert.True(t, names["nats_events_processed_duration"])
	assert.True(t, names["nats_events_publish_succeed"])
	assert.True(t, names["nats_events_publish_failed"])
	assert.True(t, names["nats_events_publish_duration"])
}

func TestInitMetricsIdempotent(t *testing.T) {
	registry := prometheus.NewRegistry()
	err := InitMetrics(registry)
	assert.NoError(t, err)

	err = InitMetrics(registry)
	assert.NoError(t, err)
}

func TestInitMetrics_DefaultMapper(t *testing.T) {
	t.Cleanup(func() { routingKeyMapperVal.Store(func(s string) string { return s }) })
	registry := prometheus.NewRegistry()
	assert.NoError(t, InitMetrics(registry)) // no options — identity mapper
	eventReceived("consumer", "Order.Created")
	families, err := registry.Gather()
	assert.NoError(t, err)
	found := false
	for _, f := range families {
		if f.GetName() == "nats_events_received" {
			for _, m := range f.GetMetric() {
				for _, lp := range m.GetLabel() {
					if *lp.Name == "routing_key" {
						assert.Equal(t, "Order.Created", *lp.Value)
						found = true
					}
				}
			}
		}
	}
	assert.True(t, found, "routing_key label not found in gathered metrics")
}

func TestInitMetrics_WithRoutingKeyMapper(t *testing.T) {
	t.Cleanup(func() { routingKeyMapperVal.Store(func(s string) string { return s }) })
	registry := prometheus.NewRegistry()
	assert.NoError(t, InitMetrics(registry, WithRoutingKeyMapper(func(k string) string {
		return "mapped." + k
	})))

	eventReceived("consumer", "Order.Created")
	families, err := registry.Gather()
	assert.NoError(t, err)
	found := false
	for _, f := range families {
		if f.GetName() == "nats_events_received" {
			for _, m := range f.GetMetric() {
				for _, lp := range m.GetLabel() {
					if *lp.Name == "routing_key" {
						assert.Equal(t, "mapped.Order.Created", *lp.Value)
						found = true
					}
				}
			}
		}
	}
	assert.True(t, found, "routing_key label not found in gathered metrics")
}

func TestInitMetrics_PublisherUsesRoutingKeyValue(t *testing.T) {
	t.Cleanup(func() { routingKeyMapperVal.Store(func(s string) string { return s }) })
	registry := prometheus.NewRegistry()
	assert.NoError(t, InitMetrics(registry))

	// Pass routing key (not full subject) — verifies the publisher fix
	eventPublishSucceed("events", "Order.Created", 10)
	families, err := registry.Gather()
	assert.NoError(t, err)
	found := false
	for _, f := range families {
		if f.GetName() == "nats_events_publish_succeed" {
			for _, m := range f.GetMetric() {
				for _, lp := range m.GetLabel() {
					if *lp.Name == "routing_key" {
						assert.Equal(t, "Order.Created", *lp.Value)
						found = true
					}
				}
			}
		}
	}
	assert.True(t, found, "routing_key label not found in gathered metrics")
}

func TestInitMetrics_EmptyMapperReturnsFallback(t *testing.T) {
	t.Cleanup(func() { routingKeyMapperVal.Store(func(s string) string { return s }) })
	registry := prometheus.NewRegistry()
	assert.NoError(t, InitMetrics(registry, WithRoutingKeyMapper(func(string) string {
		return ""
	})))

	eventReceived("consumer", "Order.Created")
	families, err := registry.Gather()
	assert.NoError(t, err)
	found := false
	for _, f := range families {
		if f.GetName() == "nats_events_received" {
			for _, m := range f.GetMetric() {
				for _, lp := range m.GetLabel() {
					if *lp.Name == "routing_key" {
						assert.Equal(t, "unknown", *lp.Value)
						found = true
					}
				}
			}
		}
	}
	assert.True(t, found, "routing_key label not found in gathered metrics")
}
