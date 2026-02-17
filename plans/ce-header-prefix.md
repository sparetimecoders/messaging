# CloudEvents Header Prefix Convention — Implementation Plan

## Context

The gomessaging library currently uses `ce-` prefix for CloudEvents headers in **all** transports. The official CloudEvents AMQP binding specifies `cloudEvents:` (or `cloudEvents_` for JMS compat), while `ce-` is correct for HTTP and NATS. This causes interoperability issues — other CloudEvents AMQP implementations won't recognize our headers and vice versa.

**Goal**: Use correct per-transport prefixes on publish, accept both old and new prefixes on consume (backward compat for existing messages in queues).

**Official spec references**:
- AMQP binding: `cloudEvents:` / `cloudEvents_` prefix ([spec](https://github.com/cloudevents/spec/blob/v1.0.2/cloudevents/bindings/amqp-protocol-binding.md))
- NATS binding: `ce-` prefix ([spec](https://github.com/cloudevents/spec/blob/v1.0.2/cloudevents/bindings/nats-protocol-binding.md))
- HTTP binding: `ce-` prefix ([spec](https://github.com/cloudevents/spec/blob/v1.0.2/cloudevents/bindings/http-protocol-binding.md))

## Approach: Transport-Level Header Mapping with Plain Functions

Keep spec/ transport-agnostic. Provide plain helper functions (no interface) for AMQP header key mapping and normalization. NATS needs no changes since `ce-` is already correct per the NATS binding spec.

### Step 1: Add bare attribute name constants to spec/

**File**: `specification/spec/cloudevents.go`

Add bare attribute name constants (without any prefix) alongside existing `ce-` prefixed constants:

```go
// Bare CloudEvents attribute names (transport-agnostic).
// Transports map these to transport-specific header keys.
const (
    CEAttrSpecVersion     = "specversion"
    CEAttrType            = "type"
    CEAttrSource          = "source"
    CEAttrID              = "id"
    CEAttrTime            = "time"
    CEAttrDataContentType = "datacontenttype"
    CEAttrSubject         = "subject"
    CEAttrDataSchema      = "dataschema"
    CEAttrCorrelationID   = "correlationid"
)
```

Keep existing `CE*` constants (`CESpecVersion = "ce-specversion"`, etc.) — they remain correct for NATS/HTTP and serve as the canonical form inside `spec.Headers`.

Update the NOTE comment (lines 34-38) to reflect the new per-transport behavior:

```go
// NOTE: The "ce-" prefixed constants below are the canonical form used internally
// and match the HTTP/NATS CloudEvents binding convention. The AMQP transport uses
// "cloudEvents:" prefix on the wire per the AMQP binding spec, and normalizes
// incoming headers to "ce-" at the transport boundary so all spec functions work
// unchanged. See AMQPCEHeaderKey() and NormalizeCEHeaders().
```

### Step 2: Add AMQP header helper functions to spec/

**File**: `specification/spec/header_mapper.go` (new)

Two plain functions — no interface needed since only AMQP requires mapping:

```go
// AMQPCEHeaderKey returns the AMQP application-properties key for a bare
// CloudEvents attribute name, using the "cloudEvents:" prefix per the
// CloudEvents AMQP Protocol Binding specification.
// Example: AMQPCEHeaderKey("specversion") → "cloudEvents:specversion"
func AMQPCEHeaderKey(attr string) string {
    return "cloudEvents:" + attr
}

// NormalizeCEHeaders rewrites incoming transport headers so that all
// CloudEvents attributes use the canonical "ce-" prefix. This allows
// consumers to accept messages with any known prefix variant:
//   - "cloudEvents:specversion" → "ce-specversion"
//   - "cloudEvents_specversion" → "ce-specversion"  (JMS compat)
//   - "ce-specversion" → unchanged
//
// Non-CE headers are preserved unchanged. The original map is not modified;
// a new map is returned.
func NormalizeCEHeaders(h Headers) Headers {
    out := make(Headers, len(h))
    for k, v := range h {
        switch {
        case strings.HasPrefix(k, "cloudEvents:"):
            out["ce-"+strings.TrimPrefix(k, "cloudEvents:")] = v
        case strings.HasPrefix(k, "cloudEvents_"):
            out["ce-"+strings.TrimPrefix(k, "cloudEvents_")] = v
        default:
            out[k] = v
        }
    }
    return out
}
```

**File**: `specification/spec/header_mapper_test.go` (new) — unit tests for both functions.

### Step 3: Update AMQP publisher to use `cloudEvents:` prefix

**File**: `golang/amqp/setup_publisher.go` (lines 176-194)

First, normalize any user-supplied `ce-*` headers to `cloudEvents:*` so users passing `spec.CEID` as a custom header key still get the correct behavior:

```go
// Normalize user-supplied ce-* headers to cloudEvents:* for AMQP wire format.
// This ensures backward compat: users passing spec.CEID ("ce-id") as a custom
// header will have it correctly mapped to "cloudEvents:id" on the wire.
//
// NOTE: Deleting and inserting during range is safe per Go spec. The new
// "cloudEvents:*" keys won't match the "ce-" prefix guard, so no double-processing.
for k, v := range headers {
    if strings.HasPrefix(k, "ce-") {
        amqpKey := "cloudEvents:" + strings.TrimPrefix(k, "ce-")
        if _, exists := headers[amqpKey]; !exists {
            headers[amqpKey] = v
        }
        delete(headers, k)
    }
}
```

Then apply the defaults using AMQP-prefixed keys:

```go
setDefault := func(key, value string) {
    if _, exists := headers[key]; !exists {
        headers[key] = value
    }
}
setDefault(spec.AMQPCEHeaderKey(spec.CEAttrSpecVersion), spec.CESpecVersionValue)
setDefault(spec.AMQPCEHeaderKey(spec.CEAttrType), routingKey)
setDefault(spec.AMQPCEHeaderKey(spec.CEAttrSource), source)
setDefault(spec.AMQPCEHeaderKey(spec.CEAttrDataContentType), contentType)
setDefault(spec.AMQPCEHeaderKey(spec.CEAttrTime), time.Now().UTC().Format(time.RFC3339))

messageID := uuid.New().String()
amqpIDKey := spec.AMQPCEHeaderKey(spec.CEAttrID)
if existing, ok := headers[amqpIDKey].(string); ok && existing != "" {
    messageID = existing
}
headers[amqpIDKey] = messageID
```

### Step 4: Update AMQP consumer to normalize incoming headers

**File**: `golang/amqp/consumer.go` (in `handleDelivery`, before line 91)

Add normalization as the very first operation, **before** `HasCEHeaders` / `ValidateCEHeaders` / `MetadataFromHeaders`:

```go
func (c *queueConsumer) handleDelivery(handler wrappedHandler, delivery amqp.Delivery, deliveryInfo spec.DeliveryInfo) {
    // Normalize CE headers first: accept cloudEvents:*, cloudEvents_*, and ce-* prefixes.
    // IMPORTANT: This MUST happen before HasCEHeaders/ValidateCEHeaders/MetadataFromHeaders,
    // which all expect the canonical "ce-" prefix.
    deliveryInfo.Headers = spec.NormalizeCEHeaders(deliveryInfo.Headers)

    isLegacy := !spec.HasCEHeaders(deliveryInfo.Headers)
    // ... rest unchanged
}
```

**OTel tracing is unaffected**: `extractToContext(delivery.Headers, c.propagator)` at line 105 reads W3C trace context headers (`traceparent`, `tracestate`), not CE headers. The normalization operates on `deliveryInfo.Headers` (a `spec.Headers` copy), not on the raw `delivery.Headers` (`amqp.Table`), so OTel propagation is completely independent.

### Step 5: NATS transport — no changes needed

`ce-` is already the correct prefix per the NATS binding spec. No normalization is needed on consume since all NATS messages will use `ce-*` headers. No changes to `golang/nats/publisher.go` or `golang/nats/consumer.go`.

### Step 6: Update GAPS.md

**File**: `GAPS.md` (lines 55-61)

Mark the "CloudEvents Header Prefix Convention" gap as resolved.

## Ordering Dependency with Legacy Compat Plan (Task #5)

This plan and the legacy compat plan both touch `consumer.go` around line 90. The correct execution order is:

```
1. NormalizeCEHeaders (this plan)          → cloudEvents:* / cloudEvents_* / ce-* → ce-*
2. HasCEHeaders check (existing)           → detects ce-* headers
3. ValidateCEHeaders (existing)            → validates ce-* headers
4. MetadataFromHeaders (existing)          → extracts metadata from ce-* headers
5. Legacy enrichment (legacy compat plan)  → fills missing metadata fields
```

**This plan should be implemented first**, since normalization must happen before any CE header inspection. The legacy compat plan adds enrichment after metadata extraction (step 5) and does not conflict.

### Interaction with AMQP Channel Safety Plan (Task #1)

The channel safety plan moves exchange declarations to `setupChannel` and adds per-publisher channels. This CE header plan modifies the header logic inside `publishMessage`. These don't conflict — the CE plan changes *what headers* `publishMessage` sets, while the channel plan changes *which channel* `publishMessage` is called on. Either can be implemented first.

## Test Fixtures

`specification/spec/testdata/cloudevents.json` uses `ce-*` prefix throughout. **No fixture changes needed** — these test spec-level functions that operate on already-normalized headers (post-transport-boundary). The normalization itself is tested in `header_mapper_test.go`.

## Files Modified

| File | Change |
|------|--------|
| `specification/spec/cloudevents.go` | Add bare attribute constants, update NOTE comment |
| `specification/spec/header_mapper.go` (new) | `AMQPCEHeaderKey()` and `NormalizeCEHeaders()` plain functions |
| `specification/spec/header_mapper_test.go` (new) | Unit tests for mapping and normalization |
| `golang/amqp/setup_publisher.go` | Use `cloudEvents:` prefix on publish, normalize user `ce-*` headers |
| `golang/amqp/consumer.go` | Normalize headers before CE validation |
| `golang/amqp/connection_test.go` | Update `Test_Publish` (line 340-344): expect `cloudEvents:*` keys instead of `ce-*` |
| `golang/amqp/setup_publisher_test.go` | Update all header assertions (lines 193-203, 225-236, 250-257) to use `cloudEvents:*` keys |
| `golang/amqp/tracing_test.go` | Update test delivery headers (line 153): `ce-*` keys still work via consumer normalization — no change needed since this tests the consumer path which normalizes |
| `golang/amqp/consumer_test.go` | Existing tests use `ce-*` in delivery headers — still valid (consumer normalizes). Add new tests for `cloudEvents:*` and `cloudEvents_*` prefixed incoming messages |
| `GAPS.md` | Mark CE prefix gap as resolved |

## Affected Test Details

### Publisher tests — assertions MUST change (check outgoing headers)

These tests verify the wire format of published messages. After the change, outgoing AMQP headers use `cloudEvents:*` prefix.

**`connection_test.go:Test_Publish` (lines 340-344)**:
```go
// Before:
require.Equal(t, "1.0", msg.Headers[spec.CESpecVersion])      // "ce-specversion"
// After:
require.Equal(t, "1.0", msg.Headers[spec.AMQPCEHeaderKey(spec.CEAttrSpecVersion)])  // "cloudEvents:specversion"
```
All 5 assertions (lines 340-344) must be updated similarly.

**`setup_publisher_test.go` (lines 193-203, 225-257)**:
- Lines 193-203: Update assertions and delete calls to use `cloudEvents:*` keys
- Lines 225-236: Tests user-supplied `ce-*` headers — after normalization these become `cloudEvents:*` on the wire. Update assertions to expect `cloudEvents:*` keys. Also update the test input `Header{Key: spec.CEID, ...}` to verify the `ce-*` → `cloudEvents:*` remapping works
- Lines 250-257: Same — user-supplied `ce-type`/`ce-source` overrides must map to `cloudEvents:type`/`cloudEvents:source`

### Consumer tests — existing assertions still valid (check post-normalization)

These tests construct delivery headers with `ce-*` prefix. Since the consumer normalizes all prefix variants to `ce-*`, existing test assertions remain correct. The `ce-*` input goes through normalization as a no-op (already canonical).

**`consumer_test.go`**: All existing tests pass unchanged. Add new test cases with `cloudEvents:*` and `cloudEvents_*` input headers to verify normalization.

**`tracing_test.go` (line 153)**: Uses `spec.CEID` (`ce-id`) in delivery headers. Consumer normalizes this — no change needed.

## Test Plan

### Unit Tests (spec/)
1. `AMQPCEHeaderKey("specversion")` → `"cloudEvents:specversion"`
2. `AMQPCEHeaderKey("correlationid")` → `"cloudEvents:correlationid"`
3. `NormalizeCEHeaders` — `cloudEvents:type` → `ce-type`
4. `NormalizeCEHeaders` — `cloudEvents_type` → `ce-type` (JMS compat)
5. `NormalizeCEHeaders` — `ce-type` → `ce-type` (passthrough)
6. `NormalizeCEHeaders` preserves non-CE headers unchanged
7. `NormalizeCEHeaders` on empty headers returns empty
8. `NormalizeCEHeaders` with mixed prefixes normalizes all (defensive — shouldn't happen per spec)
9. Verify `HasCEHeaders`, `MetadataFromHeaders`, `ValidateCEHeaders` work correctly on normalized output

### Unit Tests (amqp/)
1. Publisher emits `cloudEvents:specversion`, `cloudEvents:type`, etc.
2. Publisher respects user-supplied `cloudEvents:id` override
3. Publisher remaps user-supplied `ce-id` to `cloudEvents:id` (backward compat)
4. Publisher remaps user-supplied `ce-source` without clobbering existing `cloudEvents:source`
5. Consumer handles messages with `cloudEvents:*` headers (new format)
6. Consumer handles messages with `ce-*` headers (old format — backward compat)
7. Consumer handles messages with `cloudEvents_*` headers (JMS compat)
8. Consumer handles messages with no CE headers (legacy — unchanged behavior)

### Unit Tests (nats/)
1. Existing tests pass unchanged (`ce-` prefix is correct for NATS)

### Integration/Interop Testing
1. Run `make tests` (requires RabbitMQ) — verifies full publish/consume roundtrip with new AMQP prefix
2. Manual test: publish with old `ce-` prefix, consume with new code → verify backward compat
3. Manual test: publish with new `cloudEvents:` prefix, consume with new code → verify new format

## Backward Compatibility / Migration

**Rolling deployment safe**: During a deployment where old publishers (emitting `ce-*`) and new consumers (expecting `cloudEvents:*`) coexist:
- New consumers normalize both `ce-*` and `cloudEvents:*` to `ce-*` internally → all messages work
- Old consumers only understand `ce-*` → messages from new publishers (with `cloudEvents:*`) will have invalid CE headers for old consumers

**Migration strategy**:
1. Deploy consumers first (they accept both prefixes)
2. Deploy publishers second (they emit new prefix)
3. No message loss, no reprocessing needed

**Existing messages in queues**: Messages already enqueued with `ce-*` prefix will be consumed correctly by the new consumer normalization logic.

## Verification

```bash
# Spec module tests
cd specification && go test -race -count=1 ./spec/...

# AMQP transport tests
cd golang && go test -race -count=1 ./amqp/...

# NATS transport tests (should pass unchanged)
cd golang && go test -race -count=1 ./nats/...

# Integration tests (requires RabbitMQ)
make tests
```
