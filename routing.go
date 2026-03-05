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
	"regexp"
	"strings"
)

// MatchRoutingKey returns true if routingKey matches the binding pattern.
// Supports AMQP/NATS wildcard syntax: * matches one word, # matches zero or more.
func MatchRoutingKey(pattern, routingKey string) bool {
	b, err := regexp.MatchString(routingKeyToRegex(pattern), routingKey)
	if err != nil {
		return false
	}
	return b
}

// RoutingKeyOverlaps returns true if two binding patterns could match the same routing key.
func RoutingKeyOverlaps(p1, p2 string) bool {
	if p1 == p2 {
		return true
	} else if MatchRoutingKey(p1, p2) {
		return true
	} else if MatchRoutingKey(p2, p1) {
		return true
	}
	return false
}

// routingKeyToRegex converts an AMQP/NATS binding pattern to a regular expression.
// For example:
//
//	user.* => user\.[^.]*
//	user.# => user\..*
func routingKeyToRegex(s string) string {
	replace := strings.Replace(strings.Replace(strings.Replace(s, ".", "\\.", -1), "*", "[^.]*", -1), "#", ".*", -1)
	return fmt.Sprintf("^%s$", replace)
}
