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
	"maps"
	"slices"
	"sync"
	"testing"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/sparetimecoders/gomessaging/spec"
	"github.com/stretchr/testify/require"
)

func Test_Publisher_Setups(t *testing.T) {
	// Needed for transient stream tests
	uuid.SetRand(badRand{})

	tests := []struct {
		name              string
		opts              func(p *Publisher) []Setup
		messages          map[string]any
		expectedError     string
		expectedExchanges []ExchangeDeclaration
		expectedQueues    []QueueDeclaration
		expectedBindings  []BindingDeclaration
		expectedPublished []*Publish
		headers           []Header
	}{
		{
			name: "EventStreamConsumer",
			opts: func(p *Publisher) []Setup {
				return []Setup{QueuePublisher(p, "destQueue")}
			},
			messages: map[string]any{"key": TestMessage{"test", true}},
			headers:  []Header{{"x-header", "header"}},
			expectedPublished: []*Publish{{
				exchange:  "",
				key:       "key",
				mandatory: false,
				immediate: false,
				msg: amqp.Publishing{
					Headers:         amqp.Table{"CC": []interface{}{"destQueue"}, "service": "svc", "x-header": "header"},
					ContentType:     contentType,
					ContentEncoding: "",
					DeliveryMode:    2,
				},
			}},
		},
		{
			name: "EventStreamPublisher",
			opts: func(p *Publisher) []Setup {
				return []Setup{EventStreamPublisher(p)}
			},
			messages:          map[string]any{"key": TestMessage{"test", true}},
			headers:           []Header{{"x-header", "header"}},
			expectedExchanges: []ExchangeDeclaration{{name: topicExchangeName(defaultEventExchangeName), noWait: false, internal: false, autoDelete: false, durable: true, kind: amqp.ExchangeTopic, args: nil}},
			expectedPublished: []*Publish{{
				exchange:  topicExchangeName(defaultEventExchangeName),
				key:       "key",
				mandatory: false,
				immediate: false,
				msg: amqp.Publishing{
					Headers:         amqp.Table{"service": "svc", "x-header": "header"},
					ContentType:     contentType,
					ContentEncoding: "",
					DeliveryMode:    2,
				},
			}},
		},
		{
			name: "ServicePublisher",
			opts: func(p *Publisher) []Setup {
				return []Setup{ServicePublisher("svc", p)}
			},
			expectedExchanges: []ExchangeDeclaration{{name: serviceRequestExchangeName("svc"), noWait: false, internal: false, autoDelete: false, durable: true, kind: amqp.ExchangeDirect, args: nil}},
			messages:          map[string]any{"key": TestMessage{"test", true}},
			expectedPublished: []*Publish{{
				exchange:  serviceRequestExchangeName("svc"),
				key:       "key",
				mandatory: false,
				immediate: false,
				msg: amqp.Publishing{
					Headers:         amqp.Table{"service": "svc"},
					ContentType:     contentType,
					ContentEncoding: "",
					DeliveryMode:    2,
				},
			}},
		},
		{
			name: "ServicePublisher - multiple",
			opts: func(p *Publisher) []Setup {
				return []Setup{ServicePublisher("svc", p)}
			},
			expectedExchanges: []ExchangeDeclaration{{name: serviceRequestExchangeName("svc"), noWait: false, internal: false, autoDelete: false, durable: true, kind: amqp.ExchangeDirect, args: nil}},
			messages: map[string]any{
				"key1": TestMessage{"test", true},
				"key2": TestMessage2{"test", false},
			},
			expectedPublished: []*Publish{
				{
					exchange:  serviceRequestExchangeName("svc"),
					key:       "key1",
					mandatory: false,
					immediate: false,
					msg: amqp.Publishing{
						Headers:         amqp.Table{"service": "svc"},
						ContentType:     contentType,
						ContentEncoding: "",
						DeliveryMode:    2,
					},
				}, {
					exchange:  serviceRequestExchangeName("svc"),
					key:       "key2",
					mandatory: false,
					immediate: false,
					msg: amqp.Publishing{
						Headers:         amqp.Table{"service": "svc"},
						ContentType:     contentType,
						ContentEncoding: "",
						DeliveryMode:    2,
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			channel := NewMockAmqpChannel()
			conn := mockConnection(channel)
			p := NewPublisher()
			ctx := context.TODO()
			startErr := conn.Start(context.Background(), tt.opts(p)...)
			require.NoError(t, startErr)
			if tt.expectedExchanges != nil {
				require.Equal(t, tt.expectedExchanges, channel.ExchangeDeclarations)
			} else {
				require.Len(t, channel.ExchangeDeclarations, 0)
			}
			if tt.expectedQueues != nil {
				require.Equal(t, tt.expectedQueues, channel.QueueDeclarations)
			} else {
				require.Len(t, channel.QueueDeclarations, 0)
			}
			if tt.expectedBindings != nil {
				require.Equal(t, tt.expectedBindings, channel.BindingDeclarations)
			} else {
				require.Len(t, channel.BindingDeclarations, 0)
			}

			keys := slices.Collect(maps.Keys(tt.messages))
			slices.Sort(keys)
			for i := 0; i < len(keys); i++ {
				key := keys[i]
				msg := tt.messages[key]
				err := p.Publish(ctx, key, msg, tt.headers...)
				if tt.expectedError != "" {
					require.ErrorContains(t, err, tt.expectedError)
					continue
				} else {
					require.NoError(t, err)
				}
				if tt.expectedPublished[i] != nil {
					body, err := json.Marshal(msg)
					require.NoError(t, err)
					tt.expectedPublished[i].msg.Body = body
					actual := <-channel.Published
					// Verify CloudEvents headers are present (AMQP uses cloudEvents: prefix), then strip for comparison
					require.Equal(t, "1.0", actual.msg.Headers[spec.AMQPCEHeaderKey(spec.CEAttrSpecVersion)])
					require.Equal(t, key, actual.msg.Headers[spec.AMQPCEHeaderKey(spec.CEAttrType)])
					require.Equal(t, "svc", actual.msg.Headers[spec.AMQPCEHeaderKey(spec.CEAttrSource)])
					require.NotEmpty(t, actual.msg.Headers[spec.AMQPCEHeaderKey(spec.CEAttrID)])
					require.NotEmpty(t, actual.msg.Headers[spec.AMQPCEHeaderKey(spec.CEAttrTime)])
					delete(actual.msg.Headers, spec.AMQPCEHeaderKey(spec.CEAttrSpecVersion))
					delete(actual.msg.Headers, spec.AMQPCEHeaderKey(spec.CEAttrType))
					delete(actual.msg.Headers, spec.AMQPCEHeaderKey(spec.CEAttrSource))
					delete(actual.msg.Headers, spec.AMQPCEHeaderKey(spec.CEAttrID))
					delete(actual.msg.Headers, spec.AMQPCEHeaderKey(spec.CEAttrTime))
					delete(actual.msg.Headers, spec.AMQPCEHeaderKey(spec.CEAttrDataContentType))
					require.Equal(t, *tt.expectedPublished[i], actual)
				} else if tt.expectedError == "" {
					require.Fail(t, "nothing published, and no error wanted!")
				}
				i++
			}
		})
	}
}

func Test_Publisher_CEOverride(t *testing.T) {
	channel := NewMockAmqpChannel()
	conn := mockConnection(channel)
	p := NewPublisher()
	ctx := context.TODO()
	err := conn.Start(context.Background(), EventStreamPublisher(p))
	require.NoError(t, err)

	customID := "my-idempotent-id"
	customSubject := "order-456"
	err = p.Publish(ctx, "Order.Created", TestMessage{"test", true},
		Header{Key: spec.CEID, Value: customID},
		Header{Key: spec.CESubject, Value: customSubject},
	)
	require.NoError(t, err)

	actual := <-channel.Published
	require.Equal(t, customID, actual.msg.Headers[spec.AMQPCEHeaderKey(spec.CEAttrID)], "user-supplied ce-id should be remapped to cloudEvents:id")
	require.Equal(t, customSubject, actual.msg.Headers[spec.AMQPCEHeaderKey(spec.CEAttrSubject)], "user-supplied ce-subject should be remapped to cloudEvents:subject")
	require.Equal(t, "application/json", actual.msg.Headers[spec.AMQPCEHeaderKey(spec.CEAttrDataContentType)], "cloudEvents:datacontenttype should be auto-set")
	require.Equal(t, spec.CESpecVersionValue, actual.msg.Headers[spec.AMQPCEHeaderKey(spec.CEAttrSpecVersion)])
	require.Equal(t, "Order.Created", actual.msg.Headers[spec.AMQPCEHeaderKey(spec.CEAttrType)])
	require.NotEmpty(t, actual.msg.Headers[spec.AMQPCEHeaderKey(spec.CEAttrTime)])
}

func Test_Publisher_CEDefaultsNotOverridden(t *testing.T) {
	channel := NewMockAmqpChannel()
	conn := mockConnection(channel)
	p := NewPublisher()
	ctx := context.TODO()
	err := conn.Start(context.Background(), EventStreamPublisher(p))
	require.NoError(t, err)

	customType := "Custom.Type"
	customSource := "custom-source"
	err = p.Publish(ctx, "Order.Created", TestMessage{"test", true},
		Header{Key: spec.CEType, Value: customType},
		Header{Key: spec.CESource, Value: customSource},
	)
	require.NoError(t, err)

	actual := <-channel.Published
	require.Equal(t, customType, actual.msg.Headers[spec.AMQPCEHeaderKey(spec.CEAttrType)], "user-supplied ce-type should be remapped to cloudEvents:type")
	require.Equal(t, customSource, actual.msg.Headers[spec.AMQPCEHeaderKey(spec.CEAttrSource)], "user-supplied ce-source should be remapped to cloudEvents:source")
}

func Test_InvalidHeader(t *testing.T) {
	err := (&Publisher{}).setup(nil, "", "", nil, nil, nil, Header{Key: "", Value: ""})
	require.ErrorIs(t, err, ErrEmptyHeaderKey)
}

func Test_Publisher_ConcurrentPublish_NoRace(t *testing.T) {
	channel := NewMockAmqpChannel()
	channel.Published = make(chan Publish, 100)
	conn := mockConnection(channel)
	p := NewPublisher()
	err := conn.Start(context.Background(), EventStreamPublisher(p))
	require.NoError(t, err)

	const goroutines = 10
	const msgsPerGoroutine = 5
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for m := 0; m < msgsPerGoroutine; m++ {
				pubErr := p.Publish(context.Background(), "Order.Created", TestMessage{Msg: "test"})
				require.NoError(t, pubErr)
			}
		}(g)
	}
	wg.Wait()

	// Drain and count published messages
	total := len(channel.Published)
	require.Equal(t, goroutines*msgsPerGoroutine, total)
}

