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
	"errors"
	"fmt"
	"math"
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/sparetimecoders/gomessaging/spec"
	"github.com/stretchr/testify/require"
)

var (
	testUri = must(amqp.ParseURI(amqpURL))
)

func Test_ValidURI(t *testing.T) {
	conn, err := NewFromURL("svc", "amqp://user:password@localhost:67333/a")
	require.NoError(t, err)
	require.NotNil(t, conn)
	require.Equal(t, "localhost", conn.URI().Host)
	require.Equal(t, 67333, conn.URI().Port)
	require.Equal(t, "a", conn.URI().Vhost)
	require.Equal(t, "user", conn.URI().Username)
	require.Equal(t, "password", conn.URI().Password)
}

func Test_EmptyServiceName(t *testing.T) {
	_, err := NewFromURL("", "amqp://user:password@localhost:5672")
	require.EqualError(t, err, "service name must not be empty")
}

func Test_InvalidURI(t *testing.T) {
	_, err := NewFromURL("svc", "")
	require.EqualError(t, err, "AMQP scheme must be either 'amqp://' or 'amqps://'")
}

func Test_PublishServiceResponse(t *testing.T) {
	c := mockConnection(NewMockAmqpChannel())
	err := c.PublishServiceResponse(context.Background(), "target", "key", struct {
	}{})

	require.NoError(t, err)
}

func Test_AmqpVersion(t *testing.T) {
	require.Equal(t, "_unknown_", version())
}

func Test_Start_MultipleCallsFails(t *testing.T) {
	conn := &Connection{
		started: true,
	}
	err := conn.Start(context.Background())
	require.EqualError(t, err, "already started")
}

func Test_Start_SetupFails(t *testing.T) {
	mockChannel := &MockAmqpChannel{
		consumeFn: func(queue, consumer string, autoAck, exclusive, noLocal, noWait bool, args amqp.Table) (<-chan amqp.Delivery, error) {
			return nil, errors.New("error consuming queue")
		},
	}

	mockAmqpConnection := &MockAmqpConnection{
		channelFn: func() (amqpChannel, error) {
			return mockChannel, nil
		},
	}
	conn := newConnection("test", testUri)
	conn.connection = mockAmqpConnection

	err := conn.Start(context.Background(),
		EventStreamConsumer("test", func(ctx context.Context, msg spec.ConsumableEvent[Message]) error {
			return errors.New("failed")
		}))
	require.Error(t, err)
	require.EqualError(t, err, "failed to create consumer for queue events.topic.exchange.queue.test: error consuming queue")
}

func Test_Start_ConnectionFail(t *testing.T) {
	conn, err := NewFromURL("svc", "amqp://user:password@localhost:67333/a")
	require.NoError(t, err)
	conn.dialFn = func(url string, cfg amqp.Config) (*amqp.Connection, error) {
		return nil, errors.New("failed to connect")
	}
	err = conn.Start(context.Background())
	require.Error(t, err)
	require.EqualError(t, err, "failed to connect")
}

func Test_Start_FailToGetResponseChannel(t *testing.T) {
	mockAmqpConnection := &MockAmqpConnection{
		channelFn: func() (amqpChannel, error) {
			return nil, fmt.Errorf("failure to create channel")
		},
	}
	conn := Connection{
		connection: mockAmqpConnection,
	}
	err := conn.Start(context.Background())
	require.EqualError(t, err, "failed to create response channel: failure to create channel")
}

func Test_Start_FailToGetSetupChannel(t *testing.T) {
	count := 0
	mockAmqpConnection := &MockAmqpConnection{
		channelFn: func() (amqpChannel, error) {
			count++
			switch count {
			case 1: // response channel
				return &MockAmqpChannel{}, nil
			default: // setup channel
				return nil, fmt.Errorf("failure to create channel")
			}
		},
	}
	conn := Connection{
		connection: mockAmqpConnection,
	}
	err := conn.Start(context.Background())
	require.EqualError(t, err, "failed to create setup channel: failure to create channel")
}

func Test_Start_FailToCloseSetupChannel(t *testing.T) {
	mockAmqpConnection := &MockAmqpConnection{
		channelFn: func() (amqpChannel, error) {
			return &MockAmqpChannel{
				closeFn: func() error {
					return fmt.Errorf("failure to close channel")
				},
			}, nil
		},
	}
	conn := Connection{
		connection: mockAmqpConnection,
	}
	err := conn.Start(context.Background())
	require.EqualError(t, err, "failed to close setup channel: failure to close channel")
}

