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

package amqp

import (
	"errors"
	"fmt"

	"github.com/sparetimecoders/gomessaging/spec"
)

// Header represents a key-value pair attached to an AMQP message.
// Value types are constrained by amqp.Table; see the AMQP 0-9-1 spec for allowed types.
type Header struct {
	Key   string
	Value any
}

// ErrEmptyHeaderKey is returned when a Header is created with an empty key.
var ErrEmptyHeaderKey = errors.New("empty key not allowed")

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

func validateHeaders(h spec.Headers) error {
	for k, v := range h {
		hdr := Header{k, v}
		if err := hdr.validateKey(); err != nil {
			return err
		}
	}
	return nil
}

var reservedHeaderKeys = []string{headerService}
