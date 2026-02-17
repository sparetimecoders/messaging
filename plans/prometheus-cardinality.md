# Prometheus Metrics High Cardinality Fix

## Context

Both AMQP and NATS transports use `routing_key` as a Prometheus label on all 18 metric vectors. The routing key value comes directly from the wire (AMQP `delivery.RoutingKey` / NATS subject). If routing keys ever contain dynamic segments (entity IDs, UUIDs like `order.created.550e8400-e29b-41d4-a716-446655440000`), the unbounded label cardinality causes metric explosion and Prometheus OOM.

Currently routing keys follow static `Entity.Event` patterns (`Order.Created`, `email.send`), so this is a latent risk — but there is no enforcement preventing dynamic keys, and one careless publisher can break the entire monitoring stack.

Additionally, the NATS publisher metrics use the full NATS subject (`events.Order.Created`) as a `subject` label, which is inconsistent with the consumer metrics and AMQP transport that use `routing_key`.

## Approach: Configurable `WithRoutingKeyMapper`

Add an optional `func(string) string` mapper to `InitMetrics` via a functional option. The mapper is applied to every routing key before it becomes a Prometheus label value.

**Why this approach:**
- Pure `func(string) string` — zero new dependencies
- Consistent with existing patterns (`WithSpanNameFn`, `WithPublishSpanNameFn`)
- User owns the transformation logic — works for any future key shape
- Default is identity (pass-through) — **no breaking change for existing users**
- Variadic `...MetricsOption` on `InitMetrics` is fully backward-compatible

**Code duplication across amqp/ and nats/:** Both packages get identical `MetricsOption`, `metricsConfig`, `WithRoutingKeyMapper`, `routingKeyMapperVal`, `routingKeyLabel`. This is intentionally duplicated because `spec/` must remain transport-agnostic with zero Prometheus dependency. Each transport owns its metrics lifecycle and can evolve its options independently.

**Rejected alternatives:**
- *CloudEvents `ce-type` header* — same unbounded value as routing key, no cardinality protection
- *Drop `routing_key` entirely* — loses per-event-type dashboards/alerting capability, irreversible API break
- *LRU-capped cardinality* — non-deterministic, silent metric loss, unnecessary complexity for currently-static keys

## Current State

### AMQP Metrics (`golang/amqp/metrics.go`)
9 metric vectors, all with `routing_key` label:
- Consumer: `amqp_events_{received,without_handler,not_parsable,nack,ack}{queue, routing_key}`
- Consumer duration: `amqp_events_processed_duration{queue, routing_key, result}`
- Publisher: `amqp_events_publish_{succeed,failed}{exchange, routing_key}`
- Publisher duration: `amqp_events_publish_duration{exchange, routing_key, result}`

Helper functions (`eventReceived`, `eventAck`, etc.) pass raw routing key directly to `.WithLabelValues()`.

### NATS Metrics (`golang/nats/metrics.go`)
9 metric vectors:
- Consumer: uses `[consumer, routing_key]` — same pattern as AMQP
- Publisher: uses `[stream, subject]` — **inconsistent** — passes full NATS subject not routing key

### Call Sites
- **AMQP consumer** (`golang/amqp/consumer.go:70,79,124,131,140,150`): passes `deliveryInfo.Key` (raw AMQP routing key)
- **AMQP publisher** (`golang/amqp/setup_publisher.go:218,222`): passes `routingKey` parameter
- **NATS consumer** (`golang/nats/consumer.go:127,135,187,194,203,213,223,231,268,280`): passes extracted routing key
- **NATS publisher** (`golang/nats/publisher.go:139,143`): passes `subject` (full NATS subject) — **bug**

## Implementation Steps

### Step 1: Add `MetricsOption` and `WithRoutingKeyMapper` to both packages

In both `golang/amqp/metrics.go` and `golang/nats/metrics.go`:

```go
// MetricsOption configures Prometheus metric behavior.
type MetricsOption func(*metricsConfig)

type metricsConfig struct {
    routingKeyMapper func(string) string
}

// WithRoutingKeyMapper sets a function applied to every routing key before it is
// used as a Prometheus label value. Use this to normalize or redact dynamic
// segments (e.g. UUIDs) from routing keys to prevent unbounded label cardinality.
// The default is the identity function (labels pass through unchanged).
// The mapper must return a non-empty string; empty returns are replaced with "unknown".
func WithRoutingKeyMapper(fn func(string) string) MetricsOption {
    return func(cfg *metricsConfig) {
        cfg.routingKeyMapper = fn
    }
}

// package-level mapper, set by InitMetrics. Stored in atomic.Value for
// goroutine safety — reads from hot-path metric helpers are lock-free.
var routingKeyMapperVal atomic.Value // stores func(string) string

func init() {
    routingKeyMapperVal.Store(func(s string) string { return s })
}

func routingKeyLabel(key string) string {
    mapped := routingKeyMapperVal.Load().(func(string) string)(key)
    if mapped == "" {
        return "unknown"
    }
    return mapped
}
```

