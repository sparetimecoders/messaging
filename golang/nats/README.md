# gomessaging/nats

Go NATS transport with JetStream for durable event streaming and Core NATS for request-reply. Provides type-safe event consumption and publishing with automatic CloudEvents headers, OpenTelemetry tracing, and Prometheus metrics.

```
go get github.com/sparetimecoders/gomessaging/nats
```

## Quick Start

```go
package main

import (
    "context"
    "log/slog"

    "github.com/sparetimecoders/gomessaging/nats"
    "github.com/sparetimecoders/gomessaging/spec"
)

type OrderCreated struct {
    OrderID  string  `json:"orderId"`
    Customer string  `json:"customer"`
    Total    float64 `json:"total"`
}

func main() {
    ctx := context.Background()

    conn, err := nats.NewConnection("order-service", "nats://localhost:4222")
    if err != nil {
        panic(err)
    }

    publisher := nats.NewPublisher()

    err = conn.Start(ctx,
        nats.WithLogger(slog.Default()),
        nats.EventStreamPublisher(publisher),
        nats.EventStreamConsumer("Order.Created",
            func(ctx context.Context, event spec.ConsumableEvent[OrderCreated]) error {
                slog.Info("received order", "id", event.Payload.OrderID)
                return nil
            },
        ),
    )
    if err != nil {
        panic(err)
    }
    defer conn.Close()

    // Publish an event
    err = publisher.Publish(ctx, "Order.Created", OrderCreated{
        OrderID: "ORD-001", Customer: "Alice", Total: 99.95,
    })
    if err != nil {
        panic(err)
    }
}
```

## Connection

```go
conn, err := nats.NewConnection("my-service", "nats://localhost:4222",
    natsgo.MaxReconnects(-1),  // optional nats.go options
)
err = conn.Start(ctx, setup1, setup2, ...)
defer conn.Close()
```

