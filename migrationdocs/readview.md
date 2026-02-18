# Readview Migration Guide

> Service-specific migration guide for `gitlab.com/unboundsoftware/eventsourced/readview`.
> See [MIGRATION.md](./MIGRATION.md) for general concepts and the full API mapping.

## Overview

The readview library is a **consumer-only** AMQP user. It processes events from an event-sourced system and maintains read-optimized PostgreSQL views. Its AMQP surface is small and concentrated in two files:

| File | goamqp usage |
|------|-------------|
| `handlers.go` | `Connection` interface, `TypeMappingHandler`, `HandlerFunc`, `Headers`, `ErrRecoverable` |
| `handlers_test.go` | `MockConnection`, `goamqp.HandlerFunc`, `goamqp.ErrRecoverable` assertions |

The library does **not** publish messages, declare exchanges, or manage connections directly — it only provides handler functions that callers wire into their own `goamqp.Connection.Start()` setup.

## Module dependency change

```diff
// go.mod
- github.com/sparetimecoders/goamqp v0.3.3
+ github.com/sparetimecoders/gomessaging/amqp v0.x.x
+ github.com/sparetimecoders/gomessaging/spec v0.x.x
```

## `handlers.go` Migration

### 1. Connection interface removal

**Old:** readview defines its own `Connection` interface wrapping the `TypeMappingHandler` method on `goamqp.Connection`:

```go
// OLD — handlers.go
type Connection interface {
    TypeMappingHandler(handler goamqp.HandlerFunc) goamqp.HandlerFunc
}
```

**New:** `TypeMappingHandler` is now a **package-level function** in `gomessaging/amqp`, not a method on Connection. The readview `Connection` interface is no longer needed.

```go
// NEW — package-level function, no interface required
amqp.TypeMappingHandler(handler spec.EventHandler[any], routingKeyToType amqp.TypeMapper) spec.EventHandler[any]
```

The `Connection` interface and the `conn` parameter on `RegularMappingHandler` / `ExternalMappingHandler` should be removed entirely.

### 2. Handler signature change

The fundamental handler type changes from a two-return value function receiving untyped `any` + `Headers` to a context-aware single-error function receiving a typed `ConsumableEvent`:

```go
// OLD
type HandlerFunc func(msg any, headers goamqp.Headers) (response any, err error)

// NEW
type EventHandler[T any] func(ctx context.Context, event spec.ConsumableEvent[T]) error
```

Key differences:
- **Context**: Handlers now receive `context.Context` with tracing/propagation already set up.
- **ConsumableEvent**: Wraps `Metadata` (CloudEvents), `DeliveryInfo` (routing key, exchange, queue, headers), and `Payload`.
- **No response return**: The `(response any, err error)` return is replaced by just `error`. Readview handlers never used the response value meaningfully (always returned `nil` response on success), so this is a clean removal.
- **Headers access**: Instead of `goamqp.Headers` parameter, access headers via `event.DeliveryInfo.Headers` (type `spec.Headers`).

### 3. `ErrRecoverable` replacement

**Old:** `goamqp.ErrRecoverable` was a sentinel error. When a handler returned an error wrapping `ErrRecoverable`, goamqp would nack+requeue the message *without logging* it as an error:

```go
// OLD — handlers.go
if err != nil && (errors.Is(err, ErrStatusRowLocked) || errors.Is(err, ErrNotReadyToProcessEvents)) {
    return response, fmt.Errorf("%w: %v", goamqp.ErrRecoverable, err)
}
```

**New:** `gomessaging/amqp` has a simpler error model — there is no `ErrRecoverable` sentinel:

| Handler returns | Behavior |
|----------------|----------|
| `nil` | Message ACKed |
| `spec.ErrParseJSON` | Message NACKed **without** requeue (dropped) |
| Any other error | Message NACKed **with** requeue (retried) |

