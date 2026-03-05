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
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime/debug"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/sparetimecoders/messaging/specification/spec"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// Connection is a wrapper around the actual amqp.Connection and amqp.Channel
type Connection struct {
	started        bool
	serviceName    string
	amqpUri        amqp.URI
	connection     amqpConnection
	setupChannel    amqpChannel
	responseChannel amqpChannel
	queueConsumers *queueConsumers
	notificationCh chan<- spec.Notification
	errorCh        chan<- spec.ErrorNotification
	spanNameFn        func(spec.DeliveryInfo) string
	publishSpanNameFn func(exchange, routingKey string) string
	closeListener     chan error
	prefetchLimit  int
	logger         *slog.Logger
	topology       spec.Topology
	tracerProvider trace.TracerProvider
	propagator     propagation.TextMapPropagator
	legacySupport  bool
	dialFn         func(url string, cfg amqp.Config) (*amqp.Connection, error)
}

// ServiceResponsePublisher is the callback signature used to publish a response
// back to the originating service in the request-response pattern.
type ServiceResponsePublisher[T any] func(ctx context.Context, targetService, routingKey string, event T) error

var (
	// ErrEmptySuffix is returned when an empty suffix is passed to AddQueueNameSuffix.
	ErrEmptySuffix = fmt.Errorf("empty queue suffix not allowed")
	// ErrAlreadyStarted is returned when Start is called on an already-started connection.
	ErrAlreadyStarted = fmt.Errorf("already started")
)

// NewFromURL creates a new Connection by parsing an AMQP URL.
// The serviceName is used for exchange/queue naming and tracing.
func NewFromURL(serviceName string, amqpURL string) (*Connection, error) {
	if serviceName == "" {
		return nil, fmt.Errorf("service name must not be empty")
	}
	uri, err := amqp.ParseURI(amqpURL)
	if err != nil {
		return nil, err
	}
	return newConnection(serviceName, uri), nil
}

// PublishServiceResponse sends a message to targetService as a handler response
func (c *Connection) PublishServiceResponse(ctx context.Context, targetService, routingKey string, msg any) error {
	return publishMessage(ctx, c.tracer(), c.propagator, nil, c.responseChannel, msg, routingKey, serviceResponseExchangeName(c.serviceName), c.serviceName, amqp.Table{headerService: targetService})
}

// URI returns the parsed AMQP URI used by this connection.
func (c *Connection) URI() amqp.URI {
	return c.amqpUri
}

// Topology returns the messaging topology declared by this connection's setup.
// It is populated during Start() as setup functions execute.
func (c *Connection) Topology() spec.Topology {
	return c.topology
}

func (c *Connection) addEndpoint(ep spec.Endpoint) {
	c.topology.Endpoints = append(c.topology.Endpoints, ep)
}

func (c *Connection) tracer() trace.Tracer {
	return tracerFromProvider(c.tracerProvider)
}

// Start connects to AMQP, applies each Setup function to declare exchanges, queues,
// and bindings, then starts all consumers. It returns ErrAlreadyStarted if called twice.
func (c *Connection) Start(ctx context.Context, opts ...Setup) (err error) {
	if c.started {
		return ErrAlreadyStarted
	}
	if c.logger == nil {
		c.logger = slog.Default().With("service", c.serviceName)
	}
	if c.queueConsumers == nil {
		c.queueConsumers = &queueConsumers{
			consumers:  make(map[string]*queueConsumer),
			spanNameFn: spanNameFn,
		}
	} else if c.queueConsumers.consumers == nil {
		c.queueConsumers.consumers = make(map[string]*queueConsumer)
	}
	if c.connection == nil {
		if err := c.connectToAmqpURL(); err != nil {
			return err
		}
	}
	c.logger.Info("connected to amqp", "host", c.amqpUri.Host, "vhost", c.amqpUri.Vhost)

	if c.responseChannel, err = c.connection.channel(); err != nil {
		return fmt.Errorf("failed to create response channel: %w", err)
	}

	if c.setupChannel, err = c.connection.channel(); err != nil {
		return fmt.Errorf("failed to create setup channel: %w", err)
	}

	for _, f := range opts {
		if err := f(c); err != nil {
			return fmt.Errorf("setup function <%s> failed: %w", getSetupFuncName(f), err)
		}
	}

	if err = c.setupChannel.Close(); err != nil {
		return fmt.Errorf("failed to close setup channel: %w", err)
	}

	// Register connection-level close notification (after setup options have run).
	if c.closeListener != nil {
		if ac, ok := c.connection.(*amqpConn); ok {
			connCloseCh := make(chan *amqp.Error, 1)
			ac.Connection.NotifyClose(connCloseCh)
			go func() {
				for ev := range connCloseCh {
					if ev != nil {
						c.closeListener <- fmt.Errorf("connection closed: %s", ev.Error())
					}
				}
			}()
		}
	}

	if err := c.startConsumers(); err != nil {
		return err
	}

	c.started = true
	c.logger.Info("connection started", "consumers", len(c.queueConsumers.consumers))
	return nil
}

