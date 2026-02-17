# Known Gaps and Future Work

Identified during the gomessaging team review (2026-02-12).

## HIGH Priority

### No Dead Letter / Poison Message Handling

**Current behavior:** AMQP uses `Nack(requeue=true)` and NATS uses bare `Nak()`, which can create infinite retry loops for unprocessable messages.

**Risk:** RabbitMQ 4.0 defaults `delivery-limit` to 20 — without a DLX configured, messages are silently dropped at the limit. NATS has no retry limit at all without `MaxDeliver`.

**Recommendation:** Configure dead letter exchanges (AMQP) and max delivery limits (NATS). Add a `WithDeadLetter` setup option. Consider a `WithMaxRetries(n)` option that works across both transports.

### AMQP Publisher Confirms Not Default

**Current behavior:** Messages are published without waiting for broker confirmation. The `PublishNotify` setup exists but is opt-in.

**Risk:** Messages can be silently lost if the broker doesn't persist them (e.g., during failover).

**Recommendation:** Enable publisher confirms by default. Add a `WithoutPublisherConfirms()` opt-out for high-throughput scenarios where loss is acceptable.

### NATS Consumer Config Missing MaxDeliver/BackOff

**Current behavior:** JetStream consumers are created without `MaxDeliver` or `BackOff` configuration.

**Risk:** Failed messages are redelivered indefinitely with no backoff, causing tight retry loops that can overwhelm downstream services.

**Recommendation:** Set sensible defaults (e.g., `MaxDeliver: 10`, exponential `BackOff`). Add `WithMaxDeliver(n)` and `WithBackOff(durations)` consumer options.

## MEDIUM Priority

### AMQP Publish Channel Not Goroutine-Safe

**Current behavior:** A single `publishChannel` is shared across all goroutines. The underlying `amqp091-go` channel is not safe for concurrent use.

**Recommendation:** Use a channel pool or mutex-protected publish, or document that callers must synchronize access.

### NATS Streams Have No Retention Limits

**Current behavior:** JetStream streams are created without `MaxAge`, `MaxBytes`, or `MaxMsgs` limits.

**Risk:** Unbounded storage growth on the NATS server.

**Recommendation:** Add configurable retention defaults (e.g., `MaxAge: 7d`, `MaxBytes: 1GB`). Expose via `WithStreamRetention()` setup option.

### Prometheus Metrics High Cardinality Risk

**Current behavior:** Metrics include `routing_key` as a label (both AMQP and NATS).

**Risk:** High-cardinality routing keys (e.g., entity IDs in keys) can cause metric explosion and OOM in Prometheus.

**Recommendation:** Replace `routing_key` with a bounded label (e.g., pattern or message type). Or add a configurable label mapper function.

### ~~CloudEvents Header Prefix Convention~~ (RESOLVED)

**Resolved:** The AMQP transport now uses `cloudEvents:` prefix on publish per the CloudEvents AMQP Protocol Binding spec. The consumer normalizes `cloudEvents:*`, `cloudEvents_*` (JMS compat), and `ce-*` prefixes to the canonical `ce-` form at the transport boundary. NATS continues to use `ce-` prefix (correct per NATS binding spec). See `spec.AMQPCEHeaderKey()` and `spec.NormalizeCEHeaders()`.

### Backward Compatibility: Legacy goamqp Messages

**Current behavior:** When an old `goamqp` publisher sends a message (no `ce-*` headers), the new consumer logs a debug-level message and proceeds normally. `event.Metadata` fields are zero-valued.

**Risk:** Handlers that assume `event.Metadata.ID` or `event.Metadata.Timestamp` are always populated may behave incorrectly during mixed deployments (e.g., deduplication logic using `ce-id` as key, or time-based ordering using `ce-time`).

**Recommendation:** Document the edge case (done in amqp README). Handlers should guard against empty metadata during migration periods.

## LOW Priority

### Ack/Nack Error Return Values Silently Ignored

**Current behavior:** `delivery.Ack()` and `delivery.Nack()` errors are not checked in the AMQP consumer loop.

**Recommendation:** Log ack/nack failures at warning level.

### NATS Request Timeout Hardcoded

**Current behavior:** Request-reply timeout is hardcoded to 30 seconds.

**Recommendation:** Make configurable via `WithRequestTimeout(duration)` option.

### API Inconsistencies Between amqp/ and nats/

| Feature | amqp/ | nats/ |
|---------|-------|-------|
| Close listener | `CloseListener(chan error)` | Not available |
| Queue name suffix | `AddQueueNameSuffix(string)` | Not available |
| Skip queue declare | `SkipQueueDeclare()` | Not applicable |
| Prefetch limit | `WithPrefetchLimit(int)` | Not applicable |

**Recommendation:** Align APIs where semantically equivalent. Document transport-specific options clearly.

### Code Duplication in routingkey_handlers

**Current behavior:** `amqp/routingkey_handlers.go` and `nats/routingkey_handlers.go` contain near-identical pattern matching logic.

**Recommendation:** Extract shared matching logic to `spec/` module (e.g., `spec.MatchRoutingKey(pattern, key) bool`).

## Spec Gaps

### `queue-publish` Pattern Has No Topology Scenario

The `PatternQueuePublish` pattern is defined in `spec/topology.go` but has no test scenario in `spec/testdata/topology.json`.

### No NATS Broker State in Topology Fixtures

The `topology.json` fixture includes `broker.amqp` state expectations but no `broker.nats` equivalent for verifying JetStream stream/consumer configuration.
