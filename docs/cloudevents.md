# CloudEvents

All messages in messaging carry [CloudEvents 1.0](https://cloudevents.io/) metadata. This provides a standardized envelope for event attributes — who sent it, what type it is, when it happened — independent of the transport.

## Wire Format

messaging uses **binary content mode**: CloudEvents attributes are transported as message headers, and the event data is the message body (JSON-encoded).

```
Headers:
  ce-specversion:     1.0
  ce-type:            Order.Created
  ce-source:          order-service
  ce-id:              550e8400-e29b-41d4-a716-446655440000
  ce-time:            2026-03-05T14:30:00Z
  ce-datacontenttype: application/json

Body:
  {"orderId": "abc-123", "amount": 42.00}
```

Binary content mode is preferred over structured mode because it avoids wrapping the payload in a CloudEvents JSON envelope, keeping messages compatible with consumers that don't know about CloudEvents.

## Required Attributes

These headers are set automatically on every published message:

| Header | Value | Description |
|--------|-------|-------------|
| `ce-specversion` | `1.0` | CloudEvents spec version (always `1.0`) |
| `ce-type` | routing key | Event type, e.g., `Order.Created` |
| `ce-source` | service name | Publishing service, e.g., `order-service` |
| `ce-id` | UUID v4 | Unique message identifier |
| `ce-time` | RFC 3339 UTC | When the event was produced |

## Optional Attributes

| Header | Description |
|--------|-------------|
| `ce-datacontenttype` | Media type of the body (`application/json`) |
| `ce-subject` | Additional context about the event subject |
| `ce-dataschema` | URI of the payload schema |
| `ce-correlationid` | Extension attribute for request-response correlation |

## AMQP Header Prefix

The [CloudEvents AMQP binding](https://github.com/cloudevents/spec/blob/main/cloudevents/bindings/amqp-protocol-binding.md) specifies using the `cloudEvents:` prefix for application properties. messaging follows this convention:

| Context | Header format |
|---------|---------------|
| AMQP publish | `cloudEvents:specversion`, `cloudEvents:type`, etc. |
| AMQP consume | Normalized to `ce-specversion`, `ce-type`, etc. |
| NATS | `ce-specversion`, `ce-type`, etc. (no prefix change) |

### Prefix Normalization

On the consumer side, the spec normalizes all known prefix variants to the canonical `ce-` form:

| Wire format | Normalized to |
|-------------|---------------|
| `cloudEvents:type` | `ce-type` |
| `cloudEvents_type` | `ce-type` (JMS compatibility) |
| `ce-type` | `ce-type` (already canonical) |

This normalization happens at the transport boundary, so handler code always sees `ce-` prefixed headers regardless of the broker.

## Validation

`ValidateCEHeaders()` checks that required CloudEvents attributes are present and are string values. It returns a list of warnings (empty list = valid).

```go
import "github.com/sparetimecoders/messaging"

warnings := messaging.ValidateCEHeaders(headers)
// warnings: ["missing required CloudEvents attribute: ce-source"]
```

```typescript
import { validateCEHeaders } from "@gomessaging/spec";

const warnings = validateCEHeaders(headers);
```

Validation is advisory — messages with missing CE headers are still delivered. This allows gradual migration from legacy systems.

## Legacy Message Support

Messages from systems that don't set CloudEvents headers are handled gracefully through **metadata enrichment**:

```go
metadata := messaging.EnrichLegacyMetadata(metadata, deliveryInfo, uuidGenerator)
```

Enrichment fills in missing fields from the delivery context:

| Missing field | Enriched from |
|---------------|---------------|
| `ce-id` | Generated UUID v4 |
| `ce-time` | Current UTC timestamp |
| `ce-type` | Routing key from delivery info |
| `ce-source` | Source exchange/subject from delivery info |
| `ce-specversion` | `1.0` |
| `ce-datacontenttype` | `application/json` |

The `HasCEHeaders()` function distinguishes between:
- **Legacy messages** (no CE headers at all) — silently enriched
- **Malformed CE messages** (some headers present but incomplete) — enriched + warnings logged

## Metadata Access

On the consumer side, CloudEvents attributes are parsed into a structured `Metadata` object:

```go
func handler(ctx context.Context, e messaging.ConsumableEvent[OrderCreated]) error {
    e.ID              // "550e8400-e29b-41d4-a716-446655440000"
    e.Type            // "Order.Created"
    e.Source          // "order-service"
    e.Timestamp       // time.Time
    e.SpecVersion     // "1.0"
    e.CorrelationID   // "" (empty if not set)
    e.Payload         // OrderCreated{...}
    return nil
}
```

```typescript
conn.addEventConsumer<OrderCreated>("Order.Created", async (event) => {
  event.id;             // "550e8400-..."
  event.type;           // "Order.Created"
  event.source;         // "order-service"
  event.timestamp;      // Date
  event.payload;        // { orderId: "abc-123", amount: 42 }
});
```
