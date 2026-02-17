# AMQP Publish Channel Goroutine Safety

## Context

A single `publishChannel` (`connection.go:48`) is shared by **all** `Publisher` instances
and by `Connection.PublishServiceResponse`. While `amqp091-go` v1.10.0 does internally
serialize frame writes via a mutex (so concurrent `PublishWithContext` calls won't corrupt
the wire protocol), sharing one channel still creates three real problems:

1. **No fault isolation** — a channel-level error (e.g. mandatory publish to a missing
   exchange, or broker-side resource alarm) closes the single channel, killing *every*
   publisher on the connection.
2. **Serialization bottleneck** — all publishes queue behind a single internal mutex,
   preventing the broker from processing messages across channels in parallel.
3. **Broken per-publisher confirms** — `PublishNotify` (`setup.go:135-139`) enables
   confirm mode on the shared channel; confirmations from different publishers are
   multiplexed into one stream with no way to attribute them.

## Current State

### Shared channel creation — `connection.go:135`
```go
if c.publishChannel, err = c.connection.channel(); err != nil {
    return fmt.Errorf("failed to create publish channel: %w", err)
}
```

### All publisher setups receive the same channel
| Setup function | File:line | Usage |
|---|---|---|
| `StreamPublisher` | `setup_publisher.go:88,102` | `exchangeDeclare(c.publishChannel, …)` then `publisher.setup(c.publishChannel, …)` |
| `ServicePublisher` | `setup_publisher.go:125,135` | same pattern |
| `QueuePublisher` | `setup_publisher.go:117` | `publisher.setup(c.publishChannel, …)` |
| `PublishServiceResponse` | `connection.go:89` | `publishMessage(…, c.publishChannel, …)` |
| `PublishNotify` | `setup.go:137-138` | `c.publishChannel.NotifyPublish(…)` / `.Confirm(…)` |

### `Publisher` stores a single channel reference — `setup_publisher.go:41,146`
```go
type Publisher struct {
    channel amqpChannel  // line 41 — set in setup(), line 146
    ...
}
```

### Consumer handlers may publish concurrently
`RequestResponseHandler` (`request_response.go:34-41`) wires consumer handlers to call
`c.PublishServiceResponse` from within the consumer goroutine (`consumer.go:67` /
`connection.go:346`). Multiple consumers run in parallel, all publishing through the
single channel.

### Topology export uses a noop channel — `topology_export.go:38-42`
`CollectTopology` creates a `noopChannel` and assigns it to `publishChannel`. With the
new design this needs to use a noop `amqpConnection` instead.

## Chosen Approach: Per-Publisher Channel

Each `Publisher` gets its own AMQP channel, created during the setup phase.
`PublishServiceResponse` gets a dedicated response channel. Exchange declarations
during setup are moved to `c.setupChannel` (which already exists for this purpose).

### Channel cleanup

Per-publisher channels are cleaned up automatically when `Connection.Close()` calls
`c.connection.Close()` (`connection.go:182`), which closes the underlying
`*amqp.Connection` and implicitly closes all AMQP channels opened on it. There is no
need for `Publisher` to expose its own `Close()` method — publishers are scoped to a
single `Connection` and share its lifecycle. If a future use case requires
decommissioning an individual publisher (e.g. dynamic topic registration), a
`Publisher.Close()` can be added then without breaking the current API.

### Response channel concurrency

`PublishServiceResponse` is called from multiple consumer goroutines concurrently via
`RequestResponseHandler` (`request_response.go:34-41`). It uses a single dedicated
`responseChannel`. This is safe because:

1. `amqp091-go` serializes `PublishWithContext` internally via a mutex — frames will
   not interleave.
2. Per-consumer channel isolation is unnecessary here: all response publishers share
   the same response exchange (`serviceResponseExchangeName`). A channel-level error
   (e.g. broker resource alarm) would affect all response publishes equally regardless
   of how many channels are used, because they target the same exchange.
3. Response traffic is inherently lower-volume than primary event publishing (one
   response per consumed request), so the serialization bottleneck is negligible.

### Why not alternatives?

| Alternative | Reason to reject |
|---|---|
| **Mutex on shared channel** | amqp091-go already serializes internally; an extra mutex adds no safety and doesn't solve fault isolation or confirm routing |
| **Channel pool (checkout/return)** | Over-engineered; the number of publishers is known at Start() time, so a 1:1 mapping is simpler and deterministic |
| **Per-goroutine channels** | AMQP channels are not free (server-side resources, memory); goroutine count is unbounded while publisher count is bounded at setup time |

## Implementation Plan

### Step 1: Remove `publishChannel` from `Connection`

**File: `golang/amqp/connection.go`**

- Remove the `publishChannel amqpChannel` field (line 48).
- Add `responseChannel amqpChannel` field — dedicated channel for `PublishServiceResponse`.
- In `Start()` (line 135), replace the publish channel creation with response channel creation:
  ```go
  if c.responseChannel, err = c.connection.channel(); err != nil {
      return fmt.Errorf("failed to create response channel: %w", err)
  }
  ```
