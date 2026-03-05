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
	"log/slog"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/sparetimecoders/messaging/specification/spec"
)

// CollectTopology runs the given setup functions against a no-op channel to
// collect the topology that would be declared, without connecting to a broker.
// This is useful for generating topology JSON for visualization or validation.
func CollectTopology(serviceName string, setups ...Setup) (spec.Topology, error) {
	noop := &noopChannel{}
	c := &Connection{
		serviceName: serviceName,
		setupChannel: noop,
		connection:   &noopConnection{},
		queueConsumers: &queueConsumers{
			consumers:  make(map[string]*queueConsumer),
			spanNameFn: spanNameFn,
		},
		logger:   slog.New(slog.NewTextHandler(discardWriter{}, nil)),
		topology: spec.Topology{Transport: spec.TransportAMQP, ServiceName: serviceName},
	}

	for _, f := range setups {
		if err := f(c); err != nil {
			return spec.Topology{}, err
		}
	}

	return c.topology, nil
}

// noopConnection implements amqpConnection with all operations succeeding as no-ops.
type noopConnection struct{}

func (noopConnection) Close() error                        { return nil }
func (noopConnection) channel() (amqpChannel, error)       { return &noopChannel{}, nil }

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }

// noopChannel implements amqpChannel with all operations succeeding as no-ops.
type noopChannel struct{}

func (noopChannel) Close() error { return nil }

func (noopChannel) QueueBind(_, _, _ string, _ bool, _ amqp.Table) error { return nil }

func (noopChannel) Consume(_, _ string, _, _, _, _ bool, _ amqp.Table) (<-chan amqp.Delivery, error) {
	return make(chan amqp.Delivery), nil
}

func (noopChannel) ExchangeDeclare(_, _ string, _, _, _, _ bool, _ amqp.Table) error { return nil }

func (noopChannel) PublishWithContext(_ context.Context, _, _ string, _, _ bool, _ amqp.Publishing) error {
	return nil
}

func (noopChannel) QueueDeclare(name string, _, _, _, _ bool, _ amqp.Table) (amqp.Queue, error) {
	return amqp.Queue{Name: name}, nil
}

func (noopChannel) NotifyPublish(confirm chan amqp.Confirmation) chan amqp.Confirmation {
	return confirm
}

func (noopChannel) NotifyClose(c chan *amqp.Error) chan *amqp.Error { return c }

func (noopChannel) Confirm(_ bool) error { return nil }

func (noopChannel) Qos(_, _ int, _ bool) error { return nil }
