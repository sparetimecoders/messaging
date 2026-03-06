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
	"time"
)

func TestMetadataFromHeaders(t *testing.T) {
	tests := []struct {
		name                string
		headers             Headers
		wantID              string
		wantTime            time.Time
		wantSource          string
		wantType            string
		wantSubject         string
		wantDataContentType string
		wantSpecVersion     string
		wantCorrelationID   string
	}{
		{
			name: "all CE headers present",
			headers: Headers{
				CEID:              "abc-123",
				CETime:            "2025-06-15T10:30:00Z",
				CESource:          "orders-svc",
				CEType:            "Order.Created",
				CESubject:         "order-456",
				CEDataContentType: "application/json",
				CESpecVersion:     CESpecVersionValue,
				CECorrelationID:   "corr-789",
			},
			wantID:              "abc-123",
			wantTime:            time.Date(2025, 6, 15, 10, 30, 0, 0, time.UTC),
			wantSource:          "orders-svc",
			wantType:            "Order.Created",
			wantSubject:         "order-456",
			wantDataContentType: "application/json",
			wantSpecVersion:     CESpecVersionValue,
			wantCorrelationID:   "corr-789",
		},
		{
			name:    "nil headers",
			headers: nil,
			wantID:  "",
		},
		{
			name:    "empty headers",
			headers: Headers{},
			wantID:  "",
		},
		{
			name:    "only id",
			headers: Headers{CEID: "id-only"},
			wantID:  "id-only",
		},
		{
			name:     "only time",
			headers:  Headers{CETime: "2025-01-01T00:00:00Z"},
			wantID:   "",
			wantTime: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name:    "non-string id ignored",
			headers: Headers{CEID: 42},
			wantID:  "",
		},
		{
			name:    "invalid time ignored",
			headers: Headers{CETime: "not-a-time"},
			wantID:  "",
		},
		{
			name:    "non-string time ignored",
			headers: Headers{CETime: 12345},
			wantID:  "",
		},
		{
			name: "source and type only",
			headers: Headers{
				CESource: "my-service",
				CEType:   "Event.Happened",
			},
			wantSource: "my-service",
			wantType:   "Event.Happened",
		},
		{
			name: "non-string source ignored",
			headers: Headers{
				CESource: 42,
			},
			wantSource: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := MetadataFromHeaders(tt.headers)
			if m.ID != tt.wantID {
				t.Errorf("ID = %q, want %q", m.ID, tt.wantID)
			}
			if !m.Timestamp.Equal(tt.wantTime) {
				t.Errorf("Timestamp = %v, want %v", m.Timestamp, tt.wantTime)
			}
			if m.Source != tt.wantSource {
				t.Errorf("Source = %q, want %q", m.Source, tt.wantSource)
			}
			if m.Type != tt.wantType {
				t.Errorf("Type = %q, want %q", m.Type, tt.wantType)
			}
			if m.Subject != tt.wantSubject {
				t.Errorf("Subject = %q, want %q", m.Subject, tt.wantSubject)
			}
			if m.DataContentType != tt.wantDataContentType {
				t.Errorf("DataContentType = %q, want %q", m.DataContentType, tt.wantDataContentType)
			}
			if m.SpecVersion != tt.wantSpecVersion {
				t.Errorf("SpecVersion = %q, want %q", m.SpecVersion, tt.wantSpecVersion)
			}
			if m.CorrelationID != tt.wantCorrelationID {
				t.Errorf("CorrelationID = %q, want %q", m.CorrelationID, tt.wantCorrelationID)
			}
		})
	}
}

