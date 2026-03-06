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

import "time"

// Headers represent all meta-data for the message.
type Headers map[string]any

// Get returns the header value for key, or nil if not present.
func (h Headers) Get(key string) any {
	if v, ok := h[key]; ok {
		return v
	}
	return nil
}

// Metadata holds the metadata of an event.
type Metadata struct {
	ID              string    `json:"id"`
	CorrelationID   string    `json:"correlationId"`
	Timestamp       time.Time `json:"timestamp"`
	Source          string    `json:"source"`
	Type            string    `json:"type"`
	Subject         string    `json:"subject,omitempty"`
	DataContentType string    `json:"dataContentType,omitempty"`
	SpecVersion     string    `json:"specVersion"`
}

// HasCloudEvents reports whether this Metadata has been populated with
// usable CloudEvents attributes. Returns true when the ID field is non-empty,
// which occurs either from real CE headers or from EnrichLegacyMetadata
// synthetic enrichment.
func (m Metadata) HasCloudEvents() bool {
	return m.ID != ""
}

// DeliveryInfo holds transport-agnostic delivery metadata.
//
//   - Destination: the queue/subscription name (AMQP: queue, NATS: subscription)
//   - Source: the exchange/subject name (AMQP: exchange, NATS: subject)
//   - Key: the routing key/subject suffix (AMQP: routing key, NATS: subject token)
type DeliveryInfo struct {
	Destination string
	Source      string
	Key         string
	Headers     Headers
}

// ConsumableEvent represents an event that can be consumed.
// The type parameter T specifies the type of the event's payload.
type ConsumableEvent[T any] struct {
	Metadata
	DeliveryInfo DeliveryInfo
	Payload      T
}
