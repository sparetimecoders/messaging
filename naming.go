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

// Package spec defines the naming conventions, topology types, and validation
// logic for the gomessaging framework. It has zero transport dependencies and
// is shared across all transport implementations (AMQP, NATS, etc).
package messaging

import (
	"fmt"
	"strings"
)

// DefaultEventExchangeName is the default exchange name used for event streaming.
const DefaultEventExchangeName = "events"

// TopicExchangeName returns the topic exchange name for the given name.
// Format: <name>.topic.exchange
func TopicExchangeName(name string) string {
	return fmt.Sprintf("%s.%s.exchange", name, string(ExchangeTopic))
}

// ServiceEventQueueName returns the durable event queue name for a service.
// Format: <exchangeName>.queue.<service>
func ServiceEventQueueName(exchangeName, service string) string {
	return fmt.Sprintf("%s.queue.%s", exchangeName, service)
}

// ServiceRequestExchangeName returns the direct exchange name for requests to a service.
// Format: <service>.direct.exchange.request
func ServiceRequestExchangeName(service string) string {
	return fmt.Sprintf("%s.%s.exchange.request", service, string(ExchangeDirect))
}

// ServiceResponseExchangeName returns the headers exchange name for responses from a service.
// Format: <service>.headers.exchange.response
func ServiceResponseExchangeName(service string) string {
	return fmt.Sprintf("%s.%s.exchange.response", service, string(ExchangeHeaders))
}

// ServiceRequestQueueName returns the queue name for requests to a service.
// Format: <service>.direct.exchange.request.queue
func ServiceRequestQueueName(service string) string {
	return fmt.Sprintf("%s.queue", ServiceRequestExchangeName(service))
}

// ServiceResponseQueueName returns the queue name for responses from targetService to serviceName.
// Format: <targetService>.headers.exchange.response.queue.<serviceName>
func ServiceResponseQueueName(targetService, serviceName string) string {
	return fmt.Sprintf("%s.queue.%s", ServiceResponseExchangeName(targetService), serviceName)
}

// NATSStreamName extracts the base stream name from a logical name.
// If the name follows the AMQP convention "<name>.topic.exchange", the prefix is extracted.
// Otherwise the name is returned as-is.
// Examples: "events" → "events", "audit.topic.exchange" → "audit"
func NATSStreamName(name string) string {
	if before, ok := strings.CutSuffix(name, ".topic.exchange"); ok {
		return before
	}
	return name
}

// NATSSubject builds a NATS subject from a stream name and routing key.
// Format: <stream>.<routingKey>
func NATSSubject(stream, routingKey string) string {
	return fmt.Sprintf("%s.%s", stream, routingKey)
}

// TranslateWildcard converts AMQP-style wildcards to NATS-style wildcards.
// AMQP "#" (multi-level) → NATS ">" (multi-level)
// AMQP "*" (single-level) stays "*" in NATS.
func TranslateWildcard(routingKey string) string {
	return strings.ReplaceAll(routingKey, "#", ">")
}