// Close closes the amqp connection, see amqp.Connection.Close
func (c *Connection) Close() error {
	if !c.started {
		return nil
	}
	return c.connection.Close()
}

type amqpConnection interface {
	io.Closer
	channel() (amqpChannel, error)
}
type amqpConn struct {
	conn *Connection
	*amqp.Connection
}

func (c *amqpConn) channel() (amqpChannel, error) {
	ch, err := c.Connection.Channel()
	if err != nil {
		return nil, err
	}
	if err := ch.Qos(c.conn.prefetchLimit, 0, false); err != nil {
		return nil, fmt.Errorf("error setting qos: %s", err)
	}

	errChannel := make(chan *amqp.Error)
	ch.NotifyClose(errChannel)
	if c.conn.closeListener != nil {
		go func() {
			for ev := range errChannel {
				if ev != nil {
					c.conn.closeListener <- errors.New(ev.Error())
				}
			}
		}()
	}
	return ch, err
}

func dialConfig(url string, cfg amqp.Config) (*amqp.Connection, error) {
	return amqp.DialConfig(url, cfg)
}


func version() string {
	// NOTE: this doesn't work outside of a build, se we can't really test it
	if x, ok := debug.ReadBuildInfo(); ok {
		for _, y := range x.Deps {
			if y.Path == "github.com/sparetimecoders/messaging/golang/amqp" {
				return y.Version
			}
		}
	}
	return "_unknown_"
}

func amqpConfig(serviceName string) amqp.Config {
	config := amqp.Config{
		Properties: amqp.NewConnectionProperties(),
		Heartbeat:  10 * time.Second,
		Locale:     "en_US",
	}
	config.Properties.SetClientConnectionName(fmt.Sprintf("%s#%+v#@%s", serviceName, version(), hostName()))
	return config
}

func hostName() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "_unknown_"
	}
	return hostname
}

func (c *Connection) connectToAmqpURL() (err error) {
	cfg := amqpConfig(c.serviceName)

	dial := c.dialFn
	if dial == nil {
		dial = dialConfig
	}
	conn, err := dial(c.amqpUri.String(), cfg)
	if err != nil {
		return err
	}
	c.connection = &amqpConn{
		Connection: conn,
		conn:       c,
	}

	return nil
}

func (c *Connection) messageHandlerBindQueueToExchange(cfg *consumerConfig) error {
	if err := c.queueConsumers.add(cfg.queueName, cfg.routingKey, cfg.handler); err != nil {
		return err
	}

	if err := exchangeDeclare(c.setupChannel, cfg.exchangeName, cfg.kind); err != nil {
		return err
	}
	if err := queueDeclare(c.setupChannel, cfg); err != nil {
		return err
	}
	c.logger.Info("bound queue to exchange",
		"queue", cfg.queueName,
		"exchange", cfg.exchangeName,
		"routingKey", cfg.routingKey,
		"kind", cfg.kind,
	)
	return c.setupChannel.QueueBind(cfg.queueName, cfg.routingKey, cfg.exchangeName, false, cfg.queueBindingHeaders)
}

func exchangeDeclare(channel amqpChannel, name string, kind string) error {
	return channel.ExchangeDeclare(name, kind, true, false, false, false, nil)
}

func queueDeclare(channel amqpChannel, cfg *consumerConfig) error {
	_, err := channel.QueueDeclare(cfg.queueName, true, false, false, false, cfg.queueHeaders)
	return err
}

const (
	headerService = "service"
)

const contentType = "application/json"

var (
	deleteQueueAfter    = 5 * 24 * time.Hour
	defaultQueueOptions = amqp.Table{
		amqp.QueueTypeArg:            amqp.QueueTypeQuorum,
		amqp.SingleActiveConsumerArg: true,
		amqp.QueueTTLArg:             int(deleteQueueAfter.Seconds() * 1000)}
)

func newConnection(serviceName string, uri amqp.URI) *Connection {
	logger := slog.Default().With("service", serviceName)
	return &Connection{
		serviceName: serviceName,
		amqpUri:     uri,
		queueConsumers: &queueConsumers{
			consumers:  make(map[string]*queueConsumer),
			spanNameFn: spanNameFn,
		},
		prefetchLimit: 20,
		logger:        logger,
		topology:      spec.Topology{Transport: spec.TransportAMQP, ServiceName: serviceName},
		dialFn:        dialConfig,
	}
}

func (c *Connection) startConsumers() error {
	for _, consumer := range c.queueConsumers.consumers {
		consumer.logger = c.logger.With("queue", consumer.queue)
		consumer.tracer = c.tracer()
		consumer.propagator = c.propagator
		consumer.serviceName = c.serviceName
		consumer.legacySupport = c.legacySupport

		ch, err := c.connection.channel()
		if err != nil {
			return fmt.Errorf("failed to create consumer channel for queue %s: %w", consumer.queue, err)
		}

		if deliveries, err := consumer.consume(ch, c.notificationCh, c.errorCh); err != nil {
			return fmt.Errorf("failed to create consumer for queue %s: %w", consumer.queue, err)
		} else {
			c.logger.Info("started consumer", "queue", consumer.queue)
			go consumer.loop(deliveries)
		}
	}
	return nil
}
