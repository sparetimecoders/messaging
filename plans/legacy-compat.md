# Legacy goamqp Backward Compatibility Plan

## Status: IMPLEMENTED

All steps in this plan have been implemented and tested. This document serves as a record of the design decisions and implementation.

## Context

When an old goamqp publisher sends a message (no `ce-*` headers), the new AMQP consumer logs at DEBUG and proceeds with zero-valued `Metadata`. Handlers relying on `Metadata.ID` for deduplication or `Metadata.Timestamp` for ordering silently get empty/zero values. This plan added opt-in metadata enrichment for legacy messages.

## What Was Implemented

### spec module (`specification/spec/`)

| Change | Location |
|--------|----------|
| `Metadata.HasCloudEvents() bool` | `event.go:50-56` |
| `IDGenerator` type | `cloudevents.go:113-115` |
| `EnrichLegacyMetadata()` function | `cloudevents.go:117-147` |
| `TestMetadata_HasCloudEvents` | `event_test.go:54` |
| `TestEnrichLegacyMetadata` (4 subtests) | `cloudevents_test.go:206` |

### amqp module (`golang/amqp/`)

| Change | Location |
|--------|----------|
| `legacySupport bool` on `Connection` | `connection.go:60` |
| Propagation in `startConsumers()` | `connection.go:336` |
| `WithLegacySupport()` setup option | `setup.go:124-136` |
| `legacySupport bool` on `queueConsumer` | `consumer.go:49` |
| Enrichment in `handleDelivery()` | `consumer.go:100-108` |
| `Test_LegacyMessage_WithLegacySupport_EnrichedMetadata` | `consumer_test.go:548` |
| `Test_LegacyMessage_WithoutLegacySupport_StillZeroMetadata` | `consumer_test.go:600` |
| `Test_CEMessage_WithLegacySupport_NotEnriched` | `consumer_test.go:643` |
| `Test_WithLegacySupport` | `setup_test.go:97` |

## Design Decisions

1. **Enricher lives in spec, not amqp**: Transport-agnostic. If NATS ever needs legacy compat, the same function works. Uses `IDGenerator` callback to avoid adding `google/uuid` dependency to spec module.

2. **`HasCloudEvents()` is on Metadata**: Lets handlers write `if !event.HasCloudEvents() { ... }` naturally via embedded struct. Note: returns `true` for both real CE headers AND synthetically enriched metadata — the semantic is "usable metadata", not "real CE headers on the wire". Use `spec.HasCEHeaders(event.DeliveryInfo.Headers)` to check raw headers.

3. **Opt-in via `WithLegacySupport()`**: Default behavior unchanged — existing consumers still get zero-valued Metadata for legacy messages. Existing test `Test_LegacyMessage_NoHeaders_DebugLogOnly` passes unchanged.

4. **`legacySupport` propagation**: Flows from `Connection` to `queueConsumer` via `startConsumers()`, following the same pattern as `logger`, `tracer`, `propagator`, `serviceName`.

5. **setDefault pattern reuse**: `EnrichLegacyMetadata()` mirrors the publisher-side `setDefault` pattern — only sets fields that are empty, derives Type from routing key, Source from exchange name.

## Interaction with CE Header Prefix Plan

If the CE header prefix plan adds a `NormalizeCEHeaders` step, the consumer flow would be:
```
NormalizeCEHeaders → HasCEHeaders → MetadataFromHeaders → EnrichLegacyMetadata
```
This works correctly — normalization runs before legacy detection, so messages with non-standard CE prefixes (e.g., `cloudEvents:`) are normalized to `ce-*` before `HasCEHeaders` checks them. Legacy messages (no CE headers at all) are unaffected by normalization.

## Remaining Gaps

1. **NATS transport**: No legacy support needed currently (NATS is new, no legacy publishers exist). The spec-level `EnrichLegacyMetadata()` is ready if needed in the future.

2. **Prometheus metrics for enriched messages**: No dedicated counter for enriched legacy messages. Could add `legacy_messages_enriched_total` if observability is needed.

3. **Handler-level guards**: No middleware that rejects messages with incomplete metadata. Handlers must check `event.HasCloudEvents()` themselves if they require CE metadata.

## Verification

All tests pass:
```bash
cd specification && go test -race -count=1 ./spec/...        # PASS
cd specification && go test -race -count=1 ./specverify/...   # PASS
cd golang && go test -race -count=1 ./amqp/...                # PASS
cd golang && go test -race -count=1 ./nats/...                # PASS
```

## Migration Guide

```go
// Before: legacy messages have zero Metadata
conn.Start(ctx, amqp.EventStreamConsumer("Order.Created", handler))

// After: add WithLegacySupport() to get enriched Metadata
conn.Start(ctx, amqp.WithLegacySupport(), amqp.EventStreamConsumer("Order.Created", handler))

// To distinguish real CE from enriched legacy in handlers:
if !spec.HasCEHeaders(event.DeliveryInfo.Headers) {
    // This was a legacy message (metadata is synthetic)
}
```
