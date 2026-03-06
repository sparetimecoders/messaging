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
	"encoding/json"
	"os"
	"testing"
)

func loadNamingFixtures(t *testing.T) namingFixtures {
	t.Helper()
	data, err := os.ReadFile("testdata/naming.json")
	if err != nil {
		t.Fatalf("failed to read naming fixtures: %v", err)
	}
	var f namingFixtures
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("failed to parse naming fixtures: %v", err)
	}
	return f
}

func TestTopicExchangeName(t *testing.T) {
	for _, tc := range loadNamingFixtures(t).TopicExchangeName {
		t.Run(tc.Input.Name, func(t *testing.T) {
			got := TopicExchangeName(tc.Input.Name)
			if got != tc.Expected {
				t.Errorf("TopicExchangeName(%q) = %q, want %q", tc.Input.Name, got, tc.Expected)
			}
		})
	}
}

func TestServiceEventQueueName(t *testing.T) {
	for _, tc := range loadNamingFixtures(t).ServiceEventQueueName {
		t.Run(tc.Input.ExchangeName+"_"+tc.Input.Service, func(t *testing.T) {
			got := ServiceEventQueueName(tc.Input.ExchangeName, tc.Input.Service)
			if got != tc.Expected {
				t.Errorf("ServiceEventQueueName(%q, %q) = %q, want %q",
					tc.Input.ExchangeName, tc.Input.Service, got, tc.Expected)
			}
		})
	}
}

func TestServiceRequestExchangeName(t *testing.T) {
	for _, tc := range loadNamingFixtures(t).ServiceRequestExchangeName {
		t.Run(tc.Input.Service, func(t *testing.T) {
			got := ServiceRequestExchangeName(tc.Input.Service)
			if got != tc.Expected {
				t.Errorf("ServiceRequestExchangeName(%q) = %q, want %q",
					tc.Input.Service, got, tc.Expected)
			}
		})
	}
}

func TestServiceResponseExchangeName(t *testing.T) {
	for _, tc := range loadNamingFixtures(t).ServiceResponseExchangeName {
		t.Run(tc.Input.Service, func(t *testing.T) {
			got := ServiceResponseExchangeName(tc.Input.Service)
			if got != tc.Expected {
				t.Errorf("ServiceResponseExchangeName(%q) = %q, want %q",
					tc.Input.Service, got, tc.Expected)
			}
		})
	}
}

func TestServiceRequestQueueName(t *testing.T) {
	for _, tc := range loadNamingFixtures(t).ServiceRequestQueueName {
		t.Run(tc.Input.Service, func(t *testing.T) {
			got := ServiceRequestQueueName(tc.Input.Service)
			if got != tc.Expected {
				t.Errorf("ServiceRequestQueueName(%q) = %q, want %q",
					tc.Input.Service, got, tc.Expected)
			}
		})
	}
}

func TestServiceResponseQueueName(t *testing.T) {
	for _, tc := range loadNamingFixtures(t).ServiceResponseQueueName {
		t.Run(tc.Input.TargetService+"_"+tc.Input.ServiceName, func(t *testing.T) {
			got := ServiceResponseQueueName(tc.Input.TargetService, tc.Input.ServiceName)
			if got != tc.Expected {
				t.Errorf("ServiceResponseQueueName(%q, %q) = %q, want %q",
					tc.Input.TargetService, tc.Input.ServiceName, got, tc.Expected)
			}
		})
	}
}

func TestNATSStreamName(t *testing.T) {
	for _, tc := range loadNamingFixtures(t).NATSStreamName {
		t.Run(tc.Input.Name, func(t *testing.T) {
			got := NATSStreamName(tc.Input.Name)
			if got != tc.Expected {
				t.Errorf("NATSStreamName(%q) = %q, want %q", tc.Input.Name, got, tc.Expected)
			}
		})
	}
}

func TestNATSSubject(t *testing.T) {
	for _, tc := range loadNamingFixtures(t).NATSSubject {
		t.Run(tc.Input.Stream+"_"+tc.Input.RoutingKey, func(t *testing.T) {
			got := NATSSubject(tc.Input.Stream, tc.Input.RoutingKey)
			if got != tc.Expected {
				t.Errorf("NATSSubject(%q, %q) = %q, want %q",
					tc.Input.Stream, tc.Input.RoutingKey, got, tc.Expected)
			}
		})
	}
}

func TestTranslateWildcard(t *testing.T) {
	for _, tc := range loadNamingFixtures(t).TranslateWildcard {
		t.Run(tc.Input.RoutingKey, func(t *testing.T) {
			got := TranslateWildcard(tc.Input.RoutingKey)
			if got != tc.Expected {
				t.Errorf("TranslateWildcard(%q) = %q, want %q",
					tc.Input.RoutingKey, got, tc.Expected)
			}
		})
	}
}
