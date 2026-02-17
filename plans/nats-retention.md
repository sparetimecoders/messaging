# NATS Stream Retention Limits

## Context

JetStream streams are created via `ensureStream` (`connection.go:208-214`) with only
`Name`, `Subjects`, and `Storage` configured. No `MaxAge`, `MaxBytes`, or `MaxMsgs`
limits are set, meaning streams grow without bound until the NATS server runs out of
disk. In production, this risks storage exhaustion and cascading failures.

## Current State

### Stream creation — `connection.go:208-214`

```go
func (c *Connection) ensureStream(ctx context.Context, name string) (jetstream.Stream, error) {
    return c.js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
        Name:     name,
        Subjects: []string{streamSubjects(name)},
        Storage:  jetstream.FileStorage,
    })
}
```

### Where `ensureStream` is called

| Caller | File:line | Context |
|---|---|---|
| `startPendingJSConsumers` | `setup_consumer.go:215` | During grouped consumer startup, once per unique stream |
| `StreamPublisher` | `setup_publisher.go:43` | During publisher setup |

Both callers use `context.Background()` and pass only the stream name.

### Stream config is not configurable

There is no way for users to influence the `jetstream.StreamConfig` passed to
`CreateOrUpdateStream`. The `Setup` options pattern (`setup.go`) has options for
logging, tracing, propagator, span names, and notification channels — but nothing
for stream configuration.

### Consumer config vs stream config

The existing `ConsumerOptions` type (`consumer.go:52`) configures the **JetStream
consumer** (durable name suffix), not the **stream**. These are distinct NATS concepts:
- **Stream** = the persistent log (retention, storage limits, subjects)
- **Consumer** = a cursor into the stream (durable name, filter subjects, ack policy)

## Chosen Approach: Connection-Level Stream Defaults with Per-Stream Overrides

### Design

Add a `StreamConfig` struct that captures the retention-relevant subset of
`jetstream.StreamConfig`. Provide two Setup options:

1. **`WithStreamDefaults(cfg StreamConfig)`** — sets default limits applied to all
   streams created by this connection.
2. **`WithStreamConfig(stream string, cfg StreamConfig)`** — overrides defaults for
   a specific named stream.

The `ensureStream` method merges: NATS zero-values ← connection defaults ← per-stream
overrides ← hard-coded fields (Name, Subjects, Storage).

### Why not just set hard-coded defaults?

Different workloads have fundamentally different retention needs:
- An audit stream may need 365 days / unlimited bytes.
- A notification stream may need 24 hours / 100MB.
- A high-throughput event stream may need 7 days / 1GB / 10M messages.

Hard-coded defaults would force users to override them in most cases, defeating the
purpose. Instead, we keep the NATS defaults (unlimited) as the library default, but
make it trivial to set limits, and log a warning when a stream is created with no
limits configured.

### Why connection-level defaults + per-stream overrides?

