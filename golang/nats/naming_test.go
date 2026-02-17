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

	"github.com/stretchr/testify/assert"
)

func TestStreamName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"events", "events"},
		{"events.topic.exchange", "events"},
		{"audit", "audit"},
		{"audit.topic.exchange", "audit"},
		{"my-stream", "my-stream"},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			assert.Equal(t, tc.expected, streamName(tc.input))
		})
	}
}

func TestSubjectName(t *testing.T) {
	tests := []struct {
		stream     string
		routingKey string
		expected   string
	}{
		{"events", "Order.Created", "events.Order.Created"},
		{"audit", "Audit.Entry", "audit.Audit.Entry"},
		{"events", "User.Updated", "events.User.Updated"},
	}
	for _, tc := range tests {
		t.Run(tc.expected, func(t *testing.T) {
			assert.Equal(t, tc.expected, subjectName(tc.stream, tc.routingKey))
		})
	}
}

func TestFilterSubject(t *testing.T) {
	tests := []struct {
		stream     string
		routingKey string
		expected   string
	}{
		{"events", "Order.Created", "events.Order.Created"},
		{"events", "Order.#", "events.Order.>"},
		{"events", "Order.*", "events.Order.*"},
		{"events", "#", "events.>"},
	}
	for _, tc := range tests {
		t.Run(tc.expected, func(t *testing.T) {
			assert.Equal(t, tc.expected, filterSubject(tc.stream, tc.routingKey))
		})
	}
}

func TestStreamSubjects(t *testing.T) {
	assert.Equal(t, "events.>", streamSubjects("events"))
	assert.Equal(t, "audit.>", streamSubjects("audit"))
}

func TestServiceRequestSubject(t *testing.T) {
	assert.Equal(t, "email-svc.request.email.send", serviceRequestSubject("email-svc", "email.send"))
	assert.Equal(t, "orders.request.order.create", serviceRequestSubject("orders", "order.create"))
}

func TestConsumerName(t *testing.T) {
	assert.Equal(t, "orders", consumerName("orders"))
}

func TestConsumerNameWithSuffix(t *testing.T) {
	assert.Equal(t, "orders-analytics", consumerNameWithSuffix("orders", "analytics"))
}
