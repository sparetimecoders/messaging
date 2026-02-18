# Migration Guide: goamqp to gomessaging/amqp

## 1. Overview

The `gomessaging/amqp` module is the successor to `goamqp`. It uses the **same exchange/queue naming conventions** and the **same wire format** (JSON body, AMQP headers), so old and new services can coexist on the same RabbitMQ cluster during a rolling migration.

Key differences at a glance:

- **Two packages** instead of one: `gomessaging/amqp` (transport) + `gomessaging/spec` (types & handlers)
- **Generic handlers** replace `any`-typed handlers with `reflect.Type` dispatch
- **Context-first** throughout: every handler receives `context.Context`
- **Publisher confirms enabled by default** (opt out with `WithoutPublisherConfirms()`)
- **Quorum queues by default** (old module used classic queues)
- **Structured logging** via `log/slog` replaces `func(string)` error logger
- **OTel tracing, CloudEvents headers, Prometheus metrics** available as opt-in features

> **Rolling deployment safety:** The wire format is identical. A `gomessaging/amqp` publisher
> sends JSON with the same routing keys and exchange names as `goamqp`. The only infrastructure
> concern is the queue type change (classic -> quorum) — see [Section 11](#11-infrastructure-quorum-queues).

---

## 2. Breaking Changes Quick Reference

| Area | Old (`goamqp`) | New (`gomessaging/amqp` + `spec`) |
|------|----------------|----------------------------------|
| Import | `github.com/sparetimecoders/goamqp` | `github.com/sparetimecoders/gomessaging/amqp` + `github.com/sparetimecoders/gomessaging/spec` |
| Handler signature | `func(msg any, headers Headers) (any, error)` | `func(ctx context.Context, event spec.ConsumableEvent[T]) error` |
| Consumer setup | `EventStreamConsumer(rk, handler, MyEvent{}, opts...)` | `EventStreamConsumer[MyEvent](rk, handler, opts...)` |
| Options type | `QueueBindingConfigSetup` | `ConsumerOptions` |
| Publisher creation | `NewPublisher()` | `NewPublisher(opts...)` with `WithConfirm`, `WithoutPublisherConfirms` |
| Publishing | `publisher.Publish(msg)` / `publisher.PublishWithContext(ctx, msg)` (routing by type) | `publisher.Publish(ctx, routingKey, msg)` |
| Publisher confirms | Opt-in via `PublishNotify(ch)` | **On by default**; opt out with `WithoutPublisherConfirms()` |
| Type mapping | `WithTypeMapping(rk, MyEvent{})` setup option | Removed; publisher uses explicit routing keys |
| TypeMappingHandler | `conn.TypeMappingHandler(handler)` method | `amqp.TypeMappingHandler(handler, mapper)` package function |
| Logging | `UseLogger(func(string))` | `WithLogger(*slog.Logger)` |
| Message logging | `UseMessageLogger(MessageLogger)` | Removed (use `slog` debug level) |
| Headers type | `goamqp.Headers` | `spec.Headers` (same underlying type) |
| Header struct | `goamqp.Header{Key, Value}` | `amqp.Header{Key, Value}` |
| Routing key access | `headers["routing-key"]` | `event.DeliveryInfo.Key` |
| Queue type | Classic queues | **Quorum queues** |
| Single active consumer | Not set | **Enabled by default** (opt out with `DisableSingleActiveConsumer()`) |
| Close listener | `CloseListener(ch)` → channel-level only | `CloseListener(ch)` → channel + connection level |
| Request-response | `RequestResponseHandler(rk, handler, EventType{})` | `RequestResponseHandler[T, R](rk, handler)` |
| `AmqpChannel` interface | Exported | Unexported (`amqpChannel`, internal only) |
| Errors | `ErrRecoverable`, `ErrIllegalEventType`, `ErrNilLogger`, etc. | Removed (see [Section 10](#10-removed-apis)) |

---

## 3. Import Path Changes

**Before:**
```go
import "github.com/sparetimecoders/goamqp"
```

**After:**
```go
import (
    "github.com/sparetimecoders/gomessaging/amqp"
    "github.com/sparetimecoders/gomessaging/spec"
)
```

The transport-specific types (`Header`, `Publisher`, `Setup`, consumer/publisher setup functions) live in the `amqp` package. Transport-agnostic types (`Headers`, `ConsumableEvent`, `EventHandler`, `Metadata`, `DeliveryInfo`) live in the `spec` package.

---

## 4. Handler Signature

This is the **biggest change** and affects every handler in your codebase.

### Old signature

```go
// goamqp.HandlerFunc
type HandlerFunc func(msg any, headers goamqp.Headers) (response any, err error)
```

### New signature

```go
// spec.EventHandler[T]
type EventHandler[T any] func(ctx context.Context, event spec.ConsumableEvent[T]) error
```

Where `ConsumableEvent[T]` is:

```go
type ConsumableEvent[T any] struct {
    Metadata                     // ID, Timestamp, Source, Type, SpecVersion, etc.
    DeliveryInfo DeliveryInfo    // Destination, Source (exchange), Key (routing key), Headers
    Payload      T               // Your strongly-typed message
}
```

### Migration pattern

**Before:**
```go
func handleOrderCreated(msg any, headers goamqp.Headers) (any, error) {
    order := msg.(*OrderCreated)
    routingKey := headers["routing-key"].(string)
    svc := headers["service"].(string)
    // ... process order ...
    return nil, nil
}
```

**After:**
```go
func handleOrderCreated(ctx context.Context, event spec.ConsumableEvent[OrderCreated]) error {
    order := event.Payload          // already typed as OrderCreated (not a pointer)
    routingKey := event.DeliveryInfo.Key
    svc := event.DeliveryInfo.Headers["service"].(string)
    // ... process order ...
    return nil
}
```

Key differences:
- **`ctx context.Context`** is the first parameter — use it for tracing, cancellation, and deadlines
- **No `any` type assertion** — `event.Payload` is already your concrete type `T`
- **No response return** — standard handlers return only `error`. For request-response, use `RequestResponseEventHandler[T, R]`
- **Routing key** moved from `headers["routing-key"]` to `event.DeliveryInfo.Key`
- **Headers** moved from the second parameter to `event.DeliveryInfo.Headers`
- **CloudEvents metadata** available on `event.Metadata` (ID, Timestamp, Source, Type, etc.)

### Request-response handler

**Before:**
```go
func handlePriceRequest(msg any, headers goamqp.Headers) (any, error) {
    req := msg.(*PriceRequest)
    return &PriceResponse{Price: 42}, nil
}
```

**After:**
```go
// spec.RequestResponseEventHandler[T, R]
func handlePriceRequest(ctx context.Context, event spec.ConsumableEvent[PriceRequest]) (PriceResponse, error) {
    req := event.Payload
    return PriceResponse{Price: 42}, nil
}
```

---

## 5. Consumer Setup Functions

All consumer setup functions lost the `eventType any` parameter and gained generic type parameters.

### EventStreamConsumer

**Before:**
```go
goamqp.EventStreamConsumer("order.created", handleOrder, OrderCreated{}, goamqp.AddQueueNameSuffix("extra"))
```

**After:**
```go
amqp.EventStreamConsumer[OrderCreated]("order.created", handleOrder, amqp.AddQueueNameSuffix("extra"))
```

### TransientEventStreamConsumer

**Before:**
```go
goamqp.TransientEventStreamConsumer("order.created", handleOrder, OrderCreated{})
```

**After:**
```go
amqp.TransientEventStreamConsumer[OrderCreated]("order.created", handleOrder)
```

### StreamConsumer / TransientStreamConsumer

**Before:**
```go
goamqp.StreamConsumer("custom-exchange", "my.key", handler, MyEvent{}, opts...)
goamqp.TransientStreamConsumer("custom-exchange", "my.key", handler, MyEvent{})
```

**After:**
```go
amqp.StreamConsumer[MyEvent]("custom-exchange", "my.key", handler, opts...)
amqp.TransientStreamConsumer[MyEvent]("custom-exchange", "my.key", handler)
```

### ServiceRequestConsumer

**Before:**
```go
goamqp.ServiceRequestConsumer("price.request", handlePrice, PriceRequest{})
```

**After:**
```go
amqp.ServiceRequestConsumer[PriceRequest]("price.request", handlePrice)
```

### ServiceResponseConsumer

**Before:**
```go
goamqp.ServiceResponseConsumer("pricing-service", "price.response", handleResp, PriceResponse{})
```

**After:**
```go
amqp.ServiceResponseConsumer[PriceResponse]("pricing-service", "price.response", handleResp)
```

### RequestResponseHandler

**Before:**
```go
goamqp.RequestResponseHandler("price.request", handlePriceReq, PriceRequest{})
```

**After:**
```go
amqp.RequestResponseHandler[PriceRequest, PriceResponse]("price.request", handlePriceReq)
```

Note: the handler signature also changes — see [Section 4](#4-handler-signature).

### QueueBindingConfigSetup -> ConsumerOptions

The option type was renamed:

| Old | New |
|-----|-----|
| `goamqp.QueueBindingConfigSetup` | `amqp.ConsumerOptions` |
| `goamqp.AddQueueNameSuffix(s)` | `amqp.AddQueueNameSuffix(s)` |

New options available in `ConsumerOptions`:
- `amqp.DisableSingleActiveConsumer()` — opt out of single active consumer
- `amqp.WithDeadLetter(exchange)` — configure dead letter exchange
- `amqp.WithDeadLetterRoutingKey(key)` — set dead letter routing key

---

## 6. Publisher Changes

### Creating a publisher

**Before:**
```go
pub := goamqp.NewPublisher()
```

**After:**
```go
// Default: publisher confirms enabled (blocks until broker acks)
pub := amqp.NewPublisher()

// With explicit confirm channel (you receive confirmations):
confirmCh := make(chan amqp.Confirmation, 1)
pub := amqp.NewPublisher(amqp.WithConfirm(confirmCh))

// Disable confirms for high-throughput fire-and-forget:
pub := amqp.NewPublisher(amqp.WithoutPublisherConfirms())
```

### Publishing messages

The biggest publishing change: **type-based routing is removed**. You must provide an explicit routing key.

**Before:**
```go
// Routing key resolved from type mapping
err := pub.Publish(OrderCreated{ID: "123"})
// or
err := pub.PublishWithContext(ctx, OrderCreated{ID: "123"})
```

**After:**
```go
// Explicit routing key required
err := pub.Publish(ctx, "order.created", OrderCreated{ID: "123"})
```

### Publisher confirms (PublishNotify)

**Before:**
```go
confirmCh := make(chan amqp.Confirmation, 1)

conn.Start(ctx,
    goamqp.PublishNotify(confirmCh),
    goamqp.EventStreamPublisher(pub),
)
```

**After:**
```go
confirmCh := make(chan amqp.Confirmation, 1)
pub := amqp.NewPublisher(amqp.WithConfirm(confirmCh))

conn.Start(ctx,
    amqp.EventStreamPublisher(pub),
)
```

Note: `PublishNotify` setup function is removed. Confirm channels are now configured on the publisher directly. Also, **publisher confirms are enabled by default** — even without `WithConfirm`, `Publish()` waits for broker ack and returns an error on nack.

### Publisher setup functions (unchanged names)

These functions have the same names and usage:

| Function | Notes |
|----------|-------|
| `EventStreamPublisher(pub)` | Unchanged |
| `StreamPublisher(exchange, pub)` | Unchanged |
| `ServicePublisher(targetService, pub)` | No longer returns `ErrServicePublisherAlreadyExist` |
| `QueuePublisher(pub, queueName)` | Unchanged |

---

## 7. TypeMapping & TypeMappingHandler

### WithTypeMapping — removed entirely

**Before:**
```go
conn.Start(ctx,
    goamqp.WithTypeMapping("order.created", OrderCreated{}),
    goamqp.WithTypeMapping("order.shipped", OrderShipped{}),
    goamqp.EventStreamPublisher(pub),
)
// Publisher resolves routing key from Go type:
pub.Publish(OrderCreated{ID: "123"})  // -> routing key "order.created"
```

**After:**
```go
conn.Start(ctx,
    amqp.EventStreamPublisher(pub),
)
// Publisher uses explicit routing key:
pub.Publish(ctx, "order.created", OrderCreated{ID: "123"})
```

There is no equivalent of `WithTypeMapping`. Instead, pass the routing key directly to `Publish()`.

### TypeMappingHandler — method to package function

The `TypeMappingHandler` was previously a method on `*Connection` that used the connection's internal `keyToType` map. It is now a **standalone package function** that takes an explicit `TypeMapper`.

**Before:**
```go
conn.Start(ctx,
    goamqp.WithTypeMapping("order.created", OrderCreated{}),
    goamqp.WithTypeMapping("order.shipped", OrderShipped{}),
    goamqp.EventStreamConsumer("order.*", conn.TypeMappingHandler(handler), json.RawMessage{}),
)

func handler(msg any, headers goamqp.Headers) (any, error) {
    switch m := msg.(type) {
    case *OrderCreated:
        // ...
    case *OrderShipped:
        // ...
    }
    return nil, nil
}
```

**After:**
```go
import "reflect"

typeMap := map[string]reflect.Type{
    "order.created": reflect.TypeFor[*OrderCreated](),
    "order.shipped": reflect.TypeFor[*OrderShipped](),
}

mapper := func(routingKey string) (reflect.Type, bool) {
    t, ok := typeMap[routingKey]
    return t, ok
}

conn.Start(ctx,
    amqp.EventStreamConsumer[any]("order.*",
        amqp.TypeMappingHandler(handler, mapper),
    ),
)

func handler(ctx context.Context, event spec.ConsumableEvent[any]) error {
    switch m := event.Payload.(type) {
    case *OrderCreated:
        // ...
    case *OrderShipped:
        // ...
    }
    return nil
}
```

The `TypeMapper` signature:
```go
type TypeMapper func(routingKey string) (reflect.Type, bool)
```

---

## 8. Logging

### Error logging

**Before:**
```go
goamqp.UseLogger(func(s string) {
    log.Println("AMQP error:", s)
})
```

**After:**
```go
amqp.WithLogger(slog.Default())
// or
amqp.WithLogger(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})))
```

If `WithLogger` is not called, `slog.Default()` is used automatically.

### Message logging

**Before:**
```go
goamqp.UseMessageLogger(goamqp.StdOutMessageLogger())
```

**After:**

Removed. Use slog at debug level instead. The new module logs connection lifecycle events at `Info` level. For message-level debugging, configure your `slog.Logger` at `Debug` level.

---

## 9. Headers

### Headers type

**Before:**
```go
import "github.com/sparetimecoders/goamqp"

var h goamqp.Headers = map[string]any{"foo": "bar"}
v := h.Get("foo")
```

**After:**
```go
import "github.com/sparetimecoders/gomessaging/spec"

var h spec.Headers = map[string]any{"foo": "bar"}
v := h.Get("foo")
```

The underlying type is identical (`map[string]any`). Only the import path changes.

### Header struct

**Before:**
```go
goamqp.Header{Key: "x-custom", Value: "value"}
```

**After:**
```go
amqp.Header{Key: "x-custom", Value: "value"}
```

### Accessing routing key

**Before:**
```go
func handler(msg any, headers goamqp.Headers) (any, error) {
    routingKey := headers["routing-key"].(string)
    // ...
}
```

**After:**
```go
func handler(ctx context.Context, event spec.ConsumableEvent[MyEvent]) error {
    routingKey := event.DeliveryInfo.Key
    source := event.DeliveryInfo.Source       // exchange name
    destination := event.DeliveryInfo.Destination // queue name
    headers := event.DeliveryInfo.Headers     // all AMQP headers
    // ...
}
```

The synthetic `"routing-key"` header injection is removed. The routing key, exchange, and queue are now structured fields on `DeliveryInfo`.

---

## 10. Removed APIs

| Old symbol | Status | Replacement |
|------------|--------|-------------|
| `goamqp.ErrRecoverable` | Removed | Return any error; all errors cause nack+requeue |
| `goamqp.ErrIllegalEventType` | Removed | Generics make illegal types a compile error |
| `goamqp.ErrNilLogger` | Removed | `slog.Default()` used as fallback; nil is not possible |
| `goamqp.ErrServicePublisherAlreadyExist` | Removed | No longer tracked |
| `goamqp.ErrNoRouteForMessageType` | Removed | Explicit routing keys; see `amqp.ErrNoMessageTypeForRouteKey` for TypeMappingHandler |
| `goamqp.MessageLogger` | Removed | Use `slog` at debug level |
| `goamqp.StdOutMessageLogger()` | Removed | Use `slog` at debug level |
| `goamqp.noOpMessageLogger()` | Removed | No-op is the default (just don't set debug logging) |
| `goamqp.AmqpChannel` interface | Removed (unexported) | `amqpChannel` is internal; use `*amqp.Channel` directly if testing |
| `goamqp.QueueBindingConfig` | Removed (unexported) | `consumerConfig` is internal |
| `goamqp.QueueBindingConfigSetup` | Renamed | `amqp.ConsumerOptions` |
| `goamqp.HandlerFunc` | Removed | `spec.EventHandler[T]` |
| `goamqp.WithTypeMapping(rk, type)` | Removed | Pass explicit routing key to `Publish()` |
| `conn.TypeMappingHandler(handler)` | Changed | `amqp.TypeMappingHandler(handler, mapper)` package function |
| `goamqp.UseLogger(func(string))` | Removed | `amqp.WithLogger(*slog.Logger)` |
| `goamqp.UseMessageLogger(MessageLogger)` | Removed | Use `slog` debug level |
| `goamqp.PublishNotify(ch)` | Removed | `amqp.NewPublisher(amqp.WithConfirm(ch))` |
| `goamqp.Must(t, err)` | Removed | Use standard `if err != nil` pattern |
| `publisher.Publish(msg, headers...)` | Removed | `publisher.Publish(ctx, routingKey, msg, headers...)` |
| `publisher.PublishWithContext(ctx, msg, headers...)` | Renamed | `publisher.Publish(ctx, routingKey, msg, headers...)` |
| `goamqp.ErrEmptySuffix` | Kept | `amqp.ErrEmptySuffix` (same value) |
| `goamqp.ErrAlreadyStarted` | Kept | `amqp.ErrAlreadyStarted` (same value) |

### New error values

| Symbol | Description |
|--------|-------------|
| `amqp.ErrNoMessageTypeForRouteKey` | Returned by `TypeMappingHandler` when the `TypeMapper` has no type for the routing key |
| `amqp.ErrEmptyHeaderKey` | Returned when a `Header` has an empty key |
| `spec.ErrParseJSON` | Returned when JSON unmarshaling fails inside `TypeMappingHandler` |

---

## 11. Infrastructure: Quorum Queues

The new module declares **quorum queues** by default (the old module used classic queues). Quorum queues are the RabbitMQ-recommended queue type for data safety.

### What changed

| Property | Old (goamqp) | New (gomessaging/amqp) |
|----------|-------------|----------------------|
| Queue type | Classic | **Quorum** (`x-queue-type: quorum`) |
| Single active consumer | Not set | **Enabled** (`x-single-active-consumer: true`) |
| Queue TTL | `x-expires: 432000000` (5 days) | `x-expires: 432000000` (5 days) |
| Durable | Yes | Yes |

### Why this matters

RabbitMQ does **not** allow changing a queue's type in-place. If a classic queue named `events.topic.exchange.queue.my-service` already exists, the new module's attempt to declare it as quorum will fail with a `PRECONDITION_FAILED` error.

### Migration procedure

**Option A: Delete and re-create (brief downtime per queue)**

1. Stop the old consumer
2. Delete the classic queue (via RabbitMQ management UI or `rabbitmqadmin delete queue name=...`)
3. Deploy the new consumer — it will create the quorum queue on startup
4. Messages published while the queue didn't exist are lost (topic exchange semantics)

**Option B: Blue-green with new queue names**

1. Deploy the new consumer with `AddQueueNameSuffix("v2")` — creates a new quorum queue with a different name
2. Verify it's working
3. Stop the old consumer and delete the classic queue
4. Remove the suffix in a follow-up deploy (or keep it permanently)

### Opting out of single active consumer

If you need multiple consumers processing in parallel from the same queue:

```go
amqp.EventStreamConsumer[OrderCreated]("order.created", handler,
    amqp.DisableSingleActiveConsumer(),
)
```

---

## 12. New Features Available

These are **opt-in** and not required for migration, but available once you're on the new module.

### CloudEvents headers

Published messages automatically include CloudEvents headers in AMQP binary content mode (`cloudEvents:id`, `cloudEvents:type`, `cloudEvents:source`, etc.). No code changes needed — this happens transparently.

For incoming messages, CloudEvents metadata is available on `event.Metadata`:

```go
func handler(ctx context.Context, event spec.ConsumableEvent[MyEvent]) error {
    fmt.Println(event.Metadata.ID)        // CloudEvents id
    fmt.Println(event.Metadata.Source)     // CloudEvents source
    fmt.Println(event.Metadata.Type)       // CloudEvents type
    fmt.Println(event.Metadata.Timestamp)  // CloudEvents time
    return nil
}
```

### Legacy support

When consuming messages from old `goamqp` publishers that don't send CloudEvents headers, enable `WithLegacySupport()` to get synthetic metadata:

```go
conn.Start(ctx,
    amqp.WithLegacySupport(),
    // ... consumers ...
)
```

With this option, incoming messages without CloudEvents headers will have `Metadata` populated with a generated UUID, current timestamp, routing key as type, and exchange as source.

### OTel tracing

```go
conn.Start(ctx,
    amqp.WithTracing(tracerProvider),
    amqp.WithPropagator(propagator),
    // Optional: customize span names
    amqp.WithSpanNameFn(func(info spec.DeliveryInfo) string {
        return fmt.Sprintf("consume %s", info.Key)
    }),
    amqp.WithPublishSpanNameFn(func(exchange, routingKey string) string {
        return fmt.Sprintf("publish %s", routingKey)
    }),
)
```

### Notification & error channels

```go
notifyCh := make(chan spec.Notification, 100)
errorCh := make(chan spec.ErrorNotification, 100)

conn.Start(ctx,
    amqp.WithNotificationChannel(notifyCh),
    amqp.WithErrorChannel(errorCh),
)

// Receive delivery notifications
go func() {
    for n := range notifyCh {
        fmt.Printf("processed %s in %dms\n", n.DeliveryInfo.Key, n.Duration)
    }
}()
```

### Topology export

After `Start()`, inspect the declared topology:

```go
topo := conn.Topology()
// topo.ServiceName, topo.Transport, topo.Endpoints
for _, ep := range topo.Endpoints {
    fmt.Printf("%s %s on %s (queue: %s, key: %s)\n",
        ep.Direction, ep.Pattern, ep.ExchangeName, ep.QueueName, ep.RoutingKey)
}
```

### Dead letter support

```go
amqp.EventStreamConsumer[OrderCreated]("order.created", handler,
    amqp.WithDeadLetter("my-dlx-exchange"),
    amqp.WithDeadLetterRoutingKey("order.created.failed"),
)
```
