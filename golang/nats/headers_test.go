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

	natsgo "github.com/nats-io/nats.go"
	"github.com/sparetimecoders/messaging/specification/spec"
	"github.com/stretchr/testify/assert"
)

func TestValidateKey_Empty(t *testing.T) {
	h := Header{Key: "", Value: "val"}
	assert.ErrorIs(t, h.validateKey(), ErrEmptyHeaderKey)
}

func TestValidateKey_Reserved(t *testing.T) {
	h := Header{Key: headerService, Value: "val"}
	err := h.validateKey()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "reserved key")
}

func TestValidateKey_Valid(t *testing.T) {
	h := Header{Key: "x-custom", Value: "val"}
	assert.NoError(t, h.validateKey())
}

func TestToNATSHeaders(t *testing.T) {
	tests := []struct {
		name     string
		input    spec.Headers
		expected natsgo.Header
	}{
		{
			name:     "nil headers",
			input:    nil,
			expected: natsgo.Header{},
		},
		{
			name:     "empty headers",
			input:    spec.Headers{},
			expected: natsgo.Header{},
		},
		{
			name: "string values",
			input: spec.Headers{
				"ce-type":   "Order.Created",
				"ce-source": "test-service",
			},
			expected: func() natsgo.Header {
				h := natsgo.Header{}
				h.Set("ce-type", "Order.Created")
				h.Set("ce-source", "test-service")
				return h
			}(),
		},
		{
			name: "non-string values are skipped",
			input: spec.Headers{
				"ce-type": "Order.Created",
				"count":   42,
				"flag":    true,
			},
			expected: func() natsgo.Header {
				h := natsgo.Header{}
				h.Set("ce-type", "Order.Created")
				return h
			}(),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := toNATSHeaders(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestFromNATSHeaders(t *testing.T) {
	tests := []struct {
		name     string
		input    natsgo.Header
		expected spec.Headers
	}{
		{
			name:     "nil headers",
			input:    nil,
			expected: spec.Headers{},
		},
		{
			name:     "empty headers",
			input:    natsgo.Header{},
			expected: spec.Headers{},
		},
		{
			name: "single values",
			input: func() natsgo.Header {
				h := natsgo.Header{}
				h.Set("ce-type", "Order.Created")
				h.Set("ce-source", "test-service")
				return h
			}(),
			expected: spec.Headers{
				"ce-type":   "Order.Created",
				"ce-source": "test-service",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := fromNATSHeaders(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestToFromNATSHeaders_RoundTrip(t *testing.T) {
	original := spec.Headers{
		"ce-specversion":     "1.0",
		"ce-type":            "Order.Created",
		"ce-source":          "order-service",
		"ce-id":              "abc-123",
		"ce-datacontenttype": "application/json",
	}

	natsHeaders := toNATSHeaders(original)
	result := fromNATSHeaders(natsHeaders)

	assert.Equal(t, original, result)
}
