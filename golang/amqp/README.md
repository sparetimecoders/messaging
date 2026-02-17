# gomessaging/amqp

Go AMQP transport for RabbitMQ. Provides type-safe event consumption and publishing with automatic CloudEvents headers, OpenTelemetry tracing, and Prometheus metrics.

```
go get github.com/sparetimecoders/gomessaging/amqp
```

## Quick Start

```go
package main

import (
    "context"
    "log/slog"

    "github.com/sparetimecoders/gomessaging/amqp"
    "github.com/sparetimecoders/gomessaging/spec"
)

type OrderCreated struct {
    OrderID  string  `json:"orderId"`
    Customer string  `json:"customer"`
    Total    float64 `json:"total"`
}

func main() {
    ctx := context.Background()

    conn, err := amqp.NewFromURL("order-service", "amqp://guest:guest@localhost:5672/")
    if err != nil {
        panic(err)
    }

    publisher := amqp.NewPublisher()

    err = conn.Start(ctx,
        amqp.WithLogger(slog.Default()),
        amqp.EventStreamPublisher(publisher),
        amqp.EventStreamConsumer("Order.Created",
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

Create a connection with `NewFromURL`, configure it with setup functions, then call `Start`:

```go
conn, err := amqp.NewFromURL("my-service", "amqp://guest:guest@localhost:5672/")
err = conn.Start(ctx, setup1, setup2, ...)
defer conn.Close()
```

`Start` connects to the broker, declares exchanges and queues, and begins consuming. The connection string follows the [AMQP URI format](https://www.rabbitmq.com/docs/uri-spec).

## Configuration Options

| Setup Function | Description | Default |
|---------------|-------------|---------|
| `WithLogger(logger)` | Structured logging | `slog.Default()` |
| `WithTracing(provider)` | OpenTelemetry TracerProvider | `otel.GetTracerProvider()` |
| `WithPropagator(propagator)` | OTel TextMapPropagator | `otel.GetTextMapPropagator()` |
| `WithPrefetchLimit(n)` | Message prefetch count | 20 |
| `WithSpanNameFn(fn)` | Custom consumer span names | `"{destination}#{routingKey}"` |
| `WithPublishSpanNameFn(fn)` | Custom publish span names | `"publish {routingKey}"` |
| `WithNotificationChannel(ch)` | Receive success notifications | (none) |
| `WithErrorChannel(ch)` | Receive error notifications | (none) |
| `CloseListener(ch)` | Notified on channel close | (none) |
| `PublishNotify(ch)` | Receive publish confirmations | (none) |

## Consumer Setup Functions

### Event Stream (Topic Exchange)

Subscribe to events on the default `events` exchange or a named exchange:

```go
// Durable consumer on default "events" exchange
amqp.EventStreamConsumer[OrderCreated]("Order.Created", handler)

// Transient consumer (auto-deletes on disconnect)
amqp.TransientEventStreamConsumer[OrderCreated]("Order.*", handler)

// Durable consumer on named exchange
amqp.StreamConsumer[AuditEntry]("audit", "order.#", handler)

// Transient consumer on named exchange
amqp.TransientStreamConsumer[AuditEntry]("audit", "order.#", handler)
```

### Service Request-Response

```go
// Consume incoming requests
amqp.ServiceRequestConsumer[EmailRequest]("email.send", handler)

// Consume responses from a target service
amqp.ServiceResponseConsumer[EmailResponse]("email-service", "email.send", handler)

// Combined request handler that auto-publishes responses
amqp.RequestResponseHandler[EmailRequest, EmailResponse]("email.send", handler)
```

### Consumer Options

```go
// Append suffix to queue name (for multiple consumers of the same routing key)
amqp.EventStreamConsumer[T]("Order.Created", handler,
    amqp.AddQueueNameSuffix("retry"),
)

// Disable single-active-consumer (enabled by default)
amqp.EventStreamConsumer[T]("Order.Created", handler,
    amqp.DisableSingleActiveConsumer(),
)
```

## Publisher Setup Functions

```go
publisher := amqp.NewPublisher()

// Publish to default "events" exchange
amqp.EventStreamPublisher(publisher)

// Publish to named exchange
amqp.StreamPublisher("audit", publisher)

// Publish service requests to target service
amqp.ServicePublisher("email-service", publisher)

// Publish directly to a named queue
amqp.QueuePublisher(publisher, "my-queue")
```

### Publishing Messages

```go
// Basic publish
err := publisher.Publish(ctx, "Order.Created", orderEvent)

// Publish with custom headers
err := publisher.Publish(ctx, "Order.Created", orderEvent,
    amqp.Header{Key: "ce-correlationid", Value: "abc-123"},
)
```

CloudEvents headers (`ce-specversion`, `ce-type`, `ce-source`, `ce-id`, `ce-time`, `ce-datacontenttype`) are set automatically. Custom headers with `ce-` prefix override the defaults.

## Wildcard Routing with Type Mapping

When consuming multiple event types with a wildcard routing key, use `TypeMappingHandler` to dispatch to the correct type:

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

handler := amqp.TypeMappingHandler(
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

amqp.TransientEventStreamConsumer("Order.#", handler)
```

## Error Handling

Handler return values control message acknowledgment:

| Handler Returns | Behavior |
|----------------|----------|
| `nil` | Message ACKed |
| `spec.ErrParseJSON` | Message NACKed without requeue (dropped) |
| Any other error | Message NACKed with requeue (retried) |