### Step 2: Update `InitMetrics` signature (both packages)

```go
// Before:
func InitMetrics(registerer prometheus.Registerer) error

// After:
func InitMetrics(registerer prometheus.Registerer, opts ...MetricsOption) error
```

Inside, apply options before registration:
```go
cfg := &metricsConfig{routingKeyMapper: func(s string) string { return s }}
for _, o := range opts {
    o(cfg)
}
routingKeyMapperVal.Store(cfg.routingKeyMapper)
```

Note: `atomic.Value` makes the mapper provably race-free even if `InitMetrics` is called after consumers start. Reads from `routingKeyMapperVal.Load()` in metric hot paths are lock-free. Requires adding `"sync/atomic"` to imports.

Variadic `...MetricsOption` = fully backward-compatible, existing `InitMetrics(registry)` calls compile unchanged.

### Step 3: Update all metric helper functions (both packages)

Replace every `routingKey` passed to `.WithLabelValues()` with `routingKeyLabel(routingKey)`:

```go
// Before:
func eventReceived(queue string, routingKey string) {
    eventReceivedCounter.WithLabelValues(queue, routingKey).Inc()
}

// After:
func eventReceived(queue string, routingKey string) {
    eventReceivedCounter.WithLabelValues(queue, routingKeyLabel(routingKey)).Inc()
}
```

Apply to all 7 AMQP helpers and all 7 NATS helpers.

### Step 4: Fix NATS publisher label value (keep label name for now)

The NATS publisher has a double-breaking change risk: the label **name** (`subject` → `routing_key`) AND the label **value** (full subject `events.Order.Created` → routing key `Order.Created`) would both change at once, making dashboard migration confusing during deploy.

**Strategy: fix value now, defer label rename.**

**Step 4a — Fix value only** (this PR):

**`golang/nats/publisher.go`**: Change calls from passing `subject` to `routingKey`, so the value is now the logical routing key (consistent with consumer metrics):
```go
// Before (line 139):
eventPublishFailed(p.stream, subject, elapsed)
// After:
eventPublishFailed(p.stream, routingKey, elapsed)

// Before (line 143):
eventPublishSucceed(p.stream, subject, elapsed)
// After:
eventPublishSucceed(p.stream, routingKey, elapsed)
```

**`golang/nats/metrics.go`**: Apply mapper in publisher helpers (label name stays `subject` for now):
```go
func eventPublishSucceed(stream string, routingKey string, milliseconds int64) {
    eventPublishSucceedCounter.WithLabelValues(stream, routingKeyLabel(routingKey)).Inc()
    eventPublishDuration.WithLabelValues(stream, routingKeyLabel(routingKey), "OK").Observe(...)
}
```

**Step 4b — Rename label name** (separate follow-up, out of scope):

Renaming `metricSubject` → `metricRoutingKey` on the 3 NATS publisher metric vectors is deferred to a separate PR. This lets users migrate dashboard label values first (`events.Order.Created` → `Order.Created`), then update label names (`subject` → `routing_key`) in a subsequent deploy.

Document the planned rename in the migration notes so users know it's coming.

### Step 5: Add tests

**`golang/amqp/metrics_test.go`** — add:
```go
func Test_InitMetrics_WithRoutingKeyMapper(t *testing.T) {
    registry := prometheus.NewRegistry()
    require.NoError(t, InitMetrics(registry, WithRoutingKeyMapper(func(k string) string {
        return "mapped." + k
    })))

    channel := NewMockAmqpChannel()
    err := publishMessage(context.Background(), nil, nil, nil, channel, Message{true}, "Order.Created", "exchange", "svc", nil)
    require.NoError(t, err)

    families, _ := registry.Gather()
    for _, f := range families {
        if *f.Name == "amqp_events_publish_succeed" {
            for _, m := range f.GetMetric() {
                for _, lp := range m.GetLabel() {
                    if *lp.Name == "routing_key" {
                        require.Equal(t, "mapped.Order.Created", *lp.Value)
                    }
                }
            }
        }
    }
}
```