Since readview's `ErrStatusRowLocked` and `ErrNotReadyToProcessEvents` should trigger a requeue, just returning the original error achieves the same behavior — any non-`ErrParseJSON` error is automatically nacked with requeue. The `ErrRecoverable` wrapping is no longer needed.

### 4. `RegularMappingHandler` — before/after

**Old:**
```go
func (r *Readview) RegularMappingHandler(conn Connection, name string, messageHandler MessageHandler) goamqp.HandlerFunc {
    return conn.TypeMappingHandler(func(msg any, headers goamqp.Headers) (response any, err error) {
        if _, ok := msg.(eventsourced.Event); !ok {
            r.logger.Warn(fmt.Sprintf("Got non event for readview '%s', %s", name, reflect.TypeOf(msg).String()))
            return nil, nil
        }
        if _, ok := msg.(eventsourced.ExternalSource); ok {
            return nil, nil
        }
        response, err = messageHandler(msg)
        if err != nil && (errors.Is(err, ErrStatusRowLocked) || errors.Is(err, ErrNotReadyToProcessEvents)) {
            return response, fmt.Errorf("%w: %v", goamqp.ErrRecoverable, err)
        }
        return response, err
    })
}
```

**New:**
```go
func (r *Readview) RegularMappingHandler(name string, messageHandler MessageHandler) spec.EventHandler[any] {
    return func(ctx context.Context, event spec.ConsumableEvent[any]) error {
        msg := event.Payload
        if _, ok := msg.(eventsourced.Event); !ok {
            r.logger.Warn(fmt.Sprintf("Got non event for readview '%s', %s", name, reflect.TypeOf(msg).String()))
            return nil
        }
        if _, ok := msg.(eventsourced.ExternalSource); ok {
            return nil
        }
        _, err := messageHandler(msg)
        if err != nil {
            return err // any error → nack with requeue (replaces ErrRecoverable wrapping)
        }
        return nil
    }
}
```

Changes:
- `conn Connection` parameter removed — `TypeMappingHandler` wrapping moves to the caller's setup code.
- Returns `spec.EventHandler[any]` instead of `goamqp.HandlerFunc`.
- `headers goamqp.Headers` parameter gone — headers available via `event.DeliveryInfo.Headers` if needed.
- `ErrRecoverable` wrapping removed — returning the error directly achieves nack+requeue.
- Response value discarded (readview never used it).

### 5. `ExternalMappingHandler` — before/after

**Old:**
```go
func (r *Readview) ExternalMappingHandler(conn Connection, messageHandler MessageHandler) goamqp.HandlerFunc {
    return conn.TypeMappingHandler(func(msg any, headers goamqp.Headers) (response any, err error) {
        response, err = messageHandler(msg)
        if err != nil && (errors.Is(err, ErrStatusRowLocked) || errors.Is(err, ErrNotReadyToProcessEvents)) {
            return response, fmt.Errorf("%w: %v", goamqp.ErrRecoverable, err)
        }
        return response, err
    })
}
```

**New:**
```go
func (r *Readview) ExternalMappingHandler(messageHandler MessageHandler) spec.EventHandler[any] {
    return func(ctx context.Context, event spec.ConsumableEvent[any]) error {
        _, err := messageHandler(event.Payload)
        if err != nil {
            return err // any error → nack with requeue
        }
        return nil
    }
}
```

### 6. Caller-side wiring change