## Metrics

Initialize Prometheus metrics once at startup:

```go
if err := amqp.InitMetrics(prometheus.DefaultRegisterer); err != nil {
    log.Fatal(err)
}
```

**Consumer metrics** (labels: `queue`, `routing_key`):
- `amqp_events_received` - total events received
- `amqp_events_ack` - events acknowledged
- `amqp_events_nack` - events rejected
- `amqp_events_without_handler` - events with no handler
- `amqp_events_not_parsable` - JSON parse failures
- `amqp_events_processed_duration` - processing time (ms histogram)

**Publisher metrics** (labels: `exchange`, `routing_key`):
- `amqp_events_publish_succeed` - successful publishes
- `amqp_events_publish_failed` - failed publishes
- `amqp_events_publish_duration` - publish time (ms histogram)

## Topology Export

Collect the declared topology without connecting to a broker:

```go
topology, err := amqp.CollectTopology("my-service",
    amqp.EventStreamPublisher(publisher),
    amqp.EventStreamConsumer[OrderCreated]("Order.Created", handler),
)
// topology can be serialized to JSON for validation/visualization
```

## Queue Defaults

| Setting | Value |
|---------|-------|
| Queue type | Quorum |
| Single active consumer | Enabled |
| Queue TTL | 5 days (432,000,000 ms) |
| Content type | `application/json` |
| Delivery mode | Persistent |
| AMQP heartbeat | 10 seconds |

## Best Practices

### Queue Configuration

- **Quorum queues** are used by default. This is the recommended queue type for RabbitMQ 4.0+ with replicated, durable queues and built-in poison message tracking.
- **Single-active-consumer** is enabled by default. Only one consumer per queue processes messages at a time. Use `DisableSingleActiveConsumer()` for competing consumers.
- **Queue TTL** is 5 days for durable queues (auto-deleted if no consumers). Transient queues use 1 second TTL.
- **Dead Letter Exchange**: The library does NOT configure a DLX by default. In RabbitMQ 4.0+, the default delivery-limit is 20 — messages exceeding this are silently dropped unless you configure a DLX via RabbitMQ policy. Set up DLX policies on your broker for production use.

### Prefetch / QoS

- Default prefetch is 20. Override with `WithPrefetchLimit(n)`.
- Set to 1 for strict round-robin across multiple consumers.
- Higher values (20-50) improve throughput when processing time is consistent.

### Publisher Confirms

Publisher confirms are available via `PublishNotify(confirmCh)` but not enabled by default. For guaranteed delivery with quorum queues, always enable publisher confirms — without them, messages can be silently lost if the broker crashes before replication completes.

```go
confirmCh := make(chan amqp.Confirmation, 100)
conn.Start(ctx,
    amqp.PublishNotify(confirmCh),
    // ... other setups
)
```

### Connection Management

The library uses a **fail-fast** approach: if the AMQP connection drops, the process should restart (designed for Kubernetes pod restarts). Use `CloseListener(errCh)` to detect connection loss and trigger graceful shutdown. The heartbeat interval is 10 seconds.

### Idempotency

Both transports provide at-least-once delivery — handlers MUST be idempotent. Use `ce-id` + `ce-source` as a deduplication key if needed. The `ce-id` is a UUID generated per message.

## Migrating from goamqp

This module replaces the legacy `goamqp` library. The wire format is **backward compatible in both directions** — services using old `goamqp` and new `gomessaging/amqp` can coexist on the same AMQP bus during a rolling migration.

### Wire Format Changes

The new publisher adds CloudEvents headers (`ce-specversion`, `ce-type`, `ce-source`, `ce-id`, `ce-time`, `ce-datacontenttype`) and optionally OTel trace headers (`traceparent`, `tracestate`) to every message. The message body, content type, and delivery mode are unchanged.

### Mixed Deployment Behavior

| Scenario | Behavior |
|----------|----------|
| **New publisher → Old consumer** | Safe. Old consumers ignore unknown headers. |
| **Old publisher → New consumer** | Safe. Messages are consumed and acked normally. A debug-level log notes the missing CloudEvents headers. `event.Metadata` fields (`ID`, `Source`, `Timestamp`, etc.) will be zero-valued. |
| **New publisher → New consumer** | Full CloudEvents metadata and OTel trace propagation. |

### Edge Cases

- **Empty Metadata on legacy messages**: Handlers that rely on `event.Metadata.ID`, `event.Metadata.Source`, or `event.Metadata.Timestamp` will receive empty strings / zero time for messages from old publishers. Guard against this if your handler logic depends on these fields.

- **Partial CloudEvents headers**: If a message has some but not all required CE attributes (e.g., a custom publisher that only sets `ce-id`), the consumer logs a warning with the specific missing attributes. The message is still processed normally.

- **OTel trace context**: When a message arrives without `traceparent` (from an old publisher or when tracing is disabled), the consumer starts a new root span. There is no trace context propagation for these messages.

- **`routing-key` header removed**: Old `goamqp` injected a synthetic `"routing-key"` entry into the headers map at consume time. This is no longer done. The routing key is available via `event.DeliveryInfo.Key`.

- **`ce-id` on every message**: The new publisher always sets `ce-id` (a UUID). If a downstream system relied on the absence of a message ID, this is a behavioral change.

## Helpers

```go
// Panic on error (useful for package-level initialization)
var conn = amqp.Must(amqp.NewFromURL("svc", "amqp://..."))
```
