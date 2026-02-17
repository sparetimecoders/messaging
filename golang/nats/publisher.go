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
	"encoding/json"
	"time"

	"github.com/google/uuid"
	natsgo "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/sparetimecoders/gomessaging/spec"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

const contentType = "application/json"

// Publisher is used to send messages to NATS.
type Publisher struct {
	conn           *Connection
	stream         string
	serviceName    string
	defaultHeaders []Header
	tracer         trace.Tracer
	propagator     propagation.TextMapPropagator
	spanNameFn     func(stream, routingKey string) string

	// publishFn abstracts the publish operation for JetStream vs Core.
	publishFn func(ctx context.Context, subject string, msg *natsgo.Msg) error
}

// NewPublisher creates an uninitialized Publisher. Pass it to a publisher Setup
// function (e.g. EventStreamPublisher, ServicePublisher) which wires it to the
// connection. After Start completes, call Publish to send messages.
func NewPublisher() *Publisher {
	return &Publisher{}
}

// Publish sends msg as a JSON-encoded NATS message with the given routing key.
func (p *Publisher) Publish(ctx context.Context, routingKey string, msg any, headers ...Header) error {
	natsHeaders := natsgo.Header{}
	for _, h := range p.defaultHeaders {
		natsHeaders.Set(h.Key, h.Value)
	}
	for _, h := range headers {
		if err := h.validateKey(); err != nil {
			return err
		}
		natsHeaders.Set(h.Key, h.Value)
	}

	return p.publishMessage(ctx, routingKey, msg, natsHeaders)
}

func (p *Publisher) publishMessage(ctx context.Context, routingKey string, msg any, headers natsgo.Header) error {
	tracer := p.tracer
	if tracer == nil {
		tracer = tracerFromProvider(nil)
	}
	spanNameFn := p.spanNameFn
	if spanNameFn == nil {
		spanNameFn = defaultPublishSpanName
	}

	subject := subjectName(p.stream, routingKey)
	spanAttrs := publishSpanAttributes(p.stream, subject, p.serviceName)
	ctx, span := tracer.Start(ctx, spanNameFn(p.stream, routingKey),
		trace.WithSpanKind(trace.SpanKindProducer),
		trace.WithAttributes(spanAttrs...),
	)
	defer span.End()

	jsonBytes, err := json.Marshal(msg)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	setDefault := func(key, value string) {
		if headers.Get(key) == "" {
			headers.Set(key, value)
		}
	}
	setDefault(spec.CESpecVersion, spec.CESpecVersionValue)
	setDefault(spec.CEType, routingKey)
	setDefault(spec.CESource, p.serviceName)
	setDefault(spec.CEDataContentType, contentType)
	setDefault(spec.CETime, time.Now().UTC().Format(time.RFC3339))

	messageID := uuid.New().String()
	if existing := headers.Get(spec.CEID); existing != "" {
		messageID = existing
	}
	headers.Set(spec.CEID, messageID)

	span.SetAttributes(
		attrMessagingMessageID.String(messageID),
		attrMessagingBodySize.Int(len(jsonBytes)),
	)

	headers = injectToHeaders(ctx, headers, p.propagator)

	natsMsg := &natsgo.Msg{
		Subject: subject,
		Data:    jsonBytes,
		Header:  headers,
	}

	startTime := time.Now()
	err = p.publishFn(ctx, subject, natsMsg)
	elapsed := time.Since(startTime).Milliseconds()
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		eventPublishFailed(p.stream, routingKey, elapsed)
		return err
	}
	span.SetStatus(codes.Ok, "")
	eventPublishSucceed(p.stream, routingKey, elapsed)
	return nil
}

func (p *Publisher) setup(conn *Connection, stream, serviceName string, tracer trace.Tracer, prop propagation.TextMapPropagator, spanNameFn func(string, string) string) {
	p.conn = conn
	p.stream = stream
	p.serviceName = serviceName
	p.tracer = tracer
	p.propagator = prop
	p.spanNameFn = spanNameFn
	p.defaultHeaders = append(p.defaultHeaders, Header{Key: headerService, Value: serviceName})
}

// jsPublishFn publishes via JetStream.
func jsPublishFn(js jetstream.JetStream) func(ctx context.Context, subject string, msg *natsgo.Msg) error {
	return func(ctx context.Context, subject string, msg *natsgo.Msg) error {
		_, err := js.PublishMsg(ctx, msg)
		return err
	}
}

// coreRequestFn publishes via NATS Core request-reply.
func coreRequestFn(nc *natsgo.Conn, timeout time.Duration) func(ctx context.Context, subject string, msg *natsgo.Msg) error {
	return func(ctx context.Context, subject string, msg *natsgo.Msg) error {
		_, err := nc.RequestMsg(msg, timeout)
		return err
	}
}