**Both `golang/amqp/metrics_test.go` and `golang/nats/metrics_test.go`** — add default-path test:
```go
func Test_InitMetrics_DefaultMapper(t *testing.T) {
    t.Cleanup(func() { routingKeyMapperVal.Store(func(s string) string { return s }) })
    registry := prometheus.NewRegistry()
    require.NoError(t, InitMetrics(registry)) // no options — identity mapper
    eventReceived("queue", "Order.Created")
    families, _ := registry.Gather()
    found := false
    for _, f := range families {
        if f.GetName() == "amqp_events_received" { // use "nats_events_received" for NATS
            for _, m := range f.GetMetric() {
                for _, lp := range m.GetLabel() {
                    if *lp.Name == "routing_key" {
                        require.Equal(t, "Order.Created", *lp.Value) // identity, not "unknown"
                        found = true
                    }
                }
            }
        }
    }
    require.True(t, found, "routing_key label not found in gathered metrics")
}
```

**`golang/nats/metrics_test.go`** — additionally add:
- Mapper test: call `InitMetrics` with mapper, trigger `eventReceived`, gather, assert label value is mapped
- Publisher value test: trigger `eventPublishSucceed` with routing key, gather, assert `subject` label value is the routing key (not the full NATS subject)

### Step 6: Restore mapper default in test teardown

Both test files should reset the mapper after tests that set a custom one to avoid polluting other tests:
```go
t.Cleanup(func() { routingKeyMapperVal.Store(func(s string) string { return s }) })
```

**Important:** Mapper tests must NOT use `t.Parallel()` since they mutate the package-level `routingKeyMapperVal` global. This is an inherent limitation of the package-level design (consistent with how the existing metric globals work).

## Files Modified

| File | Changes |
|------|---------|
| `golang/amqp/metrics.go` | Add `MetricsOption`, `WithRoutingKeyMapper`, `metricsConfig`, `routingKeyMapper` global, `routingKeyLabel` helper; update `InitMetrics` signature; wrap all 7 helper functions with `routingKeyLabel()` |
| `golang/amqp/metrics_test.go` | Add `Test_InitMetrics_WithRoutingKeyMapper` |
| `golang/nats/metrics.go` | Same as AMQP; update publisher helpers to accept `routingKey` param and apply mapper (label name `subject` kept for now) |
| `golang/nats/publisher.go` | Lines 139,143: change `subject` → `routingKey` in metric calls |
| `golang/nats/metrics_test.go` | Add mapper test + publisher label name test |

**No changes** to `specification/spec/`, consumer files, connection files, or setup files.

## Breaking Changes

| Change | Impact | Migration |
|--------|--------|-----------|
| NATS publisher `subject` label **value** changes from full subject (`events.Order.Created`) to routing key (`Order.Created`) | Label values change for `nats_events_publish_*` metrics | Update dashboard filter values to use routing key without stream prefix |

**Deferred to follow-up PR:**
| Change | Impact | Migration |
|--------|--------|-----------|
| NATS publisher label **name** `subject` → `routing_key` | PromQL queries using `{subject="..."}` break | Update dashboards/alerts to use `{routing_key="..."}` |

No other breaking changes. AMQP metrics and NATS consumer metrics keep identical names, labels, and default values.

**Concrete migration steps for this PR:**
1. Search dashboards and alerts for `nats_events_publish_succeed{subject=`, `nats_events_publish_failed{subject=`, `nats_events_publish_duration{subject=`
2. Update label filter **values** from full NATS subjects (e.g. `events.Order.Created`) to routing keys (e.g. `Order.Created`)
3. Label **name** stays `subject` in this PR — no PromQL key changes needed yet
4. After deploy, verify NATS publisher metrics appear with the shorter routing key values

**Planned follow-up (separate PR):**
1. Label name `subject` → `routing_key` on NATS publisher metrics
2. At that point, update PromQL from `{subject="Order.Created"}` to `{routing_key="Order.Created"}`

## Thread Safety

The `routingKeyMapper` is stored in a `sync/atomic.Value`:
- Written once during `InitMetrics` (startup)
- Read lock-free from concurrent goroutines in metric helper hot paths
- Provably race-free even if `InitMetrics` is called after consumers start (since `InitMetrics` is idempotent and could theoretically be called again)
- No mutex overhead on the metric recording path
- Cost: 4 lines of code (`atomic.Value` var, `init()`, `Load` type-assert)

Add a doc comment on `InitMetrics`: "InitMetrics should be called before Start(). The mapper is applied globally and subsequent calls overwrite the previous mapper."

## Verification

1. Run spec tests: `cd specification && go test -race -count=1 ./spec/... ./specverify/...`
2. Run AMQP tests: `cd golang && go test -race -count=1 ./amqp/...`
3. Run NATS tests: `cd golang && go test -race -count=1 ./nats/...`
4. Verify existing `InitMetrics(registry)` calls (no options) compile and behave identically
5. Verify new mapper test passes and labels are transformed correctly
6. Verify NATS publisher `subject` label **value** is the routing key (e.g., `Order.Created` not `events.Order.Created`)