func TestHasCEHeaders(t *testing.T) {
	tests := []struct {
		name    string
		headers Headers
		want    bool
	}{
		{
			name:    "nil headers",
			headers: nil,
			want:    false,
		},
		{
			name:    "empty headers",
			headers: Headers{},
			want:    false,
		},
		{
			name:    "only non-CE headers (legacy message)",
			headers: Headers{"service": "my-svc", "x-custom": "value"},
			want:    false,
		},
		{
			name:    "single CE header present",
			headers: Headers{CESpecVersion: CESpecVersionValue},
			want:    true,
		},
		{
			name: "all required CE headers present",
			headers: Headers{
				CESpecVersion: CESpecVersionValue,
				CEType:        "Order.Created",
				CESource:      "orders-svc",
				CEID:          "evt-123",
				CETime:        "2025-06-15T10:30:00Z",
			},
			want: true,
		},
		{
			name:    "CE header with non-string value still counts",
			headers: Headers{CEID: 42},
			want:    true,
		},
		{
			name:    "only optional CE headers (no required ones)",
			headers: Headers{CEDataContentType: "application/json", CESubject: "order-1"},
			want:    false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HasCEHeaders(tt.headers)
			if got != tt.want {
				t.Errorf("HasCEHeaders() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEnrichLegacyMetadata(t *testing.T) {
	fixedID := "test-uuid-123"
	idGen := func() string { return fixedID }

	t.Run("enriches zero-valued metadata", func(t *testing.T) {
		info := DeliveryInfo{Key: "Order.Created", Source: "events.topic.exchange"}
		before := time.Now().UTC()
		m := EnrichLegacyMetadata(Metadata{}, info, idGen)
		after := time.Now().UTC()

		if m.ID != fixedID {
			t.Errorf("ID = %q, want %q", m.ID, fixedID)
		}
		if m.Type != "Order.Created" {
			t.Errorf("Type = %q, want %q", m.Type, "Order.Created")
		}
		if m.Source != "events.topic.exchange" {
			t.Errorf("Source = %q, want %q", m.Source, "events.topic.exchange")
		}
		if m.Timestamp.Before(before) || m.Timestamp.After(after) {
			t.Errorf("Timestamp = %v, want between %v and %v", m.Timestamp, before, after)
		}
		if m.SpecVersion != CESpecVersionValue {
			t.Errorf("SpecVersion = %q, want %q", m.SpecVersion, CESpecVersionValue)
		}
		if m.DataContentType != "application/json" {
			t.Errorf("DataContentType = %q, want %q", m.DataContentType, "application/json")
		}
	})

	t.Run("preserves existing non-empty fields", func(t *testing.T) {
		ts := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		existing := Metadata{ID: "existing-id", Type: "Custom.Type", Source: "custom-source", Timestamp: ts}
		info := DeliveryInfo{Key: "Order.Created", Source: "events.topic.exchange"}
		m := EnrichLegacyMetadata(existing, info, idGen)

		if m.ID != "existing-id" {
			t.Errorf("ID overwritten: got %q, want %q", m.ID, "existing-id")
		}
		if m.Type != "Custom.Type" {
			t.Errorf("Type overwritten: got %q, want %q", m.Type, "Custom.Type")
		}
		if m.Source != "custom-source" {
			t.Errorf("Source overwritten: got %q, want %q", m.Source, "custom-source")
		}
		if !m.Timestamp.Equal(ts) {
			t.Errorf("Timestamp overwritten: got %v, want %v", m.Timestamp, ts)
		}
	})

	t.Run("nil idGen skips ID generation", func(t *testing.T) {
		m := EnrichLegacyMetadata(Metadata{}, DeliveryInfo{Key: "k"}, nil)
		if m.ID != "" {
			t.Errorf("ID = %q, want empty", m.ID)
		}
	})

	t.Run("preserves CorrelationID and Subject untouched", func(t *testing.T) {
		m := EnrichLegacyMetadata(Metadata{CorrelationID: "corr"}, DeliveryInfo{Key: "k"}, idGen)
		if m.CorrelationID != "corr" {
			t.Errorf("CorrelationID = %q, want %q", m.CorrelationID, "corr")
		}
		if m.Subject != "" {
			t.Errorf("Subject = %q, want empty", m.Subject)
		}
	})
}

func TestValidateCEHeaders(t *testing.T) {
	tests := []struct {
		name         string
		headers      Headers
		wantWarnings int
	}{
		{
			name: "all required present",
			headers: Headers{
				CESpecVersion: CESpecVersionValue,
				CEType:        "Order.Created",
				CESource:      "orders-svc",
				CEID:          "evt-123",
				CETime:        "2025-06-15T10:30:00Z",
			},
			wantWarnings: 0,
		},
		{
			name:         "nil headers",
			headers:      nil,
			wantWarnings: 5,
		},
		{
			name:         "empty headers",
			headers:      Headers{},
			wantWarnings: 5,
		},
		{
			name: "partially missing",
			headers: Headers{
				CESpecVersion: CESpecVersionValue,
				CEID:          "evt-123",
			},
			wantWarnings: 3,
		},
		{
			name: "non-string values",
			headers: Headers{
				CESpecVersion: 1.0,
				CEType:        42,
				CESource:      true,
				CEID:          123,
				CETime:        99,
			},
			wantWarnings: 5,
		},
		{
			name: "mix of valid and non-string",
			headers: Headers{
				CESpecVersion: CESpecVersionValue,
				CEType:        "Order.Created",
				CESource:      42, // non-string
				CEID:          "evt-123",
				CETime:        "2025-06-15T10:30:00Z",
			},
			wantWarnings: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			warnings := ValidateCEHeaders(tt.headers)
			if len(warnings) != tt.wantWarnings {
				t.Errorf("got %d warnings, want %d: %v", len(warnings), tt.wantWarnings, warnings)
			}
		})
	}
}