The readview library returns handler functions. The **caller** (e.g., a service's `main.go`) wires them into the AMQP connection. The caller must now wrap the handler with `amqp.TypeMappingHandler`:

**Old (caller):**
```go
conn.Start(ctx,
    goamqp.WithTypeMapping("Order.Created", Order.Created{}),
    goamqp.WithTypeMapping("Order.Updated", Order.Updated{}),
    goamqp.EventStreamConsumer("Order.#",
        readview.RegularMappingHandler(conn, "orders", ordersHandler), // conn.TypeMappingHandler called inside
        &json.RawMessage{},
    ),
)
```

**New (caller):**
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

conn.Start(ctx,
    amqp.EventStreamConsumer("Order.#",
        amqp.TypeMappingHandler(
            readview.RegularMappingHandler("orders", ordersHandler), // no conn param
            typeMapper,
        ),
    ),
)
```

Key differences:
- `WithTypeMapping` calls replaced by a `TypeMapper` function.
- `TypeMappingHandler` wrapping moves from inside readview to the caller.
- No `eventType` parameter on `EventStreamConsumer` — generics handle it.

## `handlers_test.go` Migration

### MockConnection removal

**Old:**
```go
type MockConnection struct{}

func (m MockConnection) TypeMappingHandler(handler goamqp.HandlerFunc) goamqp.HandlerFunc {
    return handler // pass-through
}

var _ Connection = &MockConnection{}
```

**New:** The `MockConnection` and `Connection` interface are removed entirely. Tests call the handler directly since `TypeMappingHandler` wrapping is no longer readview's responsibility:

```go
// No mock needed — call handler directly
handler := r.RegularMappingHandler("some-readview", tt.args.messageHandler)
err := handler(context.Background(), spec.ConsumableEvent[any]{Payload: tt.args.event})
```

### ErrRecoverable assertions

**Old:**
```go
wantErr: func(t assert.TestingT, err error, i ...interface{}) bool {
    return assert.ErrorIs(t, err, goamqp.ErrRecoverable)
},
```

**New:** Assert against the original error directly since `ErrRecoverable` wrapping is removed:

```go
wantErr: func(t assert.TestingT, err error, i ...interface{}) bool {
    return assert.ErrorIs(t, err, ErrStatusRowLocked)
},
```

### Response value assertions

Old tests check `(response, err)` from the handler. New tests only check `err`:

```go
// OLD
response, err := handler(tt.args.event, nil)
assert.Equal(t, tt.want, response, ...)

// NEW
err := handler(context.Background(), spec.ConsumableEvent[any]{Payload: tt.args.event})
// no response to assert
```

## `go.mod` Migration

```diff
require (
-   github.com/sparetimecoders/goamqp v0.3.3
+   github.com/sparetimecoders/gomessaging/amqp v0.x.x
+   github.com/sparetimecoders/gomessaging/spec v0.x.x
)
```

The `github.com/pkg/errors` indirect dependency (pulled by goamqp) can be removed if no other dependency requires it.

## Migration Checklist

- [ ] Update `go.mod`: replace `goamqp` with `gomessaging/amqp` + `gomessaging/spec`
- [ ] **handlers.go**:
  - [ ] Remove `Connection` interface
  - [ ] Remove `conn Connection` parameter from `RegularMappingHandler`
  - [ ] Remove `conn Connection` parameter from `ExternalMappingHandler`
  - [ ] Change return type from `goamqp.HandlerFunc` to `spec.EventHandler[any]`
  - [ ] Update handler body to use `spec.ConsumableEvent[any]` (access payload via `event.Payload`)
  - [ ] Remove `ErrRecoverable` wrapping — return errors directly
  - [ ] Remove response return values (handlers only return `error` now)
  - [ ] Update imports: `goamqp` → `gomessaging/spec`
- [ ] **handlers_test.go**:
  - [ ] Remove `MockConnection` struct and `Connection` interface assertion
  - [ ] Update test handler calls: pass `context.Background()` + `spec.ConsumableEvent[any]{Payload: event}`
  - [ ] Replace `goamqp.ErrRecoverable` assertions with direct error assertions
  - [ ] Remove response value assertions
  - [ ] Update imports: `goamqp` → `gomessaging/spec`
- [ ] **Caller services** (outside this repo):
  - [ ] Replace `WithTypeMapping` calls with a `TypeMapper` function
  - [ ] Wrap readview handlers with `amqp.TypeMappingHandler(handler, typeMapper)` at the call site
  - [ ] Consider adding `amqp.WithLegacySupport()` if consuming from pre-CloudEvents publishers
- [ ] Run tests: `go test ./...`
- [ ] Run linter: `golangci-lint run`
