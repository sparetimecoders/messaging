# Specification Reference

This document is the formal specification for messaging. Transport implementations must conform to these rules to pass the [TCK](tck.md).

## Core Types

### Transport

```
Transport = "amqp" | "nats"
```

### Endpoint Direction

```
EndpointDirection = "publish" | "consume"
```

### Exchange Kind

```
ExchangeKind = "topic" | "direct" | "headers"
```

### Pattern

```
Pattern = "event-stream" | "custom-stream" | "service-request" | "service-response" | "queue-publish"
```

### Endpoint

An endpoint describes a single exchange/queue binding declared by a service:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `direction` | EndpointDirection | Yes | Publish or consume |
| `pattern` | Pattern | Yes | Messaging pattern |
| `exchangeName` | string | Yes | Exchange or stream name |
| `exchangeKind` | ExchangeKind | Yes | Exchange type |
| `queueName` | string | Consume only | Queue or consumer name |
| `routingKey` | string | Conditional | Routing key (not required for headers exchanges) |
| `messageType` | string | No | Name of the message payload type |
| `ephemeral` | bool | No | Transient consumer (auto-delete) |

### Topology

A topology groups all endpoints for a single service:

| Field | Type | Description |
|-------|------|-------------|
| `transport` | Transport | AMQP or NATS |
| `serviceName` | string | Unique service identifier |
| `endpoints` | []Endpoint | All declared endpoints |

### Metadata (CloudEvents)

Structured representation of CloudEvents attributes:

| Field | Type | CE Attribute | Description |
|-------|------|--------------|-------------|
| `id` | string | `ce-id` | Unique message ID (UUID v4) |
| `type` | string | `ce-type` | Event type (routing key) |
| `source` | string | `ce-source` | Publishing service name |
| `timestamp` | time | `ce-time` | RFC 3339 UTC |
| `specVersion` | string | `ce-specversion` | Always `"1.0"` |
| `dataContentType` | string | `ce-datacontenttype` | `"application/json"` |
| `subject` | string | `ce-subject` | Event subject (optional) |
| `correlationID` | string | `ce-correlationid` | Request-response correlation |

### DeliveryInfo

Transport-agnostic delivery context provided to consumers:

| Field | Type | Description |
|-------|------|-------------|
| `destination` | string | Queue or consumer name |
| `source` | string | Exchange or subject |
| `key` | string | Routing key |
| `headers` | map | All message headers |

### ConsumableEvent\<T\>

The message object delivered to handler functions:

| Field | Type | Description |
|-------|------|-------------|
| (embedded) | Metadata | All CloudEvents attributes |
| (embedded) | DeliveryInfo | Delivery context |
| `payload` | T | Deserialized message body |

## Naming Rules

### MUST (required for conformance)

1. Topic exchanges MUST be named `{name}.topic.exchange`
2. Event queues MUST be named `{exchange}.queue.{service}`
3. Request exchanges MUST be named `{service}.direct.exchange.request`
4. Request queues MUST be named `{service}.direct.exchange.request.queue`
5. Response exchanges MUST be named `{service}.headers.exchange.response`
6. Response queues MUST be named `{target}.headers.exchange.response.queue.{caller}`
7. NATS stream names MUST NOT contain AMQP suffixes
8. NATS subjects MUST be `{stream}.{routingKey}`
9. Wildcard translation: AMQP `#` MUST map to NATS `>`

See [Naming Conventions](naming.md) for the full function reference.

## CloudEvents Rules

### MUST

1. All published messages MUST include `ce-specversion`, `ce-type`, `ce-source`, `ce-id`, `ce-time`
2. `ce-specversion` MUST be `"1.0"`
3. `ce-id` MUST be a UUID v4
4. `ce-time` MUST be RFC 3339 UTC
5. `ce-type` MUST be the routing key
6. `ce-source` MUST be the publishing service name
7. Binary content mode MUST be used (attributes as headers, data as body)
8. Body MUST be JSON-encoded

### MUST (AMQP-specific)

9. Published headers MUST use the `cloudEvents:` prefix
10. Consumed headers MUST be normalized to `ce-` prefix
11. `cloudEvents_` prefix (JMS) MUST also be normalized to `ce-`

### SHOULD

12. `ce-datacontenttype` SHOULD be set to `"application/json"`
13. Legacy messages (no CE headers) SHOULD be enriched transparently
14. Malformed CE messages (partial headers) SHOULD generate warnings

See [CloudEvents](cloudevents.md) for detailed behavior.

## Validation Rules

### Single Service

1. Service name MUST NOT be empty
2. Exchange name MUST NOT be empty
3. Consume endpoints MUST have a queue name (unless ephemeral)
4. Routing key MUST be present for topic and direct exchanges
5. AMQP naming conventions MUST be followed (exchange suffixes match kind)
6. NATS names MUST NOT contain AMQP suffixes or whitespace

### Cross-Service

7. Consumer routing keys (exact match, no wildcards) MUST have a matching publisher on the same exchange
8. Topologies on different transports MUST NOT be cross-validated (they can't communicate)

See [Topology Tools](topology.md) for how to run validation.

## Routing Rules

1. `*` matches exactly one dot-separated segment
2. `#` (AMQP) / `>` (NATS) matches one or more segments
3. Exact routing keys match only themselves
4. Empty routing key matches only empty routing key

## Handler Contracts

### Event Handler

```
EventHandler<T>(ctx, ConsumableEvent<T>) → error
```

- Return `nil`/`null`: message acknowledged, removed from queue
- Return error: message rejected and requeued for retry

### Request-Response Handler

```
RequestResponseEventHandler<T, R>(ctx, ConsumableEvent<T>) → (R, error)
```

- Return `(response, nil)`: response sent to caller, request acknowledged
- Return `(_, error)`: request rejected, no response sent

### Deserialization Failures

- If the message body cannot be deserialized to type `T`: message MUST be rejected permanently (no requeue)

## Observability Contracts

### SHOULD (OpenTelemetry)

1. Consumer and publisher spans with `messaging.system`, `messaging.operation`, `messaging.destination.name`
2. Context propagation via `TextMapPropagator`

### SHOULD (Prometheus)

| Metric | Type |
|--------|------|
| `{transport}_events_received` | counter |
| `{transport}_events_ack` | counter |
| `{transport}_events_nack` | counter |
| `{transport}_events_processed_duration` | histogram |
| `{transport}_events_publish_succeed` | counter |
| `{transport}_events_publish_failed` | counter |
| `{transport}_events_publish_duration` | histogram |

## Conformance Levels

| Level | Requirements |
|-------|-------------|
| **MUST** | Naming, validation, CE headers, JSON encoding, handler contracts |
| **SHOULD** | Optional CE headers, topology export, observability, broker state verification |

A transport is **conformant** when it passes all TCK scenarios, which exercise MUST-level requirements.