func Test_messageHandlerBindQueueToExchange(t *testing.T) {
	tests := []struct {
		name                     string
		queueDeclarationError    error
		exchangeDeclarationError error
		queueBindError           error
	}{
		{
			name: "ok",
		},
		{
			name:                  "queue declare error",
			queueDeclarationError: errors.New("failed to create queue"),
		},
		{
			name:                     "exchange declare error",
			exchangeDeclarationError: errors.New("failed to create exchange"),
		},
		{
			name:           "queue bind error",
			queueBindError: errors.New("failed to bind queue"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			channel := &MockAmqpChannel{
				QueueDeclarationError:    &tt.queueDeclarationError,
				ExchangeDeclarationError: &tt.exchangeDeclarationError,
				QueueBindError:           &tt.queueBindError,
			}
			conn := mockConnection(channel)
			cfg := &consumerConfig{
				routingKey:          "routingkey",
				handler:             nil,
				queueName:           "queue",
				exchangeName:        "exchange",
				kind:                amqp.ExchangeDirect,
				queueBindingHeaders: nil,
				queueHeaders:        nil,
			}
			err := conn.messageHandlerBindQueueToExchange(cfg)
			if tt.queueDeclarationError != nil {
				require.ErrorIs(t, err, tt.queueDeclarationError)
			} else if tt.exchangeDeclarationError != nil {
				require.ErrorIs(t, err, tt.exchangeDeclarationError)
			} else if tt.queueBindError != nil {
				require.ErrorIs(t, err, tt.queueBindError)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func Test_CloseCallsUnderlyingCloseMethod(t *testing.T) {
	channel := NewMockAmqpChannel()
	conn := mockConnection(channel)
	conn.started = true
	err := conn.Close()
	require.NoError(t, err)
	require.Equal(t, true, conn.connection.(*MockAmqpConnection).CloseCalled)
}

func Test_CloseWhenNotStarted(t *testing.T) {
	channel := NewMockAmqpChannel()
	conn := mockConnection(channel)
	conn.started = false
	err := conn.Close()
	require.NoError(t, err)
	require.Equal(t, false, conn.connection.(*MockAmqpConnection).CloseCalled)
}

func Test_ConnectToAmqpUrl_Ok(t *testing.T) {
	amqpConnection := &amqp.Connection{}
	conn, err := NewFromURL("svc", "amqp://user:password@localhost:12345/vhost")
	require.NoError(t, err)
	conn.dialFn = func(url string, cfg amqp.Config) (*amqp.Connection, error) {
		return amqpConnection, nil
	}
	err = conn.connectToAmqpURL()
	require.NoError(t, err)
	require.Equal(t, amqpConnection, conn.connection.(*amqpConn).Connection)
}

func Test_ConnectToAmqpUrl_ConnectionFailed(t *testing.T) {
	conn := Connection{
		dialFn: func(url string, cfg amqp.Config) (*amqp.Connection, error) {
			return nil, errors.New("failure to connect")
		},
	}
	err := conn.connectToAmqpURL()
	require.Error(t, err)
	require.Nil(t, conn.connection)
}

func Test_FailingSetupFunc(t *testing.T) {
	channel := NewMockAmqpChannel()
	conn := mockConnection(channel)
	err := conn.Start(context.Background(), func(c *Connection) error { return nil }, func(c *Connection) error { return fmt.Errorf("error message") })
	require.EqualError(t, err, "setup function <github.com/sparetimecoders/gomessaging/amqp.Test_FailingSetupFunc.func2> failed: error message")
}

func Test_AmqpConfig(t *testing.T) {
	require.Equal(t, fmt.Sprintf("servicename#_unknown_#@%s", hostName()), amqpConfig("servicename").Properties["connection_name"])
}

func Test_QueueDeclare(t *testing.T) {
	channel := NewMockAmqpChannel()
	err := queueDeclare(channel, &consumerConfig{
		queueName:    "test",
		exchangeName: "test",
		queueHeaders: defaultQueueOptions})
	require.NoError(t, err)
	require.Equal(t, 1, len(channel.QueueDeclarations))
	require.Equal(t, QueueDeclaration{name: "test", durable: true, autoDelete: false, exclusive: false, noWait: false, args: defaultQueueOptions}, channel.QueueDeclarations[0])
}

func Test_TransientQueueDeclare(t *testing.T) {
	channel := NewMockAmqpChannel()
	err := queueDeclare(channel, &consumerConfig{
		queueName:    "test",
		exchangeName: "test",
		queueHeaders: defaultQueueOptions})
	require.NoError(t, err)

	require.Equal(t, 1, len(channel.QueueDeclarations))
	require.Equal(t, QueueDeclaration{name: "test", durable: true, autoDelete: false, exclusive: false, noWait: false, args: defaultQueueOptions}, channel.QueueDeclarations[0])
}

func Test_ExchangeDeclare(t *testing.T) {
	channel := NewMockAmqpChannel()

	err := exchangeDeclare(channel, "name", "topic")
	require.NoError(t, err)
	require.Equal(t, 1, len(channel.ExchangeDeclarations))
	require.Equal(t, ExchangeDeclaration{name: "name", kind: "topic", durable: true, autoDelete: false, noWait: false, args: nil}, channel.ExchangeDeclarations[0])
}

func Test_Publish_Fail(t *testing.T) {
	channel := NewMockAmqpChannel()
	channel.publishFn = func(ctx context.Context, exchange, key string, mandatory, immediate bool, msg amqp.Publishing) error {
		return errors.New("publish failed")
	}
	err := publishMessage(context.Background(), nil, nil, nil, channel, Message{true}, "key", "exchange", "test-service", nil)
	require.EqualError(t, err, "publish failed")
}

func Test_Publish(t *testing.T) {
	channel := NewMockAmqpChannel()
	headers := amqp.Table{}
	headers["key"] = "value"
	err := publishMessage(context.Background(), nil, nil, nil, channel, Message{true}, "key", "exchange", "test-service", headers)
	require.NoError(t, err)

	publish := <-channel.Published
	require.Equal(t, "key", publish.key)
	require.Equal(t, "exchange", publish.exchange)
	require.Equal(t, false, publish.immediate)
	require.Equal(t, false, publish.mandatory)

	msg := publish.msg
	require.Equal(t, "", msg.Type)
	require.Equal(t, "application/json", msg.ContentType)
	require.Equal(t, "", msg.AppId)
	require.Equal(t, "", msg.ContentEncoding)
	require.Equal(t, "", msg.CorrelationId)
	require.Equal(t, uint8(2), msg.DeliveryMode)
	require.Equal(t, "", msg.Expiration)
	require.Equal(t, "value", msg.Headers["key"])
	require.Equal(t, "", msg.ReplyTo)

	// CloudEvents binary content mode headers (AMQP uses cloudEvents: prefix)
	require.Equal(t, "1.0", msg.Headers[spec.AMQPCEHeaderKey(spec.CEAttrSpecVersion)])
	require.Equal(t, "key", msg.Headers[spec.AMQPCEHeaderKey(spec.CEAttrType)])
	require.Equal(t, "test-service", msg.Headers[spec.AMQPCEHeaderKey(spec.CEAttrSource)])
	require.NotEmpty(t, msg.Headers[spec.AMQPCEHeaderKey(spec.CEAttrID)])
	require.NotEmpty(t, msg.Headers[spec.AMQPCEHeaderKey(spec.CEAttrTime)])

	body := &Message{}
	_ = json.Unmarshal(msg.Body, &body)
	require.Equal(t, &Message{true}, body)
	require.Equal(t, "", msg.UserId)
	require.Equal(t, uint8(0), msg.Priority)
	require.Equal(t, "", msg.MessageId)
}

func Test_Publish_Marshal_Error(t *testing.T) {
	channel := NewMockAmqpChannel()
	headers := amqp.Table{}
	headers["key"] = "value"
	err := publishMessage(context.Background(), nil, nil, nil, channel, math.Inf(1), "key", "exchange", "test-service", headers)
	require.EqualError(t, err, "json: unsupported value: +Inf")
}

func TestResponseWrapper(t *testing.T) {
	tests := []struct {
		name         string
		handlerResp  any
		handlerErr   error
		published    any
		publisherErr error
		wantErr      error
		wantResp     any
		headers      *spec.Headers
	}{
		{
			name: "handler ok - no resp - nothing published",
		},
		{
			name:        "handler ok - with resp - published",
			handlerResp: Message{},
			published:   Message{},
			wantResp:    Message{},
		},
		{
			name:         "handler ok - with resp - publish error",
			handlerResp:  Message{},
			publisherErr: errors.New("amqp error"),
			wantErr:      errors.New("failed to publish response, amqp error"),
		},
		{
			name:       "handler error - no resp - nothing published",
			handlerErr: errors.New("failed"),
			wantErr:    errors.New("failed to process message, failed"),
		},
		{
			name:        "handler error - with resp - nothing published",
			handlerResp: Message{},
			handlerErr:  errors.New("failed"),
			wantErr:     errors.New("failed to process message, failed"),
		},
		{
			name:        "handler ok - with resp - missing header",
			handlerResp: Message{},
			headers:     &spec.Headers{},
			wantErr:     errors.New(`failed to extract service name, no "service" header in message from exchange  (routing key: )`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &mockPublisher[any]{
				err:       tt.publisherErr,
				published: nil,
			}
			headers := spec.Headers(map[string]any{headerService: "test"})

			if tt.headers != nil {
				headers = *tt.headers
			}
			err := responseWrapper(func(ctx context.Context, event spec.ConsumableEvent[Message]) (any, error) {
				return tt.handlerResp, tt.handlerErr
			}, "key", p.publish)(context.TODO(), spec.ConsumableEvent[Message]{
				DeliveryInfo: spec.DeliveryInfo{Headers: headers},
			})
			p.checkPublished(t, tt.published)

			// require.Equal(t, tt.wantResp, resp)
			if tt.wantErr != nil {
				require.EqualError(t, tt.wantErr, err.Error())
			}
		})
	}
}

func Test_Publisher_ReservedHeader(t *testing.T) {
	p := NewPublisher()
	err := p.Publish(context.Background(), "key", TestMessage{Msg: "test"}, Header{"service", "header"})
	require.EqualError(t, err, "reserved key service used, please change to use another one")
}

type Message struct {
	Ok bool
}

type mockPublisher[R any] struct {
	err       error
	published R
}

func (m *mockPublisher[R]) publish(ctx context.Context, targetService, routingKey string, msg R) error {
	if m.err != nil {
		return m.err
	}
	m.published = msg
	return nil
}

func (m *mockPublisher[R]) checkPublished(t *testing.T, i R) {
	require.EqualValues(t, m.published, i)
}

func Test_Must_Panic(t *testing.T) {
	require.Panics(t, func() {
		Must[int](nil, errors.New("boom"))
	})
}

func Test_Must_Ok(t *testing.T) {
	v := 42
	result := Must(&v, nil)
	require.Equal(t, &v, result)
}

func Test_must_Panic(t *testing.T) {
	require.Panics(t, func() {
		must(0, errors.New("boom"))
	})
}

func Test_Start_ConsumerChannelFail(t *testing.T) {
	count := 0
	setupChannel := &MockAmqpChannel{}
	mockAmqpConnection := &MockAmqpConnection{
		channelFn: func() (amqpChannel, error) {
			count++
			switch count {
			case 1: // response channel
				return &MockAmqpChannel{}, nil
			case 2: // setup channel
				return setupChannel, nil
			default: // consumer channel
				return nil, fmt.Errorf("consumer channel failure")
			}
		},
	}
	conn := newConnection("test", testUri)
	conn.connection = mockAmqpConnection

	err := conn.Start(context.Background(),
		EventStreamConsumer("test", func(ctx context.Context, msg spec.ConsumableEvent[Message]) error {
			return nil
		}))
	require.Error(t, err)
	require.ErrorContains(t, err, "consumer channel failure")
}

func Test_Topology(t *testing.T) {
	c := mockConnection(NewMockAmqpChannel())
	publisher := NewPublisher()

	err := c.Start(context.Background(),
		EventStreamPublisher(publisher),
		EventStreamConsumer("Order.Created", func(ctx context.Context, msg spec.ConsumableEvent[Message]) error {
			return nil
		}),
		ServicePublisher("email-service", NewPublisher()),
		ServiceResponseConsumer[Message]("email-service", "email.send", func(ctx context.Context, msg spec.ConsumableEvent[Message]) error {
			return nil
		}),
	)
	require.NoError(t, err)

	topo := c.Topology()
	require.Equal(t, "svc", topo.ServiceName)
	require.Len(t, topo.Endpoints, 4)

	// EventStreamPublisher
	require.Equal(t, "events.topic.exchange", topo.Endpoints[0].ExchangeName)
	require.Equal(t, spec.DirectionPublish, topo.Endpoints[0].Direction)
	require.Equal(t, spec.PatternEventStream, topo.Endpoints[0].Pattern)

	// EventStreamConsumer
	require.Equal(t, "events.topic.exchange", topo.Endpoints[1].ExchangeName)
	require.Equal(t, spec.DirectionConsume, topo.Endpoints[1].Direction)
	require.Equal(t, "Order.Created", topo.Endpoints[1].RoutingKey)
	require.Equal(t, "events.topic.exchange.queue.svc", topo.Endpoints[1].QueueName)

	// ServicePublisher
	require.Equal(t, "email-service.direct.exchange.request", topo.Endpoints[2].ExchangeName)
	require.Equal(t, spec.DirectionPublish, topo.Endpoints[2].Direction)
	require.Equal(t, spec.PatternServiceRequest, topo.Endpoints[2].Pattern)

	// ServiceResponseConsumer
	require.Equal(t, "email-service.headers.exchange.response", topo.Endpoints[3].ExchangeName)
	require.Equal(t, spec.DirectionConsume, topo.Endpoints[3].Direction)
	require.Equal(t, spec.PatternServiceResponse, topo.Endpoints[3].Pattern)
	require.Equal(t, "email.send", topo.Endpoints[3].RoutingKey)
}
