# Sloth: Migration Guide from goamqp to gomessaging/amqp

> **Prerequisite:** Read [MIGRATION.md](MIGRATION.md) for the full API mapping and
> general migration concepts. This document covers only the sloth-specific changes.

## 1. Overview

Sloth uses AMQP as both a **source** (consuming delayed messages) and a **sink** (publishing
scheduled messages back to their target service). The codebase has two modules that depend
on `goamqp`:

| Module | Role | Key goamqp APIs used |
|--------|------|---------------------|
| `amqp/` | Source + Sink plugin | `NewFromURL`, `ServiceRequestConsumer`, `WithPrefetchLimit`, `CloseListener`, `PublishServiceResponse`, `ConsumableEvent[any]` |
| `client-amqp/` | Client library for callers of Sloth | `NewFromURL`, `ServicePublisher`, `NewPublisher`, `Publisher.Publish`, `WithTypeMapping`, `TypeMappingHandler`, `ServiceResponseConsumer` |

Both modules need to switch from `github.com/sparetimecoders/goamqp` to
`github.com/sparetimecoders/gomessaging/amqp` + `github.com/sparetimecoders/gomessaging/spec`.

---

## 2. `amqp/amqp.go` Migration

### 2.1 Import changes

**Before:**
```go
import (
    "github.com/sparetimecoders/goamqp"
)
```

**After:**
```go
import (
    "github.com/sparetimecoders/gomessaging/amqp"
    "github.com/sparetimecoders/gomessaging/spec"
)
```

### 2.2 Connection interface

The internal `Connection` interface must be updated to use the new `Setup` type.

**Before:**
```go
type Connection interface {
    Start(ctx context.Context, opts ...goamqp.Setup) error
    Close() error
    PublishServiceResponse(ctx context.Context, targetService, routingKey string, msg any) error
}
```

**After:**
```go
type Connection interface {
    Start(ctx context.Context, opts ...amqp.Setup) error
    Close() error
    PublishServiceResponse(ctx context.Context, targetService, routingKey string, msg any) error
}
```

The `connect` factory function also switches:

**Before:**
```go
func connectAmqp(url string) (Connection, error) {
    return goamqp.NewFromURL("sloth", url)
}
```

**After:**
```go
func connectAmqp(url string) (Connection, error) {
    return amqp.NewFromURL("sloth", url)
}
```

### 2.3 Source.Start — consumer setup

The `Source.Start` method passes setup options to `connection.Start`. All the
option constructors move from the `goamqp` package to the `amqp` package.

**Before:**
```go
func (s *Source) Start(handler model.Storer, quitter model.Quitter) error {
    go func() {
        err := <-s.closeEvents
        s.Logger.Error("received close from AMQP", "error", err)
        quitter.Quit(err)
    }()
    return s.connection.Start(
        context.Background(),
        goamqp.WithPrefetchLimit(20),
        goamqp.ServiceRequestConsumer("delay", s.handle(handler)),
        goamqp.CloseListener(s.closeEvents),
    )
}
```

**After:**
```go
func (s *Source) Start(handler model.Storer, quitter model.Quitter) error {
    go func() {
        err := <-s.closeEvents
        s.Logger.Error("received close from AMQP", "error", err)
        quitter.Quit(err)
    }()
    return s.connection.Start(
        context.Background(),
        amqp.WithPrefetchLimit(20),
        amqp.ServiceRequestConsumer[any]("delay", s.handle(handler)),
        amqp.CloseListener(s.closeEvents),
    )
}
```

> **Note:** `WithPrefetchLimit` now sets the value on the connection, and the
> default is already 20 in gomessaging. You can omit it if 20 is the desired value,
> but keeping it explicit is fine.

### 2.4 Handler signature — `ConsumableEvent`

The handler factory `handle()` changes from `goamqp.ConsumableEvent[any]` to
`spec.ConsumableEvent[any]`. The `Headers` type moves to `spec.Headers` but exposes
the same `.Get()` method.

**Before:**
```go
func (s *Source) handle(handler model.Storer) func(ctx context.Context, msg goamqp.ConsumableEvent[any]) error {
    return func(ctx context.Context, msg goamqp.ConsumableEvent[any]) error {
        temp := msg.DeliveryInfo.Headers.Get("delay-until")
        // ...
        strTarget, ok := msg.DeliveryInfo.Headers.Get("target").(string)
        // ...
        return handler.Store(model.Request{
            DelayedUntil: delayUntil,
            Target:       (*target).String(),
            Payload:      *msg.Payload.(*json.RawMessage),
        })
    }
}
```

