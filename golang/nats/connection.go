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
	"fmt"
	"log/slog"
	"reflect"
	"runtime"

	natsgo "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/sparetimecoders/gomessaging/spec"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// Connection wraps a NATS connection with JetStream support.
type Connection struct {
	started     bool
	serviceName string
	natsURL     string
	nc          *natsgo.Conn
	js          jetstream.JetStream

	consumers     []*jsConsumerHandle
	subscriptions []*natsgo.Subscription

	logger            *slog.Logger
	topology          spec.Topology
	tracerProvider    trace.TracerProvider
	propagator        propagation.TextMapPropagator
	notificationCh    chan<- spec.Notification
	errorCh           chan<- spec.ErrorNotification
	spanNameFn        func(spec.DeliveryInfo) string
	publishSpanNameFn func(stream, routingKey string) string

	connectFn func(url string, opts ...natsgo.Option) (*natsgo.Conn, error)
	natsOpts  []natsgo.Option

	// pendingJSConsumers collects JetStream consumer configs during setup.
	// They are grouped and started after all setup functions have run.
	pendingJSConsumers []*consumerConfig

	streamDefaults StreamConfig            // connection-level defaults
	streamConfigs  map[string]StreamConfig // per-stream overrides

	// collectMode disables actual NATS operations for topology collection.
	collectMode bool
}

// jsConsumerHandle tracks an active JetStream consumer context for cleanup.
type jsConsumerHandle struct {
	ctx jetstream.ConsumeContext
}

var (
	// ErrAlreadyStarted is returned when Start is called on an already-started connection.
	ErrAlreadyStarted = fmt.Errorf("already started")
	// ErrEmptySuffix is returned when an empty suffix is passed to AddConsumerNameSuffix.
	ErrEmptySuffix = fmt.Errorf("empty consumer suffix not allowed")
	// ErrNoMessageTypeForRouteKey is returned when a TypeMapper has no type for the routing key.
	ErrNoMessageTypeForRouteKey = fmt.Errorf("no message type for routing key configured")
)

// NewConnection creates a new Connection for the given service and NATS URL.
func NewConnection(serviceName, natsURL string, opts ...natsgo.Option) (*Connection, error) {
	if serviceName == "" {
		return nil, fmt.Errorf("service name must not be empty")
	}
	if natsURL == "" {
		return nil, fmt.Errorf("NATS URL must not be empty")
	}
	return newConnection(serviceName, natsURL, opts...), nil
}

func newConnection(serviceName, natsURL string, opts ...natsgo.Option) *Connection {
	return &Connection{
		serviceName: serviceName,
		natsURL:     natsURL,
		natsOpts:    opts,
		logger:      slog.Default().With("service", serviceName),
		topology:    spec.Topology{Transport: spec.TransportNATS, ServiceName: serviceName},
		connectFn:   natsgo.Connect,
	}
}

// Topology returns the messaging topology declared by this connection's setup.
func (c *Connection) Topology() spec.Topology {
	return c.topology
}

func (c *Connection) addEndpoint(ep spec.Endpoint) {
	c.topology.Endpoints = append(c.topology.Endpoints, ep)
}

func (c *Connection) tracer() trace.Tracer {
	return tracerFromProvider(c.tracerProvider)
}

func (c *Connection) log() *slog.Logger {
	if c.logger != nil {
		return c.logger
	}
	return slog.Default()
}

var defaultSpanNameFn = func(info spec.DeliveryInfo) string {
	return fmt.Sprintf("%s#%s", info.Destination, info.Key)
}

// Start connects to NATS, applies setup functions, and starts all consumers.
func (c *Connection) Start(ctx context.Context, opts ...Setup) error {
	if c.started {
		return ErrAlreadyStarted
	}

	if c.spanNameFn == nil {
		c.spanNameFn = defaultSpanNameFn
	}

	if c.collectMode {
		for _, f := range opts {
			if err := f(c); err != nil {
				return fmt.Errorf("setup function <%s> failed: %w", getSetupFuncName(f), err)
			}
		}
		c.started = true
		return nil
	}

	// Connect to NATS first so that setup functions have access to nc/js.
	connect := c.connectFn
	if connect == nil {
		connect = natsgo.Connect
	}
	nc, err := connect(c.natsURL, c.natsOpts...)
	if err != nil {
		return fmt.Errorf("failed to connect to NATS: %w", err)
	}
	c.nc = nc

	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return fmt.Errorf("failed to create JetStream context: %w", err)
	}
	c.js = js

	c.log().Info("connected to NATS", "url", c.natsURL)

	// Run setup functions (they collect consumer configs and start publishers/subscriptions).
	for _, f := range opts {
		if err := f(c); err != nil {
			nc.Close()
			return fmt.Errorf("setup function <%s> failed: %w", getSetupFuncName(f), err)
		}
	}

	// Start grouped JetStream consumers after all setups have run.
	if err := c.startPendingJSConsumers(ctx); err != nil {
		nc.Close()
		return err
	}

	c.started = true
	c.log().Info("connection started",
		"consumers", len(c.consumers),
		"subscriptions", len(c.subscriptions),
	)
	return nil
}

// Close drains the NATS connection, stopping all consumers and subscriptions.
func (c *Connection) Close() error {
	if !c.started || c.nc == nil {
		return nil
	}

	for _, ch := range c.consumers {
		if ch.ctx != nil {
			ch.ctx.Stop()
		}
	}

	return c.nc.Drain()
}

// ensureStream creates or updates a JetStream stream with retention limits.
func (c *Connection) ensureStream(ctx context.Context, name string) (jetstream.Stream, error) {
	cfg := jetstream.StreamConfig{
		Name:     name,
		Subjects: []string{streamSubjects(name)},
		Storage:  jetstream.FileStorage,
	}

	// Apply connection-level defaults.
	sc := c.streamDefaults
	// Per-stream overrides replace defaults entirely (not merged field-by-field).
	if override, ok := c.streamConfigs[name]; ok {
		sc = override
	}

	cfg.MaxAge = sc.MaxAge
	cfg.MaxBytes = sc.MaxBytes
	cfg.MaxMsgs = sc.MaxMsgs

	if sc.MaxAge == 0 && sc.MaxBytes == 0 && sc.MaxMsgs == 0 {
		c.log().Warn("stream has no retention limits configured, storage may grow unbounded",
			"stream", name,
		)
	}

	return c.js.CreateOrUpdateStream(ctx, cfg)
}

// getSetupFuncName returns the name of the Setup function.
func getSetupFuncName(f Setup) string {
	return runtime.FuncForPC(reflect.ValueOf(f).Pointer()).Name()
}
