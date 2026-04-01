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
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/sparetimecoders/messaging/spectest"
)

// NameMapper handles bidirectional mapping between template and runtime service names.
// Template names are the static names from tck.json (e.g. "orders").
// Runtime names include a random suffix (e.g. "orders-a8f3b2c1").
type NameMapper struct {
	suffix            string
	templateToRuntime map[string]string
	runtimeToTemplate map[string]string
}

// NewNameMapper creates a NameMapper with a random 8-hex-char suffix
// for the given template names.
func NewNameMapper(templateNames []string) *NameMapper {
	suffix := randomSuffix()
	m := &NameMapper{
		suffix:            suffix,
		templateToRuntime: make(map[string]string, len(templateNames)),
		runtimeToTemplate: make(map[string]string, len(templateNames)),
	}
	for _, name := range templateNames {
		runtime := name + "-" + suffix
		m.templateToRuntime[name] = runtime
		m.runtimeToTemplate[runtime] = name
	}
	return m
}

// Runtime returns the runtime name for a template name.
// Unknown names pass through unchanged.
func (m *NameMapper) Runtime(templateName string) string {
	if r, ok := m.templateToRuntime[templateName]; ok {
		return r
	}
	return templateName
}

// Template returns the template name for a runtime name.
// Unknown names pass through unchanged.
func (m *NameMapper) Template(runtimeName string) string {
	if t, ok := m.runtimeToTemplate[runtimeName]; ok {
		return t
	}
	return runtimeName
}

// Suffix returns the random suffix used for this mapper.
func (m *NameMapper) Suffix() string {
	return m.suffix
}

// MapIntents creates a copy of intents with TargetService mapped to runtime names.
// Other fields (Pattern, Direction, RoutingKey, Exchange) are unchanged.
func (m *NameMapper) MapIntents(intents []spectest.SetupIntent) []spectest.SetupIntent {
	mapped := make([]spectest.SetupIntent, len(intents))
	for i, intent := range intents {
		mapped[i] = intent
		if intent.TargetService != "" {
			mapped[i].TargetService = m.Runtime(intent.TargetService)
		}
	}
	return mapped
}

// InjectNonce adds a unique "_tckNonce" field to a JSON payload.
// Returns the modified payload and the nonce value.
func InjectNonce(payload json.RawMessage) (json.RawMessage, string) {
	nonce := uuid.New().String()
	var obj map[string]any
	if err := json.Unmarshal(payload, &obj); err != nil {
		obj = make(map[string]any)
	}
	obj["_tckNonce"] = nonce
	modified, _ := json.Marshal(obj)
	return modified, nonce
}

func randomSuffix() string {
	b := make([]byte, 4) // 4 bytes = 8 hex chars
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("tck: crypto/rand.Read failed: %v", err))
	}
	return hex.EncodeToString(b)
}