**After:**
```go
func (s *Source) handle(handler model.Storer) spec.EventHandler[any] {
    return func(ctx context.Context, msg spec.ConsumableEvent[any]) error {
        temp := msg.DeliveryInfo.Headers.Get("delay-until")
        // ...
        strTarget, ok := msg.DeliveryInfo.Headers.Get("target").(string)
        // ...
        return handler.Store(model.Request{
            DelayedUntil: delayUntil,
            Target:       (*target).String(),
            Payload:      *msg.Payload.(*json.RawMessage),
        })
    }
}
```

Key changes:
- Return type uses the named alias `spec.EventHandler[any]` instead of the raw function signature
- `goamqp.ConsumableEvent[any]` → `spec.ConsumableEvent[any]`
- The body of the handler is **unchanged** — `DeliveryInfo.Headers.Get()` is the same API

### 2.5 Sink.Start — minimal change

The `Sink.Start` only uses `CloseListener`:

**Before:**
```go
return s.connection.Start(
    context.Background(),
    goamqp.CloseListener(s.closeEvents),
)
```

**After:**
```go
return s.connection.Start(
    context.Background(),
    amqp.CloseListener(s.closeEvents),
)
```

### 2.6 Test file (`amqp_test.go`)

The test file uses `goamqp.Setup` and `goamqp.Headers` types. Update accordingly:

- `goamqp.Setup` → `amqp.Setup`
- `goamqp.Headers` → `spec.Headers`
- `MockConnection.start` signature: `func(ctx context.Context, opts ...goamqp.Setup)` → `func(ctx context.Context, opts ...amqp.Setup)`

---

## 3. `client-amqp/client.go` Migration

This module has more significant changes because it uses `WithTypeMapping`,
`TypeMappingHandler`, and the type-based `Publisher.Publish` — all of which have
breaking API changes.

### 3.1 Import changes

**Before:**
```go
import (
    "github.com/sparetimecoders/goamqp"
)
```

**After:**
```go
import (
    "github.com/sparetimecoders/gomessaging/amqp"
    "github.com/sparetimecoders/gomessaging/spec"
)
```

### 3.2 Client struct

The `Client` struct holds a `*goamqp.Connection` and `*goamqp.Publisher`. These change to
the gomessaging equivalents.

**Before:**
```go
type Client struct {
    serviceName string
    mapping     map[reflect.Type]string
    conn        *goamqp.Connection
    publisher   *goamqp.Publisher
}
```

**After:**
```go
type Client struct {
    serviceName string
    mapping     map[reflect.Type]string
    conn        *amqp.Connection
    publisher   *amqp.Publisher
}
```

### 3.3 Publisher.Publish — explicit routing key

The old `Publisher.Publish` used `WithTypeMapping` to resolve the routing key from the
Go type via reflection. The new publisher requires an **explicit routing key**.

**Before:**
```go
func (c *Client) Publish(request interface{}, when time.Time) error {
    key, exists := c.mapping[reflect.TypeOf(request).Elem()]
    if !exists {
        return fmt.Errorf("no config found for type %s", reflect.TypeOf(request).String())
    }
    buff, err := json.Marshal(request)
    if err != nil {
        return fmt.Errorf("json marshal: %w", err)
    }
    return c.publisher.Publish(
        context.Background(),
        json.RawMessage(buff),
        goamqp.Header{
            Key:   "delay-until",
            Value: when,
        },
        goamqp.Header{
            Key:   "target",
            Value: fmt.Sprintf("amqp://%s/%s", c.serviceName, key),
        },
    )
}
```

**After:**
```go
func (c *Client) Publish(request interface{}, when time.Time) error {
    key, exists := c.mapping[reflect.TypeOf(request).Elem()]
    if !exists {
        return fmt.Errorf("no config found for type %s", reflect.TypeOf(request).String())
    }
    buff, err := json.Marshal(request)
    if err != nil {
        return fmt.Errorf("json marshal: %w", err)
    }
    return c.publisher.Publish(
        context.Background(),
        "delay",                       // explicit routing key
        json.RawMessage(buff),
        amqp.Header{
            Key:   "delay-until",
            Value: when,
        },
        amqp.Header{
            Key:   "target",
            Value: fmt.Sprintf("amqp://%s/%s", c.serviceName, key),
        },
    )
}
```

