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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/sparetimecoders/gomessaging/spec"
	"github.com/stretchr/testify/require"
)

func Test_Invalid_Payload(t *testing.T) {
	err := newWrappedHandler(func(ctx context.Context, event spec.ConsumableEvent[string]) error {
		return nil
	})(context.TODO(), unmarshalEvent{Payload: []byte(`{"a":}`)})
	require.ErrorIs(t, err, spec.ErrParseJSON)
	require.ErrorContains(t, err, "invalid character '}' looking for beginning of value")
}

func Test_Consume(t *testing.T) {
	consumer := queueConsumer{
		queue:    "aQueue",
		handlers: routingKeyHandler{},
	}
	channel := &MockAmqpChannel{consumeFn: func(queue, consumerName string, autoAck, exclusive, noLocal, noWait bool, args amqp.Table) (<-chan amqp.Delivery, error) {
		require.Equal(t, consumer.queue, queue)
		require.Equal(t, "", consumerName)
		require.False(t, autoAck)
		require.False(t, exclusive)
		require.False(t, noLocal)
		require.False(t, noWait)
		require.Nil(t, args)
		deliveries := make(chan amqp.Delivery, 1)
		deliveries <- amqp.Delivery{
			MessageId: "MESSAGE_ID",
		}
		close(deliveries)
		return deliveries, nil
	}}

	deliveries, err := consumer.consume(channel, nil, nil)
	require.NoError(t, err)
	delivery := <-deliveries
	require.Equal(t, "MESSAGE_ID", delivery.MessageId)
}

func Test_Consume_Failing(t *testing.T) {
	consumer := queueConsumer{
		queue:    "aQueue",
		handlers: routingKeyHandler{},
	}
	channel := &MockAmqpChannel{consumeFn: func(queue, consumerName string, autoAck, exclusive, noLocal, noWait bool, args amqp.Table) (<-chan amqp.Delivery, error) {
		return nil, fmt.Errorf("failed")
	}}

	_, err := consumer.consume(channel, nil, nil)
	require.EqualError(t, err, "failed")
}

func Test_ConsumerLoop(t *testing.T) {
	acker := MockAcknowledger{
		Acks:    make(chan Ack, 2),
		Nacks:   make(chan Nack, 1),
		Rejects: make(chan Reject, 1),
	}
	handler := newWrappedHandler(func(ctx context.Context, msg spec.ConsumableEvent[Message]) error {
		if msg.Payload.Ok {
			return nil
		}
		return errors.New("failed")
	})

	consumer := queueConsumer{
		handlers: routingKeyHandler{},
		spanNameFn: func(info spec.DeliveryInfo) string {
			return "span"
		},
	}
	consumer.handlers.add("key1", handler)
	consumer.handlers.add("key2", handler)

	queueDeliveries := make(chan amqp.Delivery, 4)

	queueDeliveries <- delivery(acker, "key1", true)
	queueDeliveries <- delivery(acker, "key2", true)
	queueDeliveries <- delivery(acker, "key2", false)
	queueDeliveries <- delivery(acker, "missing", true)
	close(queueDeliveries)

	consumer.loop(queueDeliveries)

	require.Len(t, acker.Rejects, 1)
	require.Len(t, acker.Nacks, 1)
	require.Len(t, acker.Acks, 2)
}

