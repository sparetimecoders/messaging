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

package tck

import (
	"encoding/json"
	"testing"

	"github.com/sparetimecoders/messaging/spectest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNameMapper(t *testing.T) {
	mapper := NewNameMapper([]string{"orders", "notifications"})

	// Runtime names should have suffix.
	assert.Contains(t, mapper.Runtime("orders"), "orders-")
	assert.Contains(t, mapper.Runtime("notifications"), "notifications-")

	// Suffix is 8 hex chars.
	assert.Len(t, mapper.Suffix(), 8)

	// Round-trip.
	assert.Equal(t, "orders", mapper.Template(mapper.Runtime("orders")))
	assert.Equal(t, "notifications", mapper.Template(mapper.Runtime("notifications")))

	// Unknown names pass through.
	assert.Equal(t, "unknown", mapper.Runtime("unknown"))
	assert.Equal(t, "unknown", mapper.Template("unknown"))
}

func TestNameMapperUniqueSuffix(t *testing.T) {
	m1 := NewNameMapper([]string{"a"})
	m2 := NewNameMapper([]string{"a"})
	assert.NotEqual(t, m1.Suffix(), m2.Suffix(), "two mappers should have different suffixes")
}

func TestMapIntents(t *testing.T) {
	mapper := NewNameMapper([]string{"email-svc", "web-app"})

	intents := []spectest.SetupIntent{
		{Pattern: "service-request", Direction: "publish", TargetService: "email-svc"},
		{Pattern: "event-stream", Direction: "publish"},
	}

	mapped := mapper.MapIntents(intents)
	assert.Equal(t, mapper.Runtime("email-svc"), mapped[0].TargetService)
	assert.Equal(t, "", mapped[1].TargetService) // no target service, unchanged

	// Original not modified.
	assert.Equal(t, "email-svc", intents[0].TargetService)
}

func TestInjectNonce(t *testing.T) {
	payload := json.RawMessage(`{"orderId":"test-123","amount":42}`)
	modified, nonce := InjectNonce(payload)

	require.NotEmpty(t, nonce)

	var obj map[string]any
	require.NoError(t, json.Unmarshal(modified, &obj))

	assert.Equal(t, nonce, obj["_tckNonce"])
	assert.Equal(t, "test-123", obj["orderId"])
	assert.Equal(t, float64(42), obj["amount"])
}

func TestInjectNonceUnique(t *testing.T) {
	payload := json.RawMessage(`{}`)
	_, nonce1 := InjectNonce(payload)
	_, nonce2 := InjectNonce(payload)
	assert.NotEqual(t, nonce1, nonce2)
}

func TestInjectNoncePreservesOriginal(t *testing.T) {
	original := json.RawMessage(`{"key":"value"}`)
	_, _ = InjectNonce(original)

	// Original should not be modified.
	var obj map[string]any
	require.NoError(t, json.Unmarshal(original, &obj))
	_, hasNonce := obj["_tckNonce"]
	assert.False(t, hasNonce, "original payload should not be modified")
}