Key changes:
- `c.publisher.Publish(ctx, msg, headers...)` → `c.publisher.Publish(ctx, routingKey, msg, headers...)`
- The routing key `"delay"` is passed explicitly (this was the key registered via `WithTypeMapping("delay", json.RawMessage{})` before)
- `goamqp.Header` → `amqp.Header`
- **Publisher confirms are now on by default** — if the old code relied on fire-and-forget semantics, pass `amqp.WithoutPublisherConfirms()` to `NewPublisher()`

### 3.4 ResponseListener — TypeMappingHandler rewrite

This is the most complex change. The old code used `WithTypeMapping` to register
type↔routing-key mappings on the connection, and `conn.TypeMappingHandler(handler)` (a
method) to look up and deserialize the correct type at runtime.

In gomessaging, `TypeMappingHandler` is a **package-level function** that takes a
`TypeMapper` callback instead of relying on connection state.

**Before:**
```go
func (c *Client) ResponseListener(handler goamqp.Handler, mappings ...Mapping) goamqp.Setup {
    var opts []goamqp.Setup
    for _, r := range mappings {
        opts = append(opts,
            goamqp.WithTypeMapping(r.Key, r.Type),
            goamqp.ServiceResponseConsumer("sloth", r.Key, goamqp.TypeMappingHandler(handler)),
        )
    }
    return func(conn *goamqp.Connection) error {
        for _, opt := range opts {
            if err := opt(conn); err != nil {
                return err
            }
        }
        return nil
    }
}
```

**After:**
```go
func (c *Client) ResponseListener(handler spec.EventHandler[any], mappings ...Mapping) amqp.Setup {
    // Build a TypeMapper from the provided mappings.
    typeMap := make(map[string]reflect.Type)
    for _, m := range mappings {
        typeMap[m.Key] = reflect.TypeOf(m.Type)
    }
    mapper := func(routingKey string) (reflect.Type, bool) {
        t, ok := typeMap[routingKey]
        return t, ok
    }

    wrappedHandler := amqp.TypeMappingHandler(handler, mapper)

    var opts []amqp.Setup
    for _, r := range mappings {
        opts = append(opts,
            amqp.ServiceResponseConsumer[any]("sloth", r.Key, wrappedHandler),
        )
    }
    return func(conn *amqp.Connection) error {
        for _, opt := range opts {
            if err := opt(conn); err != nil {
                return err
            }
        }
        return nil
    }
}
```

Key changes:
- `goamqp.Handler` → `spec.EventHandler[any]`
- `goamqp.WithTypeMapping(...)` is **removed** — no longer needed
- `goamqp.TypeMappingHandler(handler)` (1-arg method on connection) → `amqp.TypeMappingHandler(handler, mapper)` (2-arg package function)
- The `TypeMapper` is a simple closure: `func(routingKey string) (reflect.Type, bool)`
- `goamqp.ServiceResponseConsumer("sloth", key, handler)` → `amqp.ServiceResponseConsumer[any]("sloth", key, handler)`
- `goamqp.Setup` → `amqp.Setup`
- `*goamqp.Connection` → `*amqp.Connection`

### 3.5 New() constructor

**Before:**
```go
func New(serviceName, amqpURL string, mappings ...Mapping) (*Client, error) {
    conn, err := goamqp.NewFromURL(serviceName, amqpURL)
    if err != nil {
        return nil, err
    }
    mapping := make(map[reflect.Type]string)
    publisher := goamqp.NewPublisher()
    setups := []goamqp.Setup{
        goamqp.ServicePublisher("sloth", publisher),
    }
    for _, r := range mappings {
        mapping[reflect.TypeOf(r.Type)] = r.Key
    }
    setups = append(setups, goamqp.WithTypeMapping("delay", json.RawMessage{}))
    err = conn.Start(context.Background(), setups...)
    if err != nil {
        return nil, err
    }
    return &Client{
        serviceName: serviceName,
        mapping:     mapping,
        conn:        conn,
        publisher:   publisher,
    }, nil
}
```

