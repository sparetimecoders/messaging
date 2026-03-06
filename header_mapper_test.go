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

package messaging

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_AMQPCEHeaderKey_SpecVersion(t *testing.T) {
	require.Equal(t, "cloudEvents:specversion", AMQPCEHeaderKey(CEAttrSpecVersion))
}

func Test_AMQPCEHeaderKey_CorrelationID(t *testing.T) {
	require.Equal(t, "cloudEvents:correlationid", AMQPCEHeaderKey(CEAttrCorrelationID))
}

func Test_NormalizeCEHeaders_CloudEventsColon(t *testing.T) {
	h := Headers{
		"cloudEvents:type":        "Order.Created",
		"cloudEvents:specversion": "1.0",
	}
	out := NormalizeCEHeaders(h)
	require.Equal(t, "Order.Created", out["ce-type"])
	require.Equal(t, "1.0", out["ce-specversion"])
}

func Test_NormalizeCEHeaders_CloudEventsUnderscore(t *testing.T) {
	h := Headers{
		"cloudEvents_type":        "Order.Created",
		"cloudEvents_specversion": "1.0",
	}
	out := NormalizeCEHeaders(h)
	require.Equal(t, "Order.Created", out["ce-type"])
	require.Equal(t, "1.0", out["ce-specversion"])
}

func Test_NormalizeCEHeaders_CEDashPassthrough(t *testing.T) {
	h := Headers{
		"ce-type":        "Order.Created",
		"ce-specversion": "1.0",
	}
	out := NormalizeCEHeaders(h)
	require.Equal(t, "Order.Created", out["ce-type"])
	require.Equal(t, "1.0", out["ce-specversion"])
}

func Test_NormalizeCEHeaders_PreservesNonCEHeaders(t *testing.T) {
	h := Headers{
		"service":     "my-svc",
		"traceparent": "00-abc-def-01",
		"x-custom":    "value",
	}
	out := NormalizeCEHeaders(h)
	require.Equal(t, "my-svc", out["service"])
	require.Equal(t, "00-abc-def-01", out["traceparent"])
	require.Equal(t, "value", out["x-custom"])
}

func Test_NormalizeCEHeaders_EmptyHeaders(t *testing.T) {
	out := NormalizeCEHeaders(Headers{})
	require.Empty(t, out)
}

func Test_NormalizeCEHeaders_MixedPrefixes(t *testing.T) {
	h := Headers{
		"cloudEvents:specversion": "1.0",
		"cloudEvents_type":        "Order.Created",
		"ce-source":               "orders-svc",
		"service":                 "my-svc",
	}
	out := NormalizeCEHeaders(h)
	require.Equal(t, "1.0", out["ce-specversion"])
	require.Equal(t, "Order.Created", out["ce-type"])
	require.Equal(t, "orders-svc", out["ce-source"])
	require.Equal(t, "my-svc", out["service"])
}

func Test_NormalizeCEHeaders_DoesNotModifyOriginal(t *testing.T) {
	h := Headers{
		"cloudEvents:type": "Order.Created",
	}
	_ = NormalizeCEHeaders(h)
	require.Equal(t, "Order.Created", h["cloudEvents:type"])
	_, exists := h["ce-type"]
	require.False(t, exists, "original map should not be modified")
}

func Test_NormalizeCEHeaders_ThenHasCEHeaders(t *testing.T) {
	h := Headers{
		"cloudEvents:specversion": "1.0",
		"cloudEvents:type":        "Order.Created",
		"cloudEvents:source":      "orders-svc",
		"cloudEvents:id":          "evt-123",
		"cloudEvents:time":        "2025-06-15T10:30:00Z",
	}
	out := NormalizeCEHeaders(h)
	require.True(t, HasCEHeaders(out))
}

func Test_NormalizeCEHeaders_ThenMetadataFromHeaders(t *testing.T) {
	h := Headers{
		"cloudEvents:specversion": "1.0",
		"cloudEvents:type":        "Order.Created",
		"cloudEvents:source":      "orders-svc",
		"cloudEvents:id":          "evt-123",
		"cloudEvents:time":        "2025-06-15T10:30:00Z",
	}
	out := NormalizeCEHeaders(h)
	m := MetadataFromHeaders(out)
	require.Equal(t, "evt-123", m.ID)
	require.Equal(t, "orders-svc", m.Source)
	require.Equal(t, "Order.Created", m.Type)
	require.Equal(t, "1.0", m.SpecVersion)
	require.False(t, m.Timestamp.IsZero())
}

func Test_NormalizeCEHeaders_ThenValidateCEHeaders(t *testing.T) {
	h := Headers{
		"cloudEvents:specversion": "1.0",
		"cloudEvents:type":        "Order.Created",
		"cloudEvents:source":      "orders-svc",
		"cloudEvents:id":          "evt-123",
		"cloudEvents:time":        "2025-06-15T10:30:00Z",
	}
	out := NormalizeCEHeaders(h)
	warnings := ValidateCEHeaders(out)
	require.Empty(t, warnings)
}