func Test_HandleDelivery(t *testing.T) {
	tests := []struct {
		name            string
		error           error
		numberOfAcks    int
		numberOfNacks   int
		numberOfRejects int
	}{
		{
			name:         "ok",
			numberOfAcks: 1,
		},
		{
			name:          "invalid JSON",
			error:         spec.ErrParseJSON,
			numberOfNacks: 1,
		},
		{
			name:            "no match for routingkey",
			error:           ErrNoMessageTypeForRouteKey,
			numberOfRejects: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errorNotifications := make(chan spec.ErrorNotification, 1)
			notifications := make(chan spec.Notification, 1)
			consumer := queueConsumer{
				errorCh:        errorNotifications,
				notificationCh: notifications,
				spanNameFn: func(info spec.DeliveryInfo) string {
					return "span"
				},
			}
			deliveryInfo := spec.DeliveryInfo{
				Key: "key",
			}
			acker := MockAcknowledger{
				Acks:    make(chan Ack, 1),
				Nacks:   make(chan Nack, 1),
				Rejects: make(chan Reject, 1),
			}
			handler := func(ctx context.Context, event unmarshalEvent) error {
				return tt.error
			}
			d := delivery(acker, "routingKey", true)
			consumer.handleDelivery(handler, d, deliveryInfo)
			if tt.error != nil {
				notification := <-errorNotifications
				require.EqualError(t, notification.Error, tt.error.Error())
			} else {
				notification := <-notifications
				require.Contains(t, notification.DeliveryInfo.Key, "key")
			}

			require.Len(t, acker.Acks, tt.numberOfAcks)
			require.Len(t, acker.Nacks, tt.numberOfNacks)
			require.Len(t, acker.Rejects, tt.numberOfRejects)
		})
	}
}

func Test_CloudEvents_Metadata_Extraction(t *testing.T) {
	var captured spec.ConsumableEvent[Message]
	handler := newWrappedHandler(func(ctx context.Context, event spec.ConsumableEvent[Message]) error {
		captured = event
		return nil
	})

	acker := MockAcknowledger{
		Acks:    make(chan Ack, 1),
		Nacks:   make(chan Nack, 1),
		Rejects: make(chan Reject, 1),
	}
	consumer := queueConsumer{
		handlers: routingKeyHandler{},
		spanNameFn: func(info spec.DeliveryInfo) string {
			return "span"
		},
	}
	consumer.handlers.add("Order.Created", handler)

	body, _ := json.Marshal(Message{Ok: true})
	d := amqp.Delivery{
		Body:       body,
		RoutingKey: "Order.Created",
		Exchange:   "events.topic.exchange",
		Headers: amqp.Table{
			spec.CESpecVersion: spec.CESpecVersionValue,
			spec.CEType:        "Order.Created",
			spec.CESource:      "orders-svc",
			spec.CEID:          "evt-123",
			spec.CETime:        "2025-06-15T10:30:00Z",
		},
		Acknowledger: &acker,
	}

	deliveries := make(chan amqp.Delivery, 1)
	deliveries <- d
	close(deliveries)
	consumer.loop(deliveries)

	require.Len(t, acker.Acks, 1)
	require.Equal(t, "evt-123", captured.ID)
	require.Equal(t, "orders-svc", captured.Source)
	require.Equal(t, "Order.Created", captured.Type)
	require.Equal(t, spec.CESpecVersionValue, captured.SpecVersion)
	require.False(t, captured.Timestamp.IsZero(), "Timestamp should be extracted from ce-time")
}

func Test_CloudEvents_Validation_Warning(t *testing.T) {
	handler := newWrappedHandler(func(ctx context.Context, event spec.ConsumableEvent[Message]) error {
		return nil
	})

	acker := MockAcknowledger{
		Acks:    make(chan Ack, 1),
		Nacks:   make(chan Nack, 1),
		Rejects: make(chan Reject, 1),
	}
	consumer := queueConsumer{
		handlers: routingKeyHandler{},
		spanNameFn: func(info spec.DeliveryInfo) string {
			return "span"
		},
	}
	consumer.handlers.add("Order.Created", handler)

	body, _ := json.Marshal(Message{Ok: true})
	d := amqp.Delivery{
		Body:       body,
		RoutingKey: "Order.Created",
		Exchange:   "events.topic.exchange",
		Headers:    amqp.Table{},
		Acknowledger: &acker,
	}

	deliveries := make(chan amqp.Delivery, 1)
	deliveries <- d
	close(deliveries)
	consumer.loop(deliveries)

	require.Len(t, acker.Acks, 1, "message should still be acked despite missing CE headers")
}

