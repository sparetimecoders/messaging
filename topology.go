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

// Transport identifies the messaging transport implementation.
type Transport string

const (
	TransportAMQP Transport = "amqp"
	TransportNATS Transport = "nats"
)

// EndpointDirection indicates whether an endpoint publishes or consumes messages.
type EndpointDirection string

const (
	DirectionPublish EndpointDirection = "publish"
	DirectionConsume EndpointDirection = "consume"
)

// ExchangeKind represents the type of an exchange.
type ExchangeKind string

const (
	ExchangeTopic   ExchangeKind = "topic"
	ExchangeDirect  ExchangeKind = "direct"
	ExchangeHeaders ExchangeKind = "headers"
)

// Pattern identifies the communication pattern an endpoint participates in.
type Pattern string

const (
	PatternEventStream     Pattern = "event-stream"
	PatternCustomStream    Pattern = "custom-stream"
	PatternServiceRequest  Pattern = "service-request"
	PatternServiceResponse Pattern = "service-response"
	PatternQueuePublish    Pattern = "queue-publish"
)

// Endpoint describes a single exchange/queue/binding that a service declares.
type Endpoint struct {
	Direction    EndpointDirection `json:"direction"`
	Pattern      Pattern          `json:"pattern"`
	ExchangeName string           `json:"exchangeName"`
	ExchangeKind ExchangeKind     `json:"exchangeKind"`
	QueueName    string           `json:"queueName,omitempty"`
	RoutingKey   string           `json:"routingKey,omitempty"`
	MessageType  string           `json:"messageType,omitempty"`
	Ephemeral    bool             `json:"ephemeral,omitempty"`
}

// Topology describes the full messaging topology declared by a single service.
type Topology struct {
	Transport   Transport  `json:"transport,omitempty"`
	ServiceName string     `json:"serviceName"`
	Endpoints   []Endpoint `json:"endpoints"`
}