- Update `PublishServiceResponse` (line 89) to use `c.responseChannel`.

### Step 2: Give each Publisher its own channel

**File: `golang/amqp/setup_publisher.go`**

For `StreamPublisher`, `ServicePublisher`, and `QueuePublisher`:

- Move `exchangeDeclare` calls from `c.publishChannel` to `c.setupChannel` (these run
  during setup, not at publish time; `setupChannel` is the correct channel for declarations).
  **This is safe** because all setup functions execute synchronously in `Start()` (lines
  143-147) before `setupChannel.Close()` (line 149). The sequential execution guarantees
  all exchange/queue declarations complete before the setup channel is closed.
- Create a per-publisher channel via `c.connection.channel()`.
- Pass the new channel to `publisher.setup()`.

Example for `StreamPublisher`:
```go
func StreamPublisher(exchange string, publisher *Publisher) Setup {
    exchangeName := topicExchangeName(exchange)
    return func(c *Connection) error {
        if err := exchangeDeclare(c.setupChannel, exchangeName, amqp.ExchangeTopic); err != nil {
            return fmt.Errorf("failed to declare exchange %s, %w", exchangeName, err)
        }
        ch, err := c.connection.channel()
        if err != nil {
            return fmt.Errorf("failed to create publisher channel: %w", err)
        }
        // ... topology + setup ...
        return publisher.setup(ch, c.serviceName, exchangeName, c.tracer(), c.propagator, c.publishSpanNameFn)
    }
}
```

Same pattern for `ServicePublisher` and `QueuePublisher`.

### Step 3: Replace `PublishNotify` with per-Publisher confirm

**File: `golang/amqp/setup_publisher.go`**

Add a `confirmCh` field to `Publisher`:
```go
type Publisher struct {
    channel        amqpChannel
    exchange       string
    serviceName    string
    defaultHeaders []Header
    tracer         trace.Tracer
    propagator     propagation.TextMapPropagator
    spanNameFn     func(exchange, routingKey string) string
    confirmCh      chan amqp.Confirmation // nil = no confirms
}
```

Add a constructor option:
```go
type PublisherOption func(*Publisher)

func WithConfirm(ch chan amqp.Confirmation) PublisherOption {
    return func(p *Publisher) {
        p.confirmCh = ch
    }
}

func NewPublisher(opts ...PublisherOption) *Publisher {
    p := &Publisher{}
    for _, o := range opts {
        o(p)
    }
    return p
}
```

In `Publisher.setup()`, enable confirm mode if requested:
```go
func (p *Publisher) setup(channel amqpChannel, ...) error {
    // ... existing setup ...
    p.channel = channel
    if p.confirmCh != nil {
        channel.NotifyPublish(p.confirmCh)
        if err := channel.Confirm(false); err != nil {
            return fmt.Errorf("failed to enable confirm mode: %w", err)
        }
    }
    return nil
}
```

**File: `golang/amqp/setup.go`**

Remove the `PublishNotify` function entirely. It operated on the shared publish channel
which no longer exists. Users migrate to `NewPublisher(WithConfirm(ch))`.

### Step 4: Update `CollectTopology`

**File: `golang/amqp/topology_export.go`**

Add a `noopConnection` that implements the `amqpConnection` interface (`connection.go:185-188`):

```go
// amqpConnection interface requires:
//   Close() error           — from io.Closer
//   channel() (amqpChannel, error)

type noopConnection struct{}

func (noopConnection) Close() error { return nil }
func (noopConnection) channel() (amqpChannel, error) { return &noopChannel{}, nil }
```

Update `CollectTopology`:
```go
func CollectTopology(serviceName string, setups ...Setup) (spec.Topology, error) {
    noop := &noopChannel{}
    c := &Connection{
        serviceName:  serviceName,
        setupChannel: noop,
        connection:   &noopConnection{},
        // ... rest unchanged ...
    }
    // ...
}
```

Remove the `publishChannel: noop` line since the field no longer exists.

### Step 5: Update mocks and existing tests

**File: `golang/amqp/mocks_test.go`**

Update `mockConnection`:
```go
func mockConnection(channel *MockAmqpChannel) *Connection {
    c := newConnection("svc", amqp.URI{})
    c.setupChannel = channel
    c.responseChannel = channel // for PublishServiceResponse
    c.connection = &MockAmqpConnection{
        channelFn: func() (amqpChannel, error) {
            return channel, nil
        },
    }
    return c
}
```
The `c.publishChannel = channel` line is replaced with `c.responseChannel = channel`.
Without this, `Test_PublishServiceResponse` would crash with a nil pointer dereference
when calling `publishMessage(…, c.responseChannel, …)`. The `channelFn` already returns
the mock channel for per-publisher channels created during setup.

**Note:** This is an intentional test simplification. In production, each
`c.connection.channel()` call yields a distinct `*amqp.Channel` with separate state
(its own confirm sequence, close notification, etc.). In tests, sharing one mock
instance lets us inspect all operations (exchange declarations, publishes, confirms)
on a single object, which is sufficient for unit-level verification.