func Test_CloudEvents_CorrelationID_Extraction(t *testing.T) {
	var captured spec.ConsumableEvent[Message]
	handler := newWrappedHandler(func(ctx context.Context, event spec.ConsumableEvent[Message]) error {
		captured = event
		return nil
	})

	acker := MockAcknowledger{
		Acks:    make(chan Ack, 1),
		Nacks:   make(chan Nack, 1),
		Rejects: make(chan Reject, 1),
	}
	consumer := queueConsumer{
		handlers: routingKeyHandler{},
		spanNameFn: func(info spec.DeliveryInfo) string {
			return "span"
		},
	}
	consumer.handlers.add("Order.Created", handler)

	body, _ := json.Marshal(Message{Ok: true})
	d := amqp.Delivery{
		Body:       body,
		RoutingKey: "Order.Created",
		Exchange:   "events.topic.exchange",
		Headers: amqp.Table{
			spec.CESpecVersion:  spec.CESpecVersionValue,
			spec.CEType:         "Order.Created",
			spec.CESource:       "orders-svc",
			spec.CEID:           "evt-456",
			spec.CETime:         "2025-06-15T10:30:00Z",
			spec.CECorrelationID: "corr-abc-123",
		},
		Acknowledger: &acker,
	}

	deliveries := make(chan amqp.Delivery, 1)
	deliveries <- d
	close(deliveries)
	consumer.loop(deliveries)

	require.Len(t, acker.Acks, 1)
	require.Equal(t, "corr-abc-123", captured.CorrelationID)
}

func Test_WrappedHandler_AnyType(t *testing.T) {
	err := newWrappedHandler(func(ctx context.Context, event spec.ConsumableEvent[any]) error {
		raw, ok := event.Payload.(json.RawMessage)
		require.True(t, ok)
		require.Equal(t, `{"a":"b"}`, string(raw))
		return nil
	})(context.TODO(), unmarshalEvent{Payload: []byte(`{"a":"b"}`)})
	require.NoError(t, err)
}

func Test_WrappedHandler_JsonRawMessage(t *testing.T) {
	err := newWrappedHandler(func(ctx context.Context, event spec.ConsumableEvent[json.RawMessage]) error {
		require.Equal(t, `{"a":"b"}`, string(event.Payload))
		return nil
	})(context.TODO(), unmarshalEvent{Payload: []byte(`{"a":"b"}`)})
	require.NoError(t, err)
}

func Test_WrappedHandler_Success(t *testing.T) {
	err := newWrappedHandler(func(ctx context.Context, event spec.ConsumableEvent[TestMessage]) error {
		require.Equal(t, "hello", event.Payload.Msg)
		return nil
	})(context.TODO(), unmarshalEvent{Payload: []byte(`{"Msg":"hello","Success":true}`)})
	require.NoError(t, err)
}

func Test_TypeMappingHandler_NilMapper(t *testing.T) {
	handler := TypeMappingHandler(func(ctx context.Context, event spec.ConsumableEvent[any]) error {
		return nil
	}, nil)
	err := handler(context.TODO(), spec.ConsumableEvent[any]{
		DeliveryInfo: spec.DeliveryInfo{Key: "key"},
		Payload:      json.RawMessage(`{}`),
	})
	require.EqualError(t, err, "TypeMapper is nil")
}

func Test_TypeMappingHandler_NoTypeForKey(t *testing.T) {
	handler := TypeMappingHandler(func(ctx context.Context, event spec.ConsumableEvent[any]) error {
		return nil
	}, func(routingKey string) (reflect.Type, bool) {
		return nil, false
	})
	err := handler(context.TODO(), spec.ConsumableEvent[any]{
		DeliveryInfo: spec.DeliveryInfo{Key: "unknown"},
		Payload:      json.RawMessage(`{}`),
	})
	require.ErrorIs(t, err, ErrNoMessageTypeForRouteKey)
}