Most services use consistent retention across their streams (e.g., "7 days for
everything"). Per-stream overrides handle the exceptions. This two-tier model:
- Reduces boilerplate (set once, applies everywhere)
- Follows the existing pattern of connection-level `With*` options
- Keeps the common case simple while allowing fine-grained control

### StreamConfig field selection

We expose only the retention-relevant fields, not the full `jetstream.StreamConfig`:

```go
type StreamConfig struct {
    MaxAge  time.Duration // 0 = unlimited (NATS default)
    MaxBytes int64        // 0 = unlimited
    MaxMsgs  int64        // 0 = unlimited
}
```

We intentionally omit `Replicas`, `Retention`, `Discard`, `Storage`, `MaxMsgSize`,
etc. These are operational concerns better handled by NATS server config or a
dedicated stream management tool. The library focuses on the most common retention
footguns.

### `CreateOrUpdateStream` compatibility

NATS JetStream's `CreateOrUpdateStream` (`js.CreateOrUpdateStream`) handles config
changes to existing streams gracefully:

- Adding or changing `MaxAge`/`MaxBytes`/`MaxMsgs` on an existing stream **succeeds**
  and the new limits take effect immediately.
- Messages exceeding the new limits are asynchronously pruned.
- Reducing limits is a **non-breaking** operation — NATS does not reject it.
- Fields not set (zero value) mean "unlimited" in NATS — this is the current behavior.

This means deploying updated retention limits requires no migration step. The next
`Start()` call applies the new config automatically.

**Important caveat**: Certain fields (like `Retention` policy change from `LimitsPolicy`
to `WorkQueuePolicy`) **cannot** be changed on an existing stream. Since we don't expose
`Retention`, this doesn't affect us. If it ever becomes relevant, the error from
`CreateOrUpdateStream` will surface clearly.

## Implementation Plan

### Step 1: Add `StreamConfig` type and connection fields

**File: `golang/nats/setup.go`**

Add the `StreamConfig` type and two new `Setup` functions:

```go
// StreamConfig configures retention limits for JetStream streams.
type StreamConfig struct {
    // MaxAge is the maximum age of messages in the stream.
    // Zero means unlimited (NATS default).
    MaxAge time.Duration

    // MaxBytes is the maximum total size of messages in the stream.
    // Zero means unlimited.
    MaxBytes int64

    // MaxMsgs is the maximum number of messages in the stream.
    // Zero means unlimited.
    MaxMsgs int64
}

// WithStreamDefaults sets default retention limits applied to all streams
// created by this connection. Per-stream overrides take precedence.
func WithStreamDefaults(cfg StreamConfig) Setup {
    return func(conn *Connection) error {
        conn.streamDefaults = cfg
        return nil
    }
}

// WithStreamConfig sets retention limits for a specific named stream,
// overriding any connection-level defaults.
func WithStreamConfig(stream string, cfg StreamConfig) Setup {
    return func(conn *Connection) error {
        if stream == "" {
            return fmt.Errorf("stream name must not be empty")
        }
        name := streamName(stream)
        if conn.streamConfigs == nil {
            conn.streamConfigs = make(map[string]StreamConfig)
        }
        conn.streamConfigs[name] = cfg
        return nil
    }
}
```

**File: `golang/nats/connection.go`**

Add fields to `Connection`:

```go
type Connection struct {
    // ... existing fields ...

    streamDefaults StreamConfig            // connection-level defaults
    streamConfigs  map[string]StreamConfig // per-stream overrides
}
```

### Step 2: Update `ensureStream` to apply retention config

**File: `golang/nats/connection.go`**

Replace the current `ensureStream` (lines 208-214):

```go
// ensureStream creates or updates a JetStream stream with retention limits.
func (c *Connection) ensureStream(ctx context.Context, name string) (jetstream.Stream, error) {
    cfg := jetstream.StreamConfig{
        Name:     name,
        Subjects: []string{streamSubjects(name)},
        Storage:  jetstream.FileStorage,
    }

    // Apply connection-level defaults.
    sc := c.streamDefaults
    // Per-stream overrides replace defaults entirely (not merged field-by-field).
    if override, ok := c.streamConfigs[name]; ok {
        sc = override
    }

    cfg.MaxAge = sc.MaxAge
    cfg.MaxBytes = sc.MaxBytes
    cfg.MaxMsgs = sc.MaxMsgs

    if sc.MaxAge == 0 && sc.MaxBytes == 0 && sc.MaxMsgs == 0 {
        c.log().Warn("stream has no retention limits configured, storage may grow unbounded",
            "stream", name,
        )
    }

    return c.js.CreateOrUpdateStream(ctx, cfg)
}
```

**Note on duplicate warnings:** `ensureStream` is called from both `StreamPublisher`
(`setup_publisher.go:43`) and `startPendingJSConsumers` (`setup_consumer.go:215`). If
a publisher and consumer both use the same stream, the warning fires twice. This is
accepted as harmless — `CreateOrUpdateStream` is idempotent, and the duplicate warning
is noisy but not incorrect. A `warnedStreams map[string]bool` could deduplicate, but
adds complexity for negligible benefit.

### Step 3: Per-stream override replaces defaults entirely (not field-by-field merge)

When a per-stream config is set via `WithStreamConfig`, it **replaces** the defaults
entirely rather than merging field-by-field. This is simpler and more predictable:

```go
// If defaults are MaxAge=7d, MaxBytes=1GB, MaxMsgs=0
// and per-stream override is MaxAge=30d
// Result: MaxAge=30d, MaxBytes=0 (unlimited), MaxMsgs=0 (unlimited)
// NOT: MaxAge=30d, MaxBytes=1GB, MaxMsgs=0
```

This avoids confusion about which fields were explicitly set vs left at zero. If a
user overrides a stream, they take full control of its retention. This is the same
pattern used by `ConsumerOptions` — options replace, not merge.

### Step 4: Update `collectMode` path

**File: `golang/nats/connection.go`**

In `collectMode` (topology export), `ensureStream` is never called (guarded by
`if !c.collectMode` checks in `setup_consumer.go:74` and `setup_publisher.go:42`).
No changes needed for topology collection.

### Step 5: Add unit tests

**File: `golang/nats/setup_test.go`** (add to existing file)

```go
func TestWithStreamDefaults(t *testing.T) {
    conn := newConnection("test-svc", "nats://localhost:4222")
    err := WithStreamDefaults(StreamConfig{
        MaxAge:   7 * 24 * time.Hour,
        MaxBytes: 1 << 30, // 1 GiB
    })(conn)
    assert.NoError(t, err)
    assert.Equal(t, 7*24*time.Hour, conn.streamDefaults.MaxAge)
    assert.Equal(t, int64(1<<30), conn.streamDefaults.MaxBytes)
}

func TestWithStreamConfig(t *testing.T) {
    conn := newConnection("test-svc", "nats://localhost:4222")
    err := WithStreamConfig("audit", StreamConfig{
        MaxAge: 365 * 24 * time.Hour,
    })(conn)
    assert.NoError(t, err)
    assert.Len(t, conn.streamConfigs, 1)
    cfg := conn.streamConfigs[streamName("audit")]
    assert.Equal(t, 365*24*time.Hour, cfg.MaxAge)
}

func TestWithStreamConfig_OverridesDefaults(t *testing.T) {
    conn := newConnection("test-svc", "nats://localhost:4222")
    _ = WithStreamDefaults(StreamConfig{MaxAge: 7 * 24 * time.Hour})(conn)
    _ = WithStreamConfig("audit", StreamConfig{MaxAge: 365 * 24 * time.Hour})(conn)

    // Per-stream config should take precedence (verified via integration test)
    assert.Equal(t, 365*24*time.Hour, conn.streamConfigs[streamName("audit")].MaxAge)
}

func TestWithStreamConfig_EmptyStreamName(t *testing.T) {
    conn := newConnection("test-svc", "nats://localhost:4222")
    err := WithStreamConfig("", StreamConfig{MaxAge: time.Hour})(conn)
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "stream name must not be empty")
}
```

**File: `golang/nats/connection_test.go`** (add to existing file)

```go
func TestEnsureStream_AppliesDefaults(t *testing.T) {
    s := startTestServer(t)
    url := serverURL(s)

    conn, err := NewConnection("retention-svc", url)
    require.NoError(t, err)

    pub := NewPublisher()
    err = conn.Start(context.Background(),
        WithStreamDefaults(StreamConfig{
            MaxAge:   24 * time.Hour,
            MaxBytes: 1024 * 1024, // 1 MiB
            MaxMsgs:  1000,
        }),
        EventStreamPublisher(pub),
    )
    require.NoError(t, err)
    defer conn.Close()

    // Verify stream config via JetStream API.
    // NOTE: conn.js is an internal field — this works because tests are in
    // package nats (not nats_test). Do not extract to an external test package.
    info, err := conn.js.Stream(context.Background(), streamName("events"))
    require.NoError(t, err)
    cfg := info.CachedInfo().Config
    assert.Equal(t, 24*time.Hour, cfg.MaxAge)
    assert.Equal(t, int64(1024*1024), cfg.MaxBytes)
    assert.Equal(t, int64(1000), cfg.MaxMsgs)
}

func TestEnsureStream_PerStreamOverride(t *testing.T) {
    s := startTestServer(t)
    url := serverURL(s)

    conn, err := NewConnection("override-svc", url)
    require.NoError(t, err)

    eventPub := NewPublisher()
    auditPub := NewPublisher()
    err = conn.Start(context.Background(),
        WithStreamDefaults(StreamConfig{MaxAge: 24 * time.Hour}),
        WithStreamConfig("audit", StreamConfig{MaxAge: 365 * 24 * time.Hour}),
        EventStreamPublisher(eventPub),
        StreamPublisher("audit", auditPub),
    )
    require.NoError(t, err)
    defer conn.Close()

    // Events stream should use defaults
    eventsInfo, err := conn.js.Stream(context.Background(), streamName("events"))
    require.NoError(t, err)
    assert.Equal(t, 24*time.Hour, eventsInfo.CachedInfo().Config.MaxAge)

    // Audit stream should use per-stream override
    auditInfo, err := conn.js.Stream(context.Background(), streamName("audit"))
    require.NoError(t, err)
    assert.Equal(t, 365*24*time.Hour, auditInfo.CachedInfo().Config.MaxAge)
    // MaxBytes/MaxMsgs should be zero (unlimited) since override replaces entirely
    assert.Equal(t, int64(0), auditInfo.CachedInfo().Config.MaxBytes)
}

func TestEnsureStream_DefaultsApplyViaConsumerPath(t *testing.T) {
    // Verifies that stream defaults also apply when the stream is created
    // via the consumer path (startPendingJSConsumers -> ensureStream), not
    // just via StreamPublisher. Both paths go through ensureStream with
    // streamName()-transformed names, so the streamConfigs key must match.
    s := startTestServer(t)
    url := serverURL(s)

    handler := func(ctx context.Context, event spec.ConsumableEvent[testMessage]) error {
        return nil
    }

    conn, err := NewConnection("consumer-retention-svc", url)
    require.NoError(t, err)

    err = conn.Start(context.Background(),
        WithStreamDefaults(StreamConfig{
            MaxAge: 48 * time.Hour,
        }),
        EventStreamConsumer("Order.Created", handler),
    )
    require.NoError(t, err)
    defer conn.Close()

    // NOTE: conn.js is an internal field — tests are in package nats.
    info, err := conn.js.Stream(context.Background(), streamName("events"))
    require.NoError(t, err)
    assert.Equal(t, 48*time.Hour, info.CachedInfo().Config.MaxAge)
}

func TestEnsureStream_NoLimitsWarning(t *testing.T) {
    // This test verifies that a warning is logged when no limits are set.
    // Use a custom slog handler to capture log output.
    s := startTestServer(t)
    url := serverURL(s)

    var buf bytes.Buffer
    handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})
    logger := slog.New(handler)

    conn, err := NewConnection("warn-svc", url)
    require.NoError(t, err)

    pub := NewPublisher()
    err = conn.Start(context.Background(),
        WithLogger(logger),
        EventStreamPublisher(pub),
    )
    require.NoError(t, err)
    defer conn.Close()

    assert.Contains(t, buf.String(), "no retention limits configured")
}
```

## Files Modified

| File | Change |
|---|---|
| `golang/nats/connection.go` | Add `streamDefaults`/`streamConfigs` fields, update `ensureStream` to apply limits and log warning |
| `golang/nats/setup.go` | Add `StreamConfig` type, `WithStreamDefaults`, `WithStreamConfig` |
| `golang/nats/setup_test.go` | Add unit tests for new Setup options |
| `golang/nats/connection_test.go` | Add integration tests for retention config on real NATS server |

## New Public API

| Symbol | Description |
|---|---|
| `StreamConfig` | Struct with `MaxAge`, `MaxBytes`, `MaxMsgs` fields |
| `WithStreamDefaults(StreamConfig)` | Setup option for connection-level stream defaults |
| `WithStreamConfig(stream, StreamConfig)` | Setup option for per-stream overrides |

## Backward Compatibility

- **No breaking changes.** All new API is additive.
- Existing code that calls `Start()` without `WithStreamDefaults` or `WithStreamConfig`
  works exactly as before — streams are created with unlimited retention (NATS defaults).
- The only behavioral change is a new `WARN` log when streams have no limits. This is
  intentional to nudge users toward setting explicit limits without breaking anything.
- `CreateOrUpdateStream` applies new limits to existing streams non-destructively.
  Deploying updated retention requires no migration — just restart the service with the
  new config.

## Usage Example

```go
conn, _ := nats.NewConnection("order-svc", "nats://localhost:4222")

pub := nats.NewPublisher()
auditPub := nats.NewPublisher()
err := conn.Start(ctx,
    // All streams default to 7 days, 1 GiB, 10M messages
    nats.WithStreamDefaults(nats.StreamConfig{
        MaxAge:   7 * 24 * time.Hour,
        MaxBytes: 1 << 30,
        MaxMsgs:  10_000_000,
    }),
    // Audit stream: 1 year, no size limit
    nats.WithStreamConfig("audit", nats.StreamConfig{
        MaxAge: 365 * 24 * time.Hour,
    }),
    nats.EventStreamPublisher(pub),
    nats.StreamPublisher("audit", auditPub),
)
```

## Test Plan

1. **Unit tests**: `cd golang && go test -race -count=1 ./nats/...` — new tests for
   `WithStreamDefaults`, `WithStreamConfig`, and the warning log.
2. **Integration tests**: New tests verify actual stream config via JetStream info API
   on an embedded NATS server (same pattern as existing tests).
3. **Spec tests**: `cd specification && go test -race -count=1 ./spec/... ./specverify/...`
   — no spec changes, but verify no regressions.
4. **Topology conformance**: Existing `topology_conformance_test.go` must still pass
   (stream config doesn't affect topology collection since `collectMode` skips
   `ensureStream`).