The second argument is a NATS URL. Additional arguments are passed as [nats.go connection options](https://pkg.go.dev/github.com/nats-io/nats.go#Option).

## Configuration Options

| Setup Function | Description | Default |
|---------------|-------------|---------|
| `WithLogger(logger)` | Structured logging | `slog.Default()` |
| `WithTracing(provider)` | OpenTelemetry TracerProvider | `otel.GetTracerProvider()` |
| `WithPropagator(propagator)` | OTel TextMapPropagator | `otel.GetTextMapPropagator()` |
| `WithSpanNameFn(fn)` | Custom consumer span names | `"{destination}#{routingKey}"` |
| `WithPublishSpanNameFn(fn)` | Custom publish span names | `"publish {routingKey}"` |
| `WithNotificationChannel(ch)` | Receive success notifications | (none) |
| `WithErrorChannel(ch)` | Receive error notifications | (none) |

## Transport Model

NATS uses two underlying mechanisms:

| gomessaging Pattern | NATS Mechanism | Subject Format |
|-------------------|----------------|---------------|
| Event stream / custom stream | **JetStream** (durable streams) | `{stream}.{routingKey}` |
| Service request | **Core NATS** (request-reply) | `{service}.request.{routingKey}` |
| Service response | **Core NATS** (automatic reply) | (handled by NATS reply mechanism) |

JetStream provides at-least-once delivery with message acknowledgment. Core NATS provides fire-and-forget request-reply with a 30-second timeout.

## Consumer Setup Functions

### Event Stream (JetStream)

Subscribe to events on the default `events` stream or a named stream:

```go
// Durable consumer on default "events" stream
nats.EventStreamConsumer[OrderCreated]("Order.Created", handler)

// Transient consumer (ephemeral, deleted on disconnect)
nats.TransientEventStreamConsumer[OrderCreated]("Order.*", handler)

// Durable consumer on named stream
nats.StreamConsumer[AuditEntry]("audit", "order.#", handler)

// Transient consumer on named stream
nats.TransientStreamConsumer[AuditEntry]("audit", "order.#", handler)
```

### Service Request-Response (Core NATS)

```go
// Consume incoming requests
nats.ServiceRequestConsumer[EmailRequest]("email.send", handler)

// Consume responses from a target service (topology declaration only)
nats.ServiceResponseConsumer[EmailResponse]("email-service", "email.send", handler)

// Combined: handle request and auto-send response via NATS reply
nats.RequestResponseHandler[EmailRequest, EmailResponse]("email.send", handler)
```

### Consumer Options

```go
// Append suffix to durable consumer name
nats.EventStreamConsumer[T]("Order.Created", handler,
    nats.AddConsumerNameSuffix("retry"),
)
```

## Publisher Setup Functions

```go
publisher := nats.NewPublisher()

// Publish to default "events" stream (JetStream)
nats.EventStreamPublisher(publisher)

// Publish to named stream (JetStream)
nats.StreamPublisher("audit", publisher)

// Publish service requests (Core NATS request-reply, 30s timeout)
nats.ServicePublisher("email-service", publisher)
```

### Publishing Messages

```go
// Basic publish
err := publisher.Publish(ctx, "Order.Created", orderEvent)

// Publish with custom headers
err := publisher.Publish(ctx, "Order.Created", orderEvent,
    nats.Header{Key: "ce-correlationid", Value: "abc-123"},
)
```

CloudEvents headers are set automatically on publish.

## Wildcard Routing

AMQP-style wildcards are translated to NATS equivalents:

| AMQP | NATS | Matches |
|------|------|---------|
| `*` | `*` | Single token |
| `#` | `>` | Zero or more tokens |

The `spec.TranslateWildcard()` function handles this conversion. In consumer setup, use AMQP-style patterns:

```go
nats.EventStreamConsumer[T]("Order.#", handler)  // matches Order.Created, Order.Line.Added, etc.
nats.EventStreamConsumer[T]("Order.*", handler)   // matches Order.Created, Order.Updated only
```

## Type Mapping for Wildcard Consumers

```go
typeMapper := func(routingKey string) (reflect.Type, bool) {
    switch routingKey {
    case "Order.Created":
        return reflect.TypeFor[*OrderCreated](), true
    case "Order.Updated":
        return reflect.TypeFor[*OrderUpdated](), true
    default:
        return nil, false
    }
}

handler := nats.TypeMappingHandler(
    func(ctx context.Context, event spec.ConsumableEvent[any]) error {
        switch v := event.Payload.(type) {
        case *OrderCreated:
            // handle created
        case *OrderUpdated:
            // handle updated
        }
        return nil
    },
    typeMapper,
)

nats.EventStreamConsumer("Order.#", handler)
```

## Error Handling

### JetStream Consumers

| Handler Returns | Behavior |
|----------------|----------|
| `nil` | Message Ack (acknowledged) |
| `spec.ErrParseJSON` or type mapping error | Message Term (permanently rejected) |
| Any other error | Message Nak (negative ack, auto-retry) |

### Core NATS (Service Request)

| Handler Returns | Behavior |
|----------------|----------|
| `nil` | Response sent via NATS reply |
| Any error | JSON error response `{"error": "..."}` sent, then error returned |

## Metrics

Initialize Prometheus metrics once at startup:

```go
if err := nats.InitMetrics(prometheus.DefaultRegisterer); err != nil {
    log.Fatal(err)
}
```

**Consumer metrics** (labels: `consumer`, `routing_key`):
- `nats_events_received` - total events received
- `nats_events_ack` - events acknowledged
- `nats_events_nak` - events negatively acknowledged
- `nats_events_without_handler` - events with no handler
- `nats_events_not_parsable` - JSON parse failures
- `nats_events_processed_duration` - processing time (ms histogram)

**Publisher metrics** (labels: `stream`, `subject`):
- `nats_events_publish_succeed` - successful publishes
- `nats_events_publish_failed` - failed publishes
- `nats_events_publish_duration` - publish time (ms histogram)

## Topology Export

Collect the declared topology without connecting to a broker:

```go
topology, err := nats.CollectTopology("my-service",
    nats.EventStreamPublisher(publisher),
    nats.EventStreamConsumer[OrderCreated]("Order.Created", handler),
)
// topology can be serialized to JSON for validation/visualization
```

## NATS Naming

The NATS transport uses simplified names compared to AMQP:

| AMQP Resource | NATS Equivalent |
|--------------|-----------------|
| `events.topic.exchange` | `events` (stream) |
| `events.topic.exchange.queue.orders` | `orders` (consumer) |
| `events.Order.Created` | `events.Order.Created` (subject) |
| `email-svc.direct.exchange.request` | `email-svc.request.email.send` (subject) |

The `spec.NATSStreamName()` function strips `.topic.exchange` suffixes so you can use the same logical names across transports.

## Best Practices

### JetStream vs Core NATS

- **JetStream** is for event streaming: durable, persistent, at-least-once delivery. Use `EventStreamConsumer` / `StreamConsumer`.
- **Core NATS** is for request-reply: synchronous, fire-and-forget. Use `ServiceRequestConsumer` / `RequestResponseHandler`.
- Don't mix them: JetStream for events that need durability, Core for RPC-style calls.

### Stream Configuration

Streams use FileStorage by default (durable). The library creates minimal stream configs. For production, configure retention limits on your NATS server to prevent unbounded growth:

- `max_age`: How long messages are kept (e.g., 7 days)
- `max_bytes`: Maximum stream size
- `num_replicas`: For clustered environments

### Consumer Durability

- `EventStreamConsumer` / `StreamConsumer` creates **durable** consumers (survive restarts, resume from last position)
- `TransientEventStreamConsumer` / `TransientStreamConsumer` creates **ephemeral** consumers (deleted on disconnect)
- Use durable for production workloads, transient for development/debugging

### Retry Behavior

The library does not configure MaxDeliver or BackOff on JetStream consumers. A continuously failing handler will create a hot retry loop. Configure these on your NATS consumers directly if needed.

### Connection Management

- Uses `nc.Drain()` for graceful shutdown (processes in-flight messages before closing)
- The NATS client has built-in reconnection, configurable via `nats.Option` parameters passed to `NewConnection`

### Idempotency

Both transports provide at-least-once delivery — handlers MUST be idempotent. Use `ce-id` + `ce-source` as a deduplication key if needed. The `ce-id` is a UUID generated per message.
