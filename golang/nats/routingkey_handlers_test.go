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
	"testing"

	"github.com/stretchr/testify/assert"
)

func noopHandler(_ context.Context, _ unmarshalEvent) error {
	return nil
}

func TestRoutingKeyHandler_ExactMatch(t *testing.T) {
	handlers := make(routingKeyHandler)
	called := false
	handlers.add("Order.Created", func(ctx context.Context, event unmarshalEvent) error {
		called = true
		return nil
	})

	h, ok := handlers.get("Order.Created")
	assert.True(t, ok)
	assert.NotNil(t, h)
	_ = h(context.Background(), unmarshalEvent{})
	assert.True(t, called)
}

func TestRoutingKeyHandler_NoMatch(t *testing.T) {
	handlers := make(routingKeyHandler)
	handlers.add("Order.Created", noopHandler)

	h, ok := handlers.get("Order.Deleted")
	assert.False(t, ok)
	assert.Nil(t, h)
}

func TestRoutingKeyHandler_WildcardStar(t *testing.T) {
	handlers := make(routingKeyHandler)
	handlers.add("Order.*", noopHandler)

	// Matches single word
	h, ok := handlers.get("Order.Created")
	assert.True(t, ok)
	assert.NotNil(t, h)

	// Does not match multi-level
	h, ok = handlers.get("Order.Created.V2")
	assert.False(t, ok)
	assert.Nil(t, h)
}

func TestRoutingKeyHandler_WildcardHash(t *testing.T) {
	handlers := make(routingKeyHandler)
	handlers.add("Order.#", noopHandler)

	// Matches single word
	h, ok := handlers.get("Order.Created")
	assert.True(t, ok)
	assert.NotNil(t, h)

	// Matches multi-level
	h, ok = handlers.get("Order.Created.V2")
	assert.True(t, ok)
	assert.NotNil(t, h)
}

func TestRoutingKeyHandler_Empty(t *testing.T) {
	handlers := make(routingKeyHandler)
	h, ok := handlers.get("anything")
	assert.False(t, ok)
	assert.Nil(t, h)
}

func TestMatch(t *testing.T) {
	tests := []struct {
		name       string
		pattern    string
		routingKey string
		expected   bool
	}{
		{"exact match", "Order.Created", "Order.Created", true},
		{"no match", "Order.Created", "Order.Deleted", false},
		{"star wildcard matches single", "Order.*", "Order.Created", true},
		{"star wildcard does not match multi", "Order.*", "Order.Created.V2", false},
		{"hash wildcard matches single", "Order.#", "Order.Created", true},
		{"hash wildcard matches multi", "Order.#", "Order.Created.V2", true},
		{"hash wildcard matches everything", "#", "Order.Created", true},
		{"star in middle", "*.Created", "Order.Created", true},
		{"star in middle no match", "*.Created", "Order.Deleted", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, match(tc.pattern, tc.routingKey))
		})
	}
}

func TestFixRegex(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Order.Created", `^Order\.Created$`},
		{"Order.*", `^Order\.[^.]*$`},
		{"Order.#", `^Order\..*$`},
		{"#", `^.*$`},
		{"*.Created", `^[^.]*\.Created$`},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			assert.Equal(t, tc.expected, fixRegex(tc.input))
		})
	}
}