func Test_TypeMappingHandler_ParseError(t *testing.T) {
	handler := TypeMappingHandler(func(ctx context.Context, event spec.ConsumableEvent[any]) error {
		return nil
	}, func(routingKey string) (reflect.Type, bool) {
		return reflect.TypeOf(&TestMessage{}), true
	})
	err := handler(context.TODO(), spec.ConsumableEvent[any]{
		DeliveryInfo: spec.DeliveryInfo{Key: "known"},
		Payload:      json.RawMessage(`{invalid`),
	})
	require.ErrorIs(t, err, spec.ErrParseJSON)
}

func Test_TypeMappingHandler_Success(t *testing.T) {
	handler := TypeMappingHandler(func(ctx context.Context, event spec.ConsumableEvent[any]) error {
		msg, ok := event.Payload.(*TestMessage)
		require.True(t, ok)
		require.Equal(t, "hello", msg.Msg)
		return nil
	}, func(routingKey string) (reflect.Type, bool) {
		if routingKey == "known" {
			return reflect.TypeOf(&TestMessage{}), true
		}
		return nil, false
	})
	err := handler(context.TODO(), spec.ConsumableEvent[any]{
		DeliveryInfo: spec.DeliveryInfo{Key: "known"},
		Payload:      json.RawMessage(`{"Msg":"hello","Success":true}`),
	})
	require.NoError(t, err)
}

func Test_LegacyMessage_NoHeaders_DebugLogOnly(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	var captured spec.ConsumableEvent[Message]
	handler := newWrappedHandler(func(ctx context.Context, event spec.ConsumableEvent[Message]) error {
		captured = event
		return nil
	})

	acker := MockAcknowledger{
		Acks:    make(chan Ack, 1),
		Nacks:   make(chan Nack, 1),
		Rejects: make(chan Reject, 1),
	}
	consumer := queueConsumer{
		handlers: routingKeyHandler{},
		logger:   logger,
		spanNameFn: func(info spec.DeliveryInfo) string {
			return "span"
		},
	}
	consumer.handlers.add("Order.Created", handler)

	body, _ := json.Marshal(Message{Ok: true})
	// Legacy message: only "service" header, no CE headers — like old goamqp publisher
	d := amqp.Delivery{
		Body:       body,
		RoutingKey: "Order.Created",
		Exchange:   "events.topic.exchange",
		Headers: amqp.Table{
			"service": "legacy-svc",
		},
		Acknowledger: &acker,
	}

	deliveries := make(chan amqp.Delivery, 1)
	deliveries <- d
	close(deliveries)
	consumer.loop(deliveries)

	require.Len(t, acker.Acks, 1, "legacy message should be acked")
	require.Contains(t, buf.String(), "received legacy message without CloudEvents headers")
	require.NotContains(t, buf.String(), "incoming message has invalid CloudEvents headers",
		"should not produce CE validation warnings for legacy messages")

	// Metadata fields should be zero-valued for legacy messages
	require.Empty(t, captured.ID)
	require.Empty(t, captured.Source)
	require.Empty(t, captured.Type)
	require.True(t, captured.Timestamp.IsZero())
	// But payload should still be deserialized
	require.True(t, captured.Payload.Ok)
}

func Test_PartialCEHeaders_StillWarns(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))

	handler := newWrappedHandler(func(ctx context.Context, event spec.ConsumableEvent[Message]) error {
		return nil
	})

	acker := MockAcknowledger{
		Acks:    make(chan Ack, 1),
		Nacks:   make(chan Nack, 1),
		Rejects: make(chan Reject, 1),
	}
	consumer := queueConsumer{
		handlers: routingKeyHandler{},
		logger:   logger,
		spanNameFn: func(info spec.DeliveryInfo) string {
			return "span"
		},
	}
	consumer.handlers.add("Order.Created", handler)

	body, _ := json.Marshal(Message{Ok: true})
	// Partially malformed CE message: has some CE headers but missing others
	d := amqp.Delivery{
		Body:       body,
		RoutingKey: "Order.Created",
		Exchange:   "events.topic.exchange",
		Headers: amqp.Table{
			spec.CESpecVersion: spec.CESpecVersionValue,
			spec.CEID:          "evt-123",
			// Missing: ce-type, ce-source, ce-time
		},
		Acknowledger: &acker,
	}

	deliveries := make(chan amqp.Delivery, 1)
	deliveries <- d
	close(deliveries)
	consumer.loop(deliveries)

	require.Len(t, acker.Acks, 1, "message should still be acked despite partial CE headers")
	require.Contains(t, buf.String(), "incoming message has invalid CloudEvents headers",
		"should warn about incomplete CE headers")
}

