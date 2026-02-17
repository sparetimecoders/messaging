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
	"errors"
	"sync/atomic"

	"github.com/prometheus/client_golang/prometheus"
)

// MetricsOption configures Prometheus metric behavior.
type MetricsOption func(*metricsConfig)

type metricsConfig struct {
	routingKeyMapper func(string) string
}

// WithRoutingKeyMapper sets a function applied to every routing key before it is
// used as a Prometheus label value. Use this to normalize or redact dynamic
// segments (e.g. UUIDs) from routing keys to prevent unbounded label cardinality.
// The default is the identity function (labels pass through unchanged).
// The mapper must return a non-empty string; empty returns are replaced with "unknown".
func WithRoutingKeyMapper(fn func(string) string) MetricsOption {
	return func(cfg *metricsConfig) {
		cfg.routingKeyMapper = fn
	}
}

// routingKeyMapperVal holds the active routing key mapper function.
// Stored in atomic.Value for goroutine safety — reads from hot-path metric
// helpers are lock-free.
var routingKeyMapperVal atomic.Value // stores func(string) string

func init() {
	routingKeyMapperVal.Store(func(s string) string { return s })
}

func routingKeyLabel(key string) string {
	mapped := routingKeyMapperVal.Load().(func(string) string)(key)
	if mapped == "" {
		return "unknown"
	}
	return mapped
}

const (
	metricQueue      = "queue"
	metricExchange   = "exchange"
	metricResult     = "result"
	metricRoutingKey = "routing_key"
)

var (
	eventReceivedCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "amqp_events_received",
			Help: "Count of AMQP events received",
		}, []string{metricQueue, metricRoutingKey},
	)

	eventWithoutHandlerCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "amqp_events_without_handler",
			Help: "Count of AMQP events without a handler",
		}, []string{metricQueue, metricRoutingKey},
	)

	eventNotParsableCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "amqp_events_not_parsable",
			Help: "Count of AMQP events that could not be parsed",
		}, []string{metricQueue, metricRoutingKey},
	)

	eventNackCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "amqp_events_nack",
			Help: "Count of AMQP events that were not acknowledged",
		}, []string{metricQueue, metricRoutingKey},
	)

	eventAckCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "amqp_events_ack",
			Help: "Count of AMQP events that were acknowledged",
		}, []string{metricQueue, metricRoutingKey},
	)

	eventProcessedDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "amqp_events_processed_duration",
			Help:    "Milliseconds taken to process an event",
			Buckets: []float64{100, 200, 500, 1000, 3000, 5000, 10000},
		}, []string{metricQueue, metricRoutingKey, metricResult},
	)

	eventPublishSucceedCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "amqp_events_publish_succeed",
			Help: "Count of AMQP events that could be published successfully",
		}, []string{metricExchange, metricRoutingKey},
	)

	eventPublishFailedCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "amqp_events_publish_failed",
			Help: "Count of AMQP events that could not be published",
		}, []string{metricExchange, metricRoutingKey},
	)

	eventPublishDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "amqp_events_publish_duration",
			Help:    "Milliseconds taken to publish an event",
			Buckets: []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000},
		}, []string{metricExchange, metricRoutingKey, metricResult},
	)
)

func eventReceived(queue string, routingKey string) {
	eventReceivedCounter.WithLabelValues(queue, routingKeyLabel(routingKey)).Inc()
}

func eventWithoutHandler(queue string, routingKey string) {
	eventWithoutHandlerCounter.WithLabelValues(queue, routingKeyLabel(routingKey)).Inc()
}

func eventNotParsable(queue string, routingKey string) {
	eventNotParsableCounter.WithLabelValues(queue, routingKeyLabel(routingKey)).Inc()
}

func eventNack(queue string, routingKey string, milliseconds int64) {
	eventNackCounter.WithLabelValues(queue, routingKeyLabel(routingKey)).Inc()

	eventProcessedDuration.
		WithLabelValues(queue, routingKeyLabel(routingKey), "NACK").
		Observe(float64(milliseconds))
}

func eventAck(queue string, routingKey string, milliseconds int64) {
	eventAckCounter.WithLabelValues(queue, routingKeyLabel(routingKey)).Inc()

	eventProcessedDuration.
		WithLabelValues(queue, routingKeyLabel(routingKey), "ACK").
		Observe(float64(milliseconds))
}

func eventPublishSucceed(exchange string, routingKey string, milliseconds int64) {
	eventPublishSucceedCounter.WithLabelValues(exchange, routingKeyLabel(routingKey)).Inc()
	eventPublishDuration.
		WithLabelValues(exchange, routingKeyLabel(routingKey), "OK").
		Observe(float64(milliseconds))
}

func eventPublishFailed(exchange string, routingKey string, milliseconds int64) {
	eventPublishFailedCounter.WithLabelValues(exchange, routingKeyLabel(routingKey)).Inc()
	eventPublishDuration.
		WithLabelValues(exchange, routingKeyLabel(routingKey), "ERROR").
		Observe(float64(milliseconds))
}

// InitMetrics registers all AMQP Prometheus metrics with the given registerer.
// Call this once during application startup before Start(), e.g.
// InitMetrics(prometheus.DefaultRegisterer). The mapper is applied globally
// and subsequent calls overwrite the previous mapper.
func InitMetrics(registerer prometheus.Registerer, opts ...MetricsOption) error {
	cfg := &metricsConfig{routingKeyMapper: func(s string) string { return s }}
	for _, o := range opts {
		o(cfg)
	}
	routingKeyMapperVal.Store(cfg.routingKeyMapper)

	collectors := []prometheus.Collector{
		eventReceivedCounter,
		eventAckCounter,
		eventNackCounter,
		eventWithoutHandlerCounter,
		eventNotParsableCounter,
		eventProcessedDuration,
		eventPublishSucceedCounter,
		eventPublishFailedCounter,
		eventPublishDuration,
	}
	for _, collector := range collectors {
		mv, ok := collector.(metricResetter)
		if ok {
			mv.Reset()
		}
		err := registerer.Register(collector)
		if err != nil && !errors.As(err, &prometheus.AlreadyRegisteredError{}) {
			return err
		}
	}
	return nil
}

// CounterVec, MetricVec and HistogramVec have a Reset func
// in order not to cast to each specific type, metricResetter can
// be used to just get access to the Reset func
type metricResetter interface {
	Reset()
}
