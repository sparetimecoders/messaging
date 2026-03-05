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

package nats

import (
	"errors"
	"fmt"

	natsgo "github.com/nats-io/nats.go"
	"github.com/sparetimecoders/messaging/specification/spec"
)

// Header represents a key-value pair attached to a NATS message.
type Header struct {
	Key   string
	Value string
}

// ErrEmptyHeaderKey is returned when a Header is created with an empty key.
var ErrEmptyHeaderKey = errors.New("empty key not allowed")

const headerService = "service"

var reservedHeaderKeys = []string{headerService}

func (h Header) validateKey() error {
	if h.Key == "" {
		return ErrEmptyHeaderKey
	}
	for _, rh := range reservedHeaderKeys {
		if rh == h.Key {
			return fmt.Errorf("reserved key %s used, please change to use another one", rh)
		}
	}
	return nil
}

// toNATSHeaders converts spec.Headers to nats.Header.
func toNATSHeaders(h spec.Headers) natsgo.Header {
	nh := natsgo.Header{}
	for k, v := range h {
		if s, ok := v.(string); ok {
			nh.Set(k, s)
		}
	}
	return nh
}

// fromNATSHeaders converts nats.Header to spec.Headers.
func fromNATSHeaders(nh natsgo.Header) spec.Headers {
	h := make(spec.Headers, len(nh))
	for k := range nh {
		h[k] = nh.Get(k)
	}
	return h
}
