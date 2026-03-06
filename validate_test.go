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
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func loadValidateFixtures(t *testing.T) validateFixtures {
	t.Helper()
	data, err := os.ReadFile("testdata/validate.json")
	if err != nil {
		t.Fatalf("failed to read validate fixtures: %v", err)
	}
	var f validateFixtures
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("failed to parse validate fixtures: %v", err)
	}
	return f
}

func TestValidate(t *testing.T) {
	for _, tc := range loadValidateFixtures(t).Single {
		t.Run(tc.Name, func(t *testing.T) {
			err := Validate(tc.Topology)
			if tc.ExpectedError == nil {
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
			} else {
				if err == nil || !strings.Contains(err.Error(), *tc.ExpectedError) {
					t.Errorf("expected error containing %q, got: %v", *tc.ExpectedError, err)
				}
			}
		})
	}
}

func TestValidateTopologies(t *testing.T) {
	for _, tc := range loadValidateFixtures(t).Cross {
		t.Run(tc.Name, func(t *testing.T) {
			err := ValidateTopologies(tc.Topologies)
			if tc.ExpectedError == nil {
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
			} else {
				if err == nil || !strings.Contains(err.Error(), *tc.ExpectedError) {
					t.Errorf("expected error containing %q, got: %v", *tc.ExpectedError, err)
				}
			}
		})
	}
}
