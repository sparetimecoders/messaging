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

import "strings"

// AMQPCEHeaderKey returns the AMQP application-properties key for a bare
// CloudEvents attribute name, using the "cloudEvents:" prefix per the
// CloudEvents AMQP Protocol Binding specification.
// Example: AMQPCEHeaderKey("specversion") -> "cloudEvents:specversion"
func AMQPCEHeaderKey(attr string) string {
	return "cloudEvents:" + attr
}

// NormalizeCEHeaders rewrites incoming transport headers so that all
// CloudEvents attributes use the canonical "ce-" prefix. This allows
// consumers to accept messages with any known prefix variant:
//   - "cloudEvents:specversion" -> "ce-specversion"
//   - "cloudEvents_specversion" -> "ce-specversion"  (JMS compat)
//   - "ce-specversion" -> unchanged
//
// Non-CE headers are preserved unchanged. The original map is not modified;
// a new map is returned.
func NormalizeCEHeaders(h Headers) Headers {
	out := make(Headers, len(h))
	for k, v := range h {
		switch {
		case strings.HasPrefix(k, "cloudEvents:"):
			out["ce-"+strings.TrimPrefix(k, "cloudEvents:")] = v
		case strings.HasPrefix(k, "cloudEvents_"):
			out["ce-"+strings.TrimPrefix(k, "cloudEvents_")] = v
		default:
			out[k] = v
		}
	}
	return out
}