func Test_NewPublisher_OldConsumer_ExtraHeadersSafe(t *testing.T) {
	// Verifies that messages with CE headers can be consumed by handlers
	// that don't care about Metadata — the payload is still correctly deserialized.
	var captured spec.ConsumableEvent[Message]
	handler := newWrappedHandler(func(ctx context.Context, event spec.ConsumableEvent[Message]) error {
		captured = event
		return nil
	})

	acker := MockAcknowledger{
		Acks:    make(chan Ack, 1),
		Nacks:   make(chan Nack, 1),
		Rejects: make(chan Reject, 1),
	}
	consumer := queueConsumer{
		handlers: routingKeyHandler{},
		spanNameFn: func(info spec.DeliveryInfo) string {
			return "span"
		},
	}
	consumer.handlers.add("Order.Created", handler)

	body, _ := json.Marshal(Message{Ok: true})
	d := amqp.Delivery{
		Body:       body,
		RoutingKey: "Order.Created",
		Exchange:   "events.topic.exchange",
		Headers: amqp.Table{
			"service":          "new-svc",
			spec.CESpecVersion: spec.CESpecVersionValue,
			spec.CEType:        "Order.Created",
			spec.CESource:      "new-svc",
			spec.CEID:          "evt-789",
			spec.CETime:        "2025-06-15T10:30:00Z",
			"traceparent":      "00-abc123-def456-01",
		},
		Acknowledger: &acker,
	}

	deliveries := make(chan amqp.Delivery, 1)
	deliveries <- d
	close(deliveries)
	consumer.loop(deliveries)

	require.Len(t, acker.Acks, 1)
	require.Equal(t, "evt-789", captured.ID)
	require.Equal(t, "new-svc", captured.Source)
	require.True(t, captured.Payload.Ok)
}

func Test_LegacyMessage_WithLegacySupport_EnrichedMetadata(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	var captured spec.ConsumableEvent[Message]
	handler := newWrappedHandler(func(ctx context.Context, event spec.ConsumableEvent[Message]) error {
		captured = event
		return nil
	})

	acker := MockAcknowledger{
		Acks:    make(chan Ack, 1),
		Nacks:   make(chan Nack, 1),
		Rejects: make(chan Reject, 1),
	}
	consumer := queueConsumer{
		handlers:      routingKeyHandler{},
		logger:        logger,
		legacySupport: true,
		spanNameFn: func(info spec.DeliveryInfo) string {
			return "span"
		},
	}
	consumer.handlers.add("Order.Created", handler)

	body, _ := json.Marshal(Message{Ok: true})
	d := amqp.Delivery{
		Body:       body,
		RoutingKey: "Order.Created",
		Exchange:   "events.topic.exchange",
		Headers: amqp.Table{
			"service": "legacy-svc",
		},
		Acknowledger: &acker,
	}

	deliveries := make(chan amqp.Delivery, 1)
	deliveries <- d
	close(deliveries)
	consumer.loop(deliveries)

	require.Len(t, acker.Acks, 1, "legacy message should be acked")
	require.NotEmpty(t, captured.ID, "ID should be a generated UUID")
	require.False(t, captured.Timestamp.IsZero(), "Timestamp should be set")
	require.Equal(t, "Order.Created", captured.Type)
	require.Equal(t, "events.topic.exchange", captured.Source)
	require.Equal(t, spec.CESpecVersionValue, captured.SpecVersion)
	require.True(t, captured.HasCloudEvents())
	require.True(t, captured.Payload.Ok)
	require.Contains(t, buf.String(), "enriched legacy message with synthetic metadata")
}

