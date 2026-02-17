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
	"fmt"

	"github.com/sparetimecoders/gomessaging/spec"
)

const defaultEventStreamName = "events"

// streamName returns the NATS stream name for a logical exchange name.
func streamName(name string) string {
	return spec.NATSStreamName(name)
}

// subjectName builds the full NATS subject: <stream>.<routingKey>.
func subjectName(stream, routingKey string) string {
	return spec.NATSSubject(stream, routingKey)
}

// filterSubject builds the NATS filter subject with wildcard translation.
func filterSubject(stream, routingKey string) string {
	return spec.NATSSubject(stream, spec.TranslateWildcard(routingKey))
}

// streamSubjects returns the subject pattern a stream should capture.
// Format: <stream>.>
func streamSubjects(stream string) string {
	return fmt.Sprintf("%s.>", stream)
}

// serviceRequestSubject returns the NATS subject for a service request.
// Format: <service>.request.<routingKey>
func serviceRequestSubject(service, routingKey string) string {
	return fmt.Sprintf("%s.request.%s", service, routingKey)
}

// consumerName returns the durable consumer name for a service on a stream.
func consumerName(service string) string {
	return service
}

// consumerNameWithSuffix returns a durable consumer name with a suffix.
func consumerNameWithSuffix(service, suffix string) string {
	return fmt.Sprintf("%s-%s", service, suffix)
}
