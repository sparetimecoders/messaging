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

package spec

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMatchRoutingKey(t *testing.T) {
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
		{"invalid regex pattern", "[", "anything", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, MatchRoutingKey(tc.pattern, tc.routingKey))
		})
	}
}

func TestRoutingKeyToRegex(t *testing.T) {
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
			assert.Equal(t, tc.expected, routingKeyToRegex(tc.input))
		})
	}
}

func TestRoutingKeyOverlaps(t *testing.T) {
	tests := []struct {
		name     string
		p1       string
		p2       string
		expected bool
	}{
		{"identical patterns", "Order.Created", "Order.Created", true},
		{"no overlap", "Order.Created", "Order.Deleted", false},
		{"wildcard overlaps exact", "Order.#", "Order.Created", true},
		{"exact overlaps wildcard", "Order.Created", "Order.#", true},
		{"star overlaps exact", "Order.*", "Order.Created", true},
		{"non-overlapping wildcards", "Order.*", "User.*", false},
		{"hash overlaps star", "Order.#", "Order.*", true},
		{"invalid regex", "[", "Order.Created", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, RoutingKeyOverlaps(tc.p1, tc.p2))
		})
	}
}
