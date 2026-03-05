package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/sparetimecoders/messaging/golang/amqp"
	"github.com/sparetimecoders/messaging/golang/nats"
	"github.com/sparetimecoders/messaging/specification/spec"
)

// amqpSetups returns the AMQP setup functions for the go-demo service.
func amqpSetups(pub *amqp.Publisher, broadcast func(SSEEvent)) []amqp.Setup {
	return []amqp.Setup{
		amqp.EventStreamPublisher(pub),
		amqp.EventStreamConsumer("Order.Created", orderHandler("amqp", broadcast)),
		amqp.EventStreamConsumer("Ping", pingHandler("amqp", broadcast)),
		amqp.RequestResponseHandler("email.send", emailHandler("amqp", broadcast)),
	}
}

// natsSetups returns the NATS setup functions for the go-demo service.
func natsSetups(pub *nats.Publisher, broadcast func(SSEEvent)) []nats.Setup {
	return []nats.Setup{
		nats.EventStreamPublisher(pub),
		nats.EventStreamConsumer("Order.Created", orderHandler("nats", broadcast)),
		nats.EventStreamConsumer("Ping", pingHandler("nats", broadcast)),
		nats.RequestResponseHandler("email.send", emailHandler("nats", broadcast)),
	}
}

func orderHandler(transport string, broadcast func(SSEEvent)) spec.EventHandler[OrderCreated] {
	return func(ctx context.Context, event spec.ConsumableEvent[OrderCreated]) error {
		slog.Info("received Order.Created",
			"transport", transport,
			"source", event.Payload.Source,
			"orderId", event.Payload.OrderID,
		)
		broadcast(SSEEvent{
			Type:       "received",
			Transport:  transport,
			Source:     event.Payload.Source,
			RoutingKey: event.DeliveryInfo.Key,
			Payload:    event.Payload,
			TraceID:    event.Metadata.ID,
		})
		return nil
	}
}

func pingHandler(transport string, broadcast func(SSEEvent)) spec.EventHandler[PingMessage] {
	return func(ctx context.Context, event spec.ConsumableEvent[PingMessage]) error {
		slog.Info("received Ping",
			"transport", transport,
			"source", event.Payload.Source,
			"message", event.Payload.Message,
		)
		broadcast(SSEEvent{
			Type:       "received",
			Transport:  transport,
			Source:     event.Payload.Source,
			RoutingKey: event.DeliveryInfo.Key,
			Payload:    event.Payload,
			TraceID:    event.Metadata.ID,
		})
		return nil
	}
}

func emailHandler(transport string, broadcast func(SSEEvent)) spec.RequestResponseEventHandler[EmailRequest, EmailResponse] {
	return func(ctx context.Context, event spec.ConsumableEvent[EmailRequest]) (EmailResponse, error) {
		slog.Info("handling email request",
			"transport", transport,
			"to", event.Payload.To,
			"subject", event.Payload.Subject,
		)
		broadcast(SSEEvent{
			Type:       "received",
			Transport:  transport,
			Source:     event.Source,
			RoutingKey: event.DeliveryInfo.Key,
			Payload:    event.Payload,
			TraceID:    event.Metadata.ID,
		})
		resp := EmailResponse{
			MessageID: fmt.Sprintf("msg-%s", uuid.New().String()[:8]),
			Status:    "sent",
		}
		return resp, nil
	}
}
