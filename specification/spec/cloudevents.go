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
	"fmt"
	"time"
)

// Bare CloudEvents attribute names (transport-agnostic).
// Transports map these to transport-specific header keys.
const (
	CEAttrSpecVersion     = "specversion"
	CEAttrType            = "type"
	CEAttrSource          = "source"
	CEAttrID              = "id"
	CEAttrTime            = "time"
	CEAttrDataContentType = "datacontenttype"
	CEAttrSubject         = "subject"
	CEAttrDataSchema      = "dataschema"
	CEAttrCorrelationID   = "correlationid"
)

// CloudEvents attribute header keys for binary content mode.
// In binary content mode, CloudEvents attributes are carried as transport
// headers while the event data occupies the message body unchanged.
//
// NOTE: The "ce-" prefixed constants below are the canonical form used internally
// and match the HTTP/NATS CloudEvents binding convention. The AMQP transport uses
// "cloudEvents:" prefix on the wire per the AMQP binding spec, and normalizes
// incoming headers to "ce-" at the transport boundary so all spec functions work
// unchanged. See AMQPCEHeaderKey() and NormalizeCEHeaders().
//
// Idempotency: The combination of CEID + CESource forms the deduplication key.
// Consumers SHOULD be idempotent since both AMQP and NATS provide at-least-once
// delivery guarantees.
const (
	CESpecVersion      = "ce-specversion"
	CEType             = "ce-type"
	CESource           = "ce-source"
	CEID               = "ce-id"
	CETime             = "ce-time"
	CESpecVersionValue = "1.0"

	// Optional CE attributes
	CEDataContentType = "ce-datacontenttype"
	CESubject         = "ce-subject"
	CEDataSchema      = "ce-dataschema"

	// Extension attribute for correlation
	CECorrelationID = "ce-correlationid"
)

// CERequiredAttributes lists header keys required by CE 1.0.
var CERequiredAttributes = []string{CESpecVersion, CEType, CESource, CEID, CETime}

// HasCEHeaders reports whether h contains at least one CE required attribute.
// Use this to distinguish legacy (pre-CloudEvents) messages — which have no CE
// headers at all — from malformed CE messages that are missing some attributes.
func HasCEHeaders(h Headers) bool {
	for _, key := range CERequiredAttributes {
		if _, ok := h[key]; ok {
			return true
		}
	}
	return false
}

// MetadataFromHeaders extracts CloudEvents metadata from message headers
// into a Metadata struct.
func MetadataFromHeaders(h Headers) Metadata {
	var m Metadata
	m.ID = headerString(h, CEID)
	m.Source = headerString(h, CESource)
	m.Type = headerString(h, CEType)
	m.Subject = headerString(h, CESubject)
	m.DataContentType = headerString(h, CEDataContentType)
	m.SpecVersion = headerString(h, CESpecVersion)
	m.CorrelationID = headerString(h, CECorrelationID)
	if s := headerString(h, CETime); s != "" {
		if parsed, err := time.Parse(time.RFC3339, s); err == nil {
			m.Timestamp = parsed
		}
	}
	return m
}

// ValidateCEHeaders checks that all required CloudEvents 1.0 attributes
// are present and are strings. It returns a list of warnings for any
// missing or non-string attributes. An empty slice means all required
// attributes are valid.
func ValidateCEHeaders(h Headers) []string {
	var warnings []string
	for _, key := range CERequiredAttributes {
		v, ok := h[key]
		if !ok {
			warnings = append(warnings, fmt.Sprintf("missing required attribute %q", key))
			continue
		}
		if _, ok := v.(string); !ok {
			warnings = append(warnings, fmt.Sprintf("attribute %q is not a string", key))
		}
	}
	return warnings
}

// IDGenerator is a function that generates a unique identifier string.
// The amqp and nats transports typically supply uuid.New().String().
type IDGenerator func() string

// EnrichLegacyMetadata populates a zero-valued Metadata with synthetic
// CloudEvents attributes derived from transport delivery information.
// This mirrors the publisher-side setDefault pattern.
//
// Fields set when empty: ID (from idGen), Timestamp (time.Now().UTC()),
// Type (from info.Key), Source (from info.Source),
// DataContentType ("application/json"), SpecVersion ("1.0").
//
// Fields NOT set: CorrelationID, Subject (cannot be inferred).
// Any non-empty fields in m are preserved (not overwritten).
func EnrichLegacyMetadata(m Metadata, info DeliveryInfo, idGen IDGenerator) Metadata {
	if m.ID == "" && idGen != nil {
		m.ID = idGen()
	}
	if m.Timestamp.IsZero() {
		m.Timestamp = time.Now().UTC()
	}
	if m.Type == "" {
		m.Type = info.Key
	}
	if m.Source == "" {
		m.Source = info.Source
	}
	if m.DataContentType == "" {
		m.DataContentType = "application/json"
	}
	if m.SpecVersion == "" {
		m.SpecVersion = CESpecVersionValue
	}
	return m
}

func headerString(h Headers, key string) string {
	if v, ok := h[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
