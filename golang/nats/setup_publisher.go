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
	"context"
	"fmt"
	"time"

	"github.com/sparetimecoders/gomessaging/spec"
)

// EventStreamPublisher sets up a publisher for the default "events" stream.
func EventStreamPublisher(publisher *Publisher) Setup {
	return StreamPublisher(defaultEventStreamName, publisher)
}

// StreamPublisher sets up a publisher for a named JetStream stream.
func StreamPublisher(stream string, publisher *Publisher) Setup {
	name := streamName(stream)
	return func(c *Connection) error {
		if !c.collectMode {
			if _, err := c.ensureStream(context.Background(), name); err != nil {
				return fmt.Errorf("failed to ensure stream %s: %w", name, err)
			}
			publisher.publishFn = jsPublishFn(c.js)
		}
		c.log().Info("configured publisher", "stream", name)
		pattern := spec.PatternEventStream
		if stream != defaultEventStreamName {
			pattern = spec.PatternCustomStream
		}
		c.pendingStreams = append(c.pendingStreams, name)
		c.addEndpoint(spec.Endpoint{
			Direction:    spec.DirectionPublish,
			Pattern:      pattern,
			ExchangeName: name,
			ExchangeKind: spec.ExchangeTopic,
		})
		publisher.setup(c, name, c.serviceName, c.tracer(), c.propagator, c.publishSpanNameFn)
		return nil
	}
}

// ServicePublisher sets up a publisher that sends requests to targetService
// using NATS Core request-reply.
func ServicePublisher(targetService string, publisher *Publisher) Setup {
	return func(c *Connection) error {
		if !c.collectMode {
			timeout := c.requestTimeout
			if timeout == 0 {
				timeout = 30 * time.Second
			}
			publisher.publishFn = coreRequestFn(c.nc, timeout)
		}
		publisher.subjectFn = func(_, routingKey string) string {
			return serviceRequestSubject(targetService, routingKey)
		}
		c.log().Info("configured service publisher", "targetService", targetService)
		c.addEndpoint(spec.Endpoint{
			Direction:    spec.DirectionPublish,
			Pattern:      spec.PatternServiceRequest,
			ExchangeName: targetService,
			ExchangeKind: spec.ExchangeDirect,
		})
		publisher.setup(c, targetService, c.serviceName, c.tracer(), c.propagator, c.publishSpanNameFn)
		return nil
	}
}