**File: `golang/amqp/connection_test.go`**

- Update `Test_Start_FailToGetPublishChannel` — rename to test the response channel
  creation failure, and update expected error message.
- Update `Test_Start_FailToGetSetupChannel` — channel creation count changes:
  channel call #1 is now the response channel, #2 is the setup channel.
- Remove or rewrite `Test_PublishNotify` (in `setup_test.go`) to test per-publisher
  confirms instead.

**File: `golang/amqp/setup_test.go`**

- Remove `Test_PublishNotify` (the `PublishNotify` setup function is removed).
- Add `Test_Publisher_WithConfirm` to verify confirm mode is enabled on the
  publisher's own channel.

### Step 6: Add concurrency tests

**File: `golang/amqp/setup_publisher_test.go`** (new tests)

```go
func Test_Publisher_ConcurrentPublish_NoRace(t *testing.T) {
    // Create two publishers with separate mock channels
    // Publish from multiple goroutines concurrently
    // Verify no races (run with -race)
    // Verify all messages are delivered
}

func Test_Publisher_ChannelIsolation(t *testing.T) {
    // Needs a custom setup that returns distinct mock channels per channel() call
    // (cannot use mockConnection which returns the same mock for all calls).
    // Use a channelFn that returns a new MockAmqpChannel each time, keeping
    // references to each so we can simulate an error on one and verify the other
    // still works.
    //
    // Create two publishers via separate setup functions
    // Simulate channel error on one publisher's channel
    // Verify the other publisher can still publish
}

func Test_Publisher_WithConfirm(t *testing.T) {
    // Create publisher with WithConfirm option
    // Verify Confirm() was called on the channel
    // Verify NotifyPublish() was called with the confirm channel
}

func Test_PublishServiceResponse_UsesResponseChannel(t *testing.T) {
    // Create a connection with distinct mock channels for response vs publisher
    // Call PublishServiceResponse
    // Verify the publish went through the responseChannel mock, not a publisher channel
    // Guards against regressions back to a shared channel
}
```

## Files Modified

| File | Change |
|---|---|
| `golang/amqp/connection.go` | Remove `publishChannel`, add `responseChannel`, update `Start()` and `PublishServiceResponse` |
| `golang/amqp/setup_publisher.go` | Per-publisher channels, `PublisherOption`/`WithConfirm`, move exchange decls to setupChannel |
| `golang/amqp/setup.go` | Remove `PublishNotify` |
| `golang/amqp/topology_export.go` | Add `noopConnection`, update `CollectTopology` |
| `golang/amqp/mocks_test.go` | Remove `publishChannel` assignment from `mockConnection` |
| `golang/amqp/connection_test.go` | Update channel-creation tests |
| `golang/amqp/setup_test.go` | Remove `Test_PublishNotify`, add `Test_Publisher_WithConfirm` |
| `golang/amqp/setup_publisher_test.go` | Add concurrent publish and isolation tests |

## New Public API

| Symbol | Description |
|---|---|
| `PublisherOption` | Functional option type for `NewPublisher` |
| `WithConfirm(chan amqp.Confirmation)` | Enables publisher confirms on this publisher's channel |

## Removed Public API

| Symbol | Migration |
|---|---|
| `PublishNotify` | Use `NewPublisher(WithConfirm(ch))` instead |

## Backward Compatibility

- `NewPublisher()` with no options works identically to before (no confirm mode).
- `Publisher.Publish()` signature is unchanged.
- `PublishNotify` is removed — this is a **breaking change**. Since the library is pre-1.0,
  this is acceptable. The migration requires moving confirm setup from `Connection` to
  `Publisher`:
- All other `Setup` functions (`EventStreamPublisher`, `ServicePublisher`, etc.) keep
  the same signatures.

### `PublishNotify` migration example

**Before** (connection-level, shared channel confirms):
```go
pub := amqp.NewPublisher()
confirmCh := make(chan amqp.Confirmation, 10)
conn.Start(ctx,
    amqp.EventStreamPublisher(pub),
    amqp.PublishNotify(confirmCh), // applied to shared publishChannel
)
```

**After** (per-publisher confirms):
```go
confirmCh := make(chan amqp.Confirmation, 10)
pub := amqp.NewPublisher(amqp.WithConfirm(confirmCh)) // confirms on this publisher's channel
conn.Start(ctx,
    amqp.EventStreamPublisher(pub),
)
```

This is an improvement: each publisher now has its own confirm stream, so confirmations
can be attributed to the specific publisher that produced them.

## Test Plan

1. **Unit tests**: Run `cd golang && go test -race -count=1 ./amqp/...`
2. **Race detector**: The concurrent publish test must pass with `-race`.
3. **Spec tests**: Run `cd specification && go test -race -count=1 ./spec/... ./specverify/...`
   to verify no spec regressions.
4. **Integration tests**: `make tests` (requires RabbitMQ) — verifies real broker behavior.
5. **Topology conformance**: Existing `topology_conformance_test.go` must still pass (it uses
   `CollectTopology` which we're updating).