func Test_Publisher_ChannelIsolation(t *testing.T) {
	// Each publisher gets its own mock channel from c.connection.channel().
	var channels []*MockAmqpChannel
	setupChannel := NewMockAmqpChannel()

	mockConn := &MockAmqpConnection{
		channelFn: func() (amqpChannel, error) {
			ch := NewMockAmqpChannel()
			channels = append(channels, ch)
			return ch, nil
		},
	}

	conn := newConnection("svc", amqp.URI{})
	conn.setupChannel = setupChannel
	conn.connection = mockConn

	pub1 := NewPublisher()
	pub2 := NewPublisher()
	err := conn.Start(context.Background(),
		EventStreamPublisher(pub1),
		StreamPublisher("custom-stream", pub2),
	)
	require.NoError(t, err)

	// channels[0] = response channel, channels[1] = setup channel,
	// channels[2] = pub1's channel, channels[3] = pub2's channel
	require.GreaterOrEqual(t, len(channels), 4, "expected at least 4 channels: response + setup + 2 publisher")

	pub1Channel := channels[2]
	pub2Channel := channels[3]

	// Simulate channel error on pub1's channel
	pub1Channel.publishFn = func(ctx context.Context, exchange, key string, mandatory, immediate bool, msg amqp.Publishing) error {
		return errors.New("channel closed")
	}

	// pub1 should fail
	err = pub1.Publish(context.Background(), "Order.Created", TestMessage{Msg: "test"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "channel closed")

	// pub2 should still work because it uses a different channel
	err = pub2.Publish(context.Background(), "Order.Created", TestMessage{Msg: "test"})
	require.NoError(t, err)
	published := <-pub2Channel.Published
	require.Equal(t, "Order.Created", published.key)
}

func Test_Publisher_WithConfirm(t *testing.T) {
	channel := NewMockAmqpChannel()
	conn := mockConnection(channel)

	confirmCh := make(chan amqp.Confirmation, 10)
	p := NewPublisher(WithConfirm(confirmCh))
	err := conn.Start(context.Background(), EventStreamPublisher(p))
	require.NoError(t, err)

	require.True(t, channel.ConfirmCalled, "Confirm() should have been called on the publisher's channel")
	require.NotNil(t, channel.Confirms, "NotifyPublish() should have been called")
	require.Equal(t, &confirmCh, channel.Confirms, "confirm channel should match")
}

func Test_Publisher_DefaultConfirmsEnabled(t *testing.T) {
	channel := NewMockAmqpChannel()
	conn := mockConnection(channel)
	p := NewPublisher()
	err := conn.Start(context.Background(), EventStreamPublisher(p))
	require.NoError(t, err)

	require.True(t, channel.ConfirmCalled, "Confirm() should be called by default")
	require.NotNil(t, channel.Confirms, "NotifyPublish() should be called by default")
}

func Test_Publisher_WithoutPublisherConfirms(t *testing.T) {
	channel := NewMockAmqpChannel()
	conn := mockConnection(channel)
	p := NewPublisher(WithoutPublisherConfirms())
	err := conn.Start(context.Background(), EventStreamPublisher(p))
	require.NoError(t, err)

	require.False(t, channel.ConfirmCalled, "Confirm() should not be called when confirms are disabled")
	require.Nil(t, channel.Confirms, "NotifyPublish() should not be called when confirms are disabled")
}

func Test_Publisher_ConfirmAck(t *testing.T) {
	channel := NewMockAmqpChannel()
	conn := mockConnection(channel)
	p := NewPublisher()
	err := conn.Start(context.Background(), EventStreamPublisher(p))
	require.NoError(t, err)

	// Mock sends ack automatically in PublishWithContext
	err = p.Publish(context.Background(), "Order.Created", TestMessage{Msg: "test"})
	require.NoError(t, err)
}

func Test_Publisher_ConfirmNack(t *testing.T) {
	channel := NewMockAmqpChannel()
	// Override publishFn to send nack instead of ack
	channel.publishFn = func(ctx context.Context, exchange, key string, mandatory, immediate bool, msg amqp.Publishing) error {
		channel.Published <- Publish{exchange, key, mandatory, immediate, msg}
		if channel.Confirms != nil {
			*channel.Confirms <- amqp.Confirmation{
				DeliveryTag: 1,
				Ack:         false,
			}
		}
		return nil
	}
	conn := mockConnection(channel)
	p := NewPublisher()
	err := conn.Start(context.Background(), EventStreamPublisher(p))
	require.NoError(t, err)

	err = p.Publish(context.Background(), "Order.Created", TestMessage{Msg: "test"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "broker nacked publish")
}

func Test_Publisher_ConfirmContextCancelled(t *testing.T) {
	channel := NewMockAmqpChannel()
	// Override publishFn to NOT send any confirmation
	channel.publishFn = func(ctx context.Context, exchange, key string, mandatory, immediate bool, msg amqp.Publishing) error {
		channel.Published <- Publish{exchange, key, mandatory, immediate, msg}
		// Do not send confirmation — simulate a timeout
		return nil
	}
	conn := mockConnection(channel)
	p := NewPublisher()
	err := conn.Start(context.Background(), EventStreamPublisher(p))
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err = p.Publish(ctx, "Order.Created", TestMessage{Msg: "test"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "context cancelled waiting for publish confirm")
}

func Test_PublishServiceResponse_UsesResponseChannel(t *testing.T) {
	// Create distinct mock channels for response vs publisher
	responseChannel := NewMockAmqpChannel()
	publisherChannel := NewMockAmqpChannel()

	conn := newConnection("svc", amqp.URI{})
	conn.setupChannel = NewMockAmqpChannel()
	conn.responseChannel = responseChannel
	conn.connection = &MockAmqpConnection{
		channelFn: func() (amqpChannel, error) {
			return publisherChannel, nil
		},
	}

	err := conn.PublishServiceResponse(context.Background(), "target", "key", struct{}{})
	require.NoError(t, err)

	// The response should have gone through the responseChannel, not publisherChannel
	require.Equal(t, 1, len(responseChannel.Published), "publish should go through response channel")
	require.Equal(t, 0, len(publisherChannel.Published), "publish should not go through publisher channel")
}
