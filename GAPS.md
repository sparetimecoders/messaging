# Known Gaps and Future Work

Identified during the gomessaging team review (2026-02-12).

## HIGH Priority

### ~~No Dead Letter / Poison Message Handling~~ (RESOLVED)

**Resolved:** AMQP now provides `WithDeadLetter(exchange)` and `WithDeadLetterRoutingKey(key)` consumer options that set `x-dead-letter-exchange` and `x-dead-letter-routing-key` queue arguments. The user manages DLX infrastructure (exchange + dead letter queue). RabbitMQ 4.0 quorum queues enforce `delivery-limit: 20` by default, routing to DLX if configured. See `amqp/queue_binding_config.go`.

NATS now provides `WithMaxDeliver(n)` and `WithBackOff(durations)` per-consumer options, plus `WithConsumerDefaults(cfg)` for connection-level defaults. After MaxDeliver attempts, the server terminates the message. See `nats/consumer.go` and `nats/setup.go`.

### ~~AMQP Publisher Confirms Not Default~~ (RESOLVED)

**Resolved:** Publisher confirms are now enabled by default. `Publish()` waits for broker confirmation and returns error on nack. Use `WithoutPublisherConfirms()` to opt out for high-throughput scenarios. The existing `WithConfirm(ch)` option still works for custom confirm channel usage. See `amqp/setup_publisher.go`.

### ~~NATS Consumer Config Missing MaxDeliver/BackOff~~ (RESOLVED)

**Resolved:** `WithMaxDeliver(n)` and `WithBackOff(durations)` consumer options configure JetStream consumer delivery limits and backoff. `WithConsumerDefaults(cfg)` sets connection-level defaults; per-consumer options take precedence. See `nats/consumer.go`, `nats/setup.go`, and `nats/setup_consumer.go`.

## MEDIUM Priority

### ~~AMQP Publish Channel Not Goroutine-Safe~~ (RESOLVED)

**Resolved:** Each publisher now gets its own dedicated AMQP channel, eliminating the goroutine safety issue. See per-publisher channel allocation in `amqp/setup_publisher.go`.

### ~~NATS Streams Have No Retention Limits~~ (RESOLVED)

**Resolved:** `WithStreamDefaults` applies sensible retention limits (MaxAge 7d, MaxBytes 1GB, MaxMsgs 1M) and `WithStreamConfig` allows full custom configuration. See `nats/stream_options.go`.

### ~~Prometheus Metrics High Cardinality Risk~~ (RESOLVED)

**Resolved:** `WithRoutingKeyMapper` option on `InitMetrics` allows normalizing or redacting dynamic segments from routing keys before they become Prometheus label values. Default is identity (pass-through). See `amqp/metrics.go` and `nats/metrics.go`.

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