func Test_LegacyMessage_WithoutLegacySupport_StillZeroMetadata(t *testing.T) {
	var captured spec.ConsumableEvent[Message]
	handler := newWrappedHandler(func(ctx context.Context, event spec.ConsumableEvent[Message]) error {
		captured = event
		return nil
	})

	acker := MockAcknowledger{
		Acks:    make(chan Ack, 1),
		Nacks:   make(chan Nack, 1),
		Rejects: make(chan Reject, 1),
	}
	consumer := queueConsumer{
		handlers:      routingKeyHandler{},
		legacySupport: false,
		spanNameFn: func(info spec.DeliveryInfo) string {
			return "span"
		},
	}
	consumer.handlers.add("Order.Created", handler)

	body, _ := json.Marshal(Message{Ok: true})
	d := amqp.Delivery{
		Body:       body,
		RoutingKey: "Order.Created",
		Exchange:   "events.topic.exchange",
		Headers: amqp.Table{
			"service": "legacy-svc",
		},
		Acknowledger: &acker,
	}

	deliveries := make(chan amqp.Delivery, 1)
	deliveries <- d
	close(deliveries)
	consumer.loop(deliveries)

	require.Len(t, acker.Acks, 1)
	require.Empty(t, captured.ID, "should remain zero-valued without legacy support")
	require.True(t, captured.Timestamp.IsZero())
	require.False(t, captured.HasCloudEvents())
}

func Test_CEMessage_WithLegacySupport_NotEnriched(t *testing.T) {
	var captured spec.ConsumableEvent[Message]
	handler := newWrappedHandler(func(ctx context.Context, event spec.ConsumableEvent[Message]) error {
		captured = event
		return nil
	})

	acker := MockAcknowledger{
		Acks:    make(chan Ack, 1),
		Nacks:   make(chan Nack, 1),
		Rejects: make(chan Reject, 1),
	}
	consumer := queueConsumer{
		handlers:      routingKeyHandler{},
		legacySupport: true,
		spanNameFn: func(info spec.DeliveryInfo) string {
			return "span"
		},
	}
	consumer.handlers.add("Order.Created", handler)

	body, _ := json.Marshal(Message{Ok: true})
	d := amqp.Delivery{
		Body:       body,
		RoutingKey: "Order.Created",
		Exchange:   "events.topic.exchange",
		Headers: amqp.Table{
			spec.CESpecVersion: spec.CESpecVersionValue,
			spec.CEType:        "Order.Created",
			spec.CESource:      "real-svc",
			spec.CEID:          "real-id-123",
			spec.CETime:        "2025-06-15T10:30:00Z",
		},
		Acknowledger: &acker,
	}

	deliveries := make(chan amqp.Delivery, 1)
	deliveries <- d
	close(deliveries)
	consumer.loop(deliveries)

	require.Len(t, acker.Acks, 1)
	require.Equal(t, "real-id-123", captured.ID, "should use real CE ID, not synthetic")
	require.Equal(t, "real-svc", captured.Source, "should use real CE source")
}

func Test_ConsumerLoop_LogsOnChannelClose(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelError}))
	consumer := queueConsumer{
		queue:    "test-queue",
		handlers: routingKeyHandler{},
		logger:   logger,
		spanNameFn: func(info spec.DeliveryInfo) string {
			return "span"
		},
	}

	deliveries := make(chan amqp.Delivery)
	close(deliveries)

	consumer.loop(deliveries)

	require.Contains(t, buf.String(), "consumer loop exited")
	require.Contains(t, buf.String(), "test-queue")
}

