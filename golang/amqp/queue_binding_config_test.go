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
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/require"
)

func TestEmptyQueueNameSuffix(t *testing.T) {
	require.EqualError(t, AddQueueNameSuffix("")(&consumerConfig{}), ErrEmptySuffix.Error())
}

func TestQueueNameSuffix(t *testing.T) {
	cfg := &consumerConfig{queueName: "queue"}
	require.NoError(t, AddQueueNameSuffix("suffix")(cfg))
	require.Equal(t, "queue-suffix", cfg.queueName)
}

func TestDisableSingleActiveConsumer(t *testing.T) {
	cfg := &consumerConfig{
		queueHeaders: amqp.Table{amqp.SingleActiveConsumerArg: true},
	}
	require.NoError(t, DisableSingleActiveConsumer()(cfg))
	require.Equal(t, false, cfg.queueHeaders[amqp.SingleActiveConsumerArg])
}

func TestWithDeadLetter(t *testing.T) {
	cfg := &consumerConfig{
		queueHeaders: amqp.Table{},
	}
	require.NoError(t, WithDeadLetter("my-dlx")(cfg))
	require.Equal(t, "my-dlx", cfg.queueHeaders["x-dead-letter-exchange"])
}

func TestWithDeadLetterRoutingKey(t *testing.T) {
	cfg := &consumerConfig{
		queueHeaders: amqp.Table{},
	}
	require.NoError(t, WithDeadLetter("my-dlx")(cfg))
	require.NoError(t, WithDeadLetterRoutingKey("dead-letter-key")(cfg))
	require.Equal(t, "my-dlx", cfg.queueHeaders["x-dead-letter-exchange"])
	require.Equal(t, "dead-letter-key", cfg.queueHeaders["x-dead-letter-routing-key"])
}

func TestWithDeadLetter_EmptyExchange(t *testing.T) {
	cfg := &consumerConfig{
		queueHeaders: amqp.Table{},
	}
	err := WithDeadLetter("")(cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "dead letter exchange must not be empty")
}
