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

func TestHeaders_Get_Existing(t *testing.T) {
	h := Headers{"key": "value"}
	got := h.Get("key")
	if got != "value" {
		t.Errorf("Get(key) = %v, want %v", got, "value")
	}
}

func TestHeaders_Get_Missing(t *testing.T) {
	h := Headers{"key": "value"}
	got := h.Get("missing")
	if got != nil {
		t.Errorf("Get(missing) = %v, want nil", got)
	}
}

func TestHeaders_Get_NilMap(t *testing.T) {
	var h Headers
	got := h.Get("key")
	if got != nil {
		t.Errorf("Get(key) on nil = %v, want nil", got)
	}
}

func TestMetadata_HasCloudEvents(t *testing.T) {
	tests := []struct {
		name string
		m    Metadata
		want bool
	}{
		{"zero-valued metadata", Metadata{}, false},
		{"metadata with ID", Metadata{ID: "abc"}, true},
		{"metadata with everything except ID", Metadata{Source: "svc", Type: "Event", SpecVersion: "1.0", Timestamp: time.Now()}, false},
		{"fully populated", Metadata{ID: "abc", Source: "svc", Type: "Event"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.m.HasCloudEvents(); got != tt.want {
				t.Errorf("HasCloudEvents() = %v, want %v", got, tt.want)
			}
		})
	}
}