func Test_CloudEvents_AMQPPrefix_Normalization(t *testing.T) {
	var captured spec.ConsumableEvent[Message]
	handler := newWrappedHandler(func(ctx context.Context, event spec.ConsumableEvent[Message]) error {
		captured = event
		return nil
	})

	acker := MockAcknowledger{
		Acks:    make(chan Ack, 1),
		Nacks:   make(chan Nack, 1),
		Rejects: make(chan Reject, 1),
	}
	consumer := queueConsumer{
		handlers: routingKeyHandler{},
		spanNameFn: func(info spec.DeliveryInfo) string {
			return "span"
		},
	}
	consumer.handlers.add("Order.Created", handler)

	body, _ := json.Marshal(Message{Ok: true})
	d := amqp.Delivery{
		Body:       body,
		RoutingKey: "Order.Created",
		Exchange:   "events.topic.exchange",
		Headers: amqp.Table{
			"cloudEvents:specversion":    spec.CESpecVersionValue,
			"cloudEvents:type":           "Order.Created",
			"cloudEvents:source":         "orders-svc",
			"cloudEvents:id":             "evt-amqp-123",
			"cloudEvents:time":           "2025-06-15T10:30:00Z",
			"cloudEvents:correlationid":  "corr-amqp-456",
		},
		Acknowledger: &acker,
	}

	deliveries := make(chan amqp.Delivery, 1)
	deliveries <- d
	close(deliveries)
	consumer.loop(deliveries)

	require.Len(t, acker.Acks, 1)
	require.Equal(t, "evt-amqp-123", captured.ID)
	require.Equal(t, "orders-svc", captured.Source)
	require.Equal(t, "Order.Created", captured.Type)
	require.Equal(t, spec.CESpecVersionValue, captured.SpecVersion)
	require.Equal(t, "corr-amqp-456", captured.CorrelationID)
	require.False(t, captured.Timestamp.IsZero(), "Timestamp should be extracted from cloudEvents:time")
}

func Test_CloudEvents_JMSPrefix_Normalization(t *testing.T) {
	var captured spec.ConsumableEvent[Message]
	handler := newWrappedHandler(func(ctx context.Context, event spec.ConsumableEvent[Message]) error {
		captured = event
		return nil
	})

	acker := MockAcknowledger{
		Acks:    make(chan Ack, 1),
		Nacks:   make(chan Nack, 1),
		Rejects: make(chan Reject, 1),
	}
	consumer := queueConsumer{
		handlers: routingKeyHandler{},
		spanNameFn: func(info spec.DeliveryInfo) string {
			return "span"
		},
	}
	consumer.handlers.add("Order.Created", handler)

	body, _ := json.Marshal(Message{Ok: true})
	d := amqp.Delivery{
		Body:       body,
		RoutingKey: "Order.Created",
		Exchange:   "events.topic.exchange",
		Headers: amqp.Table{
			"cloudEvents_specversion": spec.CESpecVersionValue,
			"cloudEvents_type":        "Order.Created",
			"cloudEvents_source":      "orders-svc",
			"cloudEvents_id":          "evt-jms-789",
			"cloudEvents_time":        "2025-06-15T10:30:00Z",
		},
		Acknowledger: &acker,
	}

	deliveries := make(chan amqp.Delivery, 1)
	deliveries <- d
	close(deliveries)
	consumer.loop(deliveries)

	require.Len(t, acker.Acks, 1)
	require.Equal(t, "evt-jms-789", captured.ID)
	require.Equal(t, "orders-svc", captured.Source)
	require.Equal(t, "Order.Created", captured.Type)
	require.Equal(t, spec.CESpecVersionValue, captured.SpecVersion)
	require.False(t, captured.Timestamp.IsZero(), "Timestamp should be extracted from cloudEvents_time")
}

func delivery(acker MockAcknowledger, routingKey string, success bool) amqp.Delivery {
	body, _ := json.Marshal(Message{success})

	return amqp.Delivery{
		Body:         body,
		RoutingKey:   routingKey,
		Acknowledger: &acker,
	}
}