**After:**
```go
func New(serviceName, amqpURL string, mappings ...Mapping) (*Client, error) {
    conn, err := amqp.NewFromURL(serviceName, amqpURL)
    if err != nil {
        return nil, err
    }
    mapping := make(map[reflect.Type]string)
    publisher := amqp.NewPublisher()
    setups := []amqp.Setup{
        amqp.ServicePublisher("sloth", publisher),
    }
    for _, r := range mappings {
        mapping[reflect.TypeOf(r.Type)] = r.Key
    }
    // WithTypeMapping("delay", json.RawMessage{}) is no longer needed —
    // the routing key is passed explicitly in Publish().
    err = conn.Start(context.Background(), setups...)
    if err != nil {
        return nil, err
    }
    return &Client{
        serviceName: serviceName,
        mapping:     mapping,
        conn:        conn,
        publisher:   publisher,
    }, nil
}
```

Key changes:
- `goamqp.NewFromURL` → `amqp.NewFromURL`
- `goamqp.NewPublisher()` → `amqp.NewPublisher()`
- `goamqp.ServicePublisher` → `amqp.ServicePublisher`
- `goamqp.WithTypeMapping("delay", json.RawMessage{})` is **removed** — not needed since `Publish()` now takes an explicit routing key

---

## 4. go.mod Changes

### `amqp/go.mod`

```diff
-require github.com/sparetimecoders/goamqp v0.3.0
+require (
+    github.com/sparetimecoders/gomessaging/amqp v0.x.0
+    github.com/sparetimecoders/gomessaging/spec v0.x.0
+)
```

### `client-amqp/go.mod`

```diff
-require github.com/sparetimecoders/goamqp v0.3.1
-replace github.com/sparetimecoders/goamqp => /Users/peter/source/stc/goamqp
+require (
+    github.com/sparetimecoders/gomessaging/amqp v0.x.0
+    github.com/sparetimecoders/gomessaging/spec v0.x.0
+)
```

---

## 5. Migration Checklist

### `amqp/` module
- [ ] Update `go.mod`: replace `goamqp` dependency with `gomessaging/amqp` + `gomessaging/spec`
- [ ] Update imports in `amqp.go`: `goamqp` → `amqp` + `spec`
- [ ] Update `Connection` interface: `goamqp.Setup` → `amqp.Setup`
- [ ] Update `connectAmqp`: `goamqp.NewFromURL` → `amqp.NewFromURL`
- [ ] Update `Source.Start`: `goamqp.WithPrefetchLimit` → `amqp.WithPrefetchLimit`
- [ ] Update `Source.Start`: `goamqp.ServiceRequestConsumer` → `amqp.ServiceRequestConsumer[any]`
- [ ] Update `Source.Start` and `Sink.Start`: `goamqp.CloseListener` → `amqp.CloseListener`
- [ ] Update `handle()` return type: `goamqp.ConsumableEvent[any]` → `spec.ConsumableEvent[any]`
- [ ] Update `amqp_test.go`: `goamqp.Setup` → `amqp.Setup`, `goamqp.Headers` → `spec.Headers`
- [ ] Run tests and verify

### `client-amqp/` module
- [ ] Update `go.mod`: replace `goamqp` dependency with `gomessaging/amqp` + `gomessaging/spec`, remove `replace` directive
- [ ] Update imports in `client.go`: `goamqp` → `amqp` + `spec`
- [ ] Update `Client` struct: `*goamqp.Connection` → `*amqp.Connection`, `*goamqp.Publisher` → `*amqp.Publisher`
- [ ] Update `New()`: replace all `goamqp.` prefixes with `amqp.`
- [ ] Remove `goamqp.WithTypeMapping("delay", json.RawMessage{})` from `New()`
- [ ] Update `Publish()`: add explicit routing key `"delay"` as second arg to `publisher.Publish`
- [ ] Update `Publish()`: `goamqp.Header` → `amqp.Header`
- [ ] Rewrite `ResponseListener`:
  - [ ] Change handler param type: `goamqp.Handler` → `spec.EventHandler[any]`
  - [ ] Remove `goamqp.WithTypeMapping` calls
  - [ ] Build a `TypeMapper` closure from the mappings
  - [ ] Replace `goamqp.TypeMappingHandler(handler)` with `amqp.TypeMappingHandler(handler, mapper)`
  - [ ] Replace `goamqp.ServiceResponseConsumer` with `amqp.ServiceResponseConsumer[any]`
- [ ] Consider publisher confirms behavior (now on by default)
- [ ] Run tests and verify
