# Implementing a Transport

This guide walks through building a conformant gomessaging transport from scratch — in any language, for any message broker.

## What You're Building

A transport library that:
1. Exposes the gomessaging API (publishers, consumers, connection lifecycle)
2. Maps the five messaging patterns to broker primitives
3. Uses deterministic naming for all broker resources
4. Attaches CloudEvents metadata to every message
5. Exports topology for validation and visualization
6. Passes the TCK

## Step 1: Port the Spec

Start by implementing the naming functions and core types from the spec module. You can either:

- **Translate from source**: Port the Go or TypeScript spec code directly
- **Use the JSON fixtures**: Load [`testdata/naming.json`](../testdata/naming.json) as your test oracle and implement functions that produce matching outputs

### Naming Functions to Implement

| Function | Example |
|----------|---------|
| `topicExchangeName("events")` | `"events.topic.exchange"` |
| `serviceEventQueueName("events.topic.exchange", "orders")` | `"events.topic.exchange.queue.orders"` |
| `serviceRequestExchangeName("billing")` | `"billing.direct.exchange.request"` |
| `serviceResponseExchangeName("billing")` | `"billing.headers.exchange.response"` |
| `serviceRequestQueueName("billing")` | `"billing.direct.exchange.request.queue"` |
| `serviceResponseQueueName("billing", "orders")` | `"billing.headers.exchange.response.queue.orders"` |

For NATS additionally:

| Function | Example |
|----------|---------|
| `natsStreamName("audit.topic.exchange")` | `"audit"` |
| `natsSubject("events", "Order.Created")` | `"events.Order.Created"` |
| `translateWildcard("Order.#")` | `"Order.>"` |

### Types to Define

- `Transport`, `EndpointDirection`, `ExchangeKind`, `Pattern` (enums)
- `Endpoint`, `Topology` (structures)
- `Metadata`, `DeliveryInfo`, `ConsumableEvent<T>` (message types)
- `EventHandler<T>`, `RequestResponseEventHandler<T, R>` (handler signatures)

## Step 2: Pass Fixture Tests

Load each JSON fixture file and verify your implementation produces identical outputs:

| Fixture | What to test |
|---------|-------------|
| [`naming.json`](../testdata/naming.json) | All naming functions match expected outputs |
| [`validate.json`](../testdata/validate.json) | Validation rules produce expected errors |
| [`topology.json`](../testdata/topology.json) | Setup intents generate correct endpoints |
| [`cloudevents.json`](../testdata/cloudevents.json) | CE header validation and metadata extraction |
| [`constants.json`](../testdata/constants.json) | Enum values match |

The `spectest` package (Go) and the test files in `typescript/__tests__/` show how to load and assert against fixtures.

## Step 3: Map Patterns to Broker Primitives

Each pattern maps to specific broker resources. Your transport creates these during `Start()`:

### AMQP Mapping

| Pattern | Exchange | Queue | Binding |
|---------|----------|-------|---------|
| event-stream | `events.topic.exchange` (topic) | `events.topic.exchange.queue.{service}` | routing key |
| custom-stream | `{name}.topic.exchange` (topic) | `{name}.topic.exchange.queue.{service}` | routing key |
| service-request | `{service}.direct.exchange.request` (direct) | `{service}.direct.exchange.request.queue` | routing key |
| service-response | `{service}.headers.exchange.response` (headers) | `{target}.headers.exchange.response.queue.{caller}` | header match |
| queue-publish | default exchange | `{queueName}` | — |

**AMQP considerations:**
- Use quorum queues for durability
- Enable publisher confirms
- Set dead letter exchanges for poison message handling

### NATS Mapping

| Pattern | Stream | Subject | Consumer |
|---------|--------|---------|----------|
| event-stream | `events` | `events.{routingKey}` | durable pull consumer `{service}` |
| custom-stream | `{name}` | `{name}.{routingKey}` | durable pull consumer `{service}` |
| service-request | — | Core NATS request-reply | — |
| service-response | — | Core NATS reply subject | — |
| queue-publish | — | Core NATS publish | — |

**NATS considerations:**
- Create JetStream streams with retention limits
- Configure `MaxDeliver` and `BackOff` for consumer retry
- Use Core NATS (not JetStream) for request-reply

## Step 4: Add CloudEvents

### On Publish

Set these headers on every outgoing message:

| Header | Value |
|--------|-------|
| `ce-specversion` / `cloudEvents:specversion` | `"1.0"` |
| `ce-type` / `cloudEvents:type` | routing key |
| `ce-source` / `cloudEvents:source` | service name |
| `ce-id` / `cloudEvents:id` | UUID v4 |
| `ce-time` / `cloudEvents:time` | RFC 3339 UTC now |
| `ce-datacontenttype` / `cloudEvents:datacontenttype` | `"application/json"` |

Use `cloudEvents:` prefix for AMQP, `ce-` prefix for NATS and everything else.

### On Consume

1. Normalize header prefixes to `ce-` (handle `cloudEvents:`, `cloudEvents_`, and `ce-` variants)
2. Parse headers into a `Metadata` struct
3. Run `ValidateCEHeaders()` and log warnings for missing attributes
4. If no CE headers present (legacy message), enrich with synthetic metadata from delivery context

## Step 5: Add Observability

### OpenTelemetry Tracing

Create spans for publish and consume operations:

```
Span attributes:
  messaging.system       = "amqp" | "nats"
  messaging.operation    = "publish" | "receive"
  messaging.destination.name = exchange or stream name
```

Propagate trace context through message headers using `TextMapPropagator`.

### Prometheus Metrics

Register these metrics (prefix with transport name):

| Metric | Type |
|--------|------|
| `{transport}_events_received` | counter |
| `{transport}_events_ack` | counter |
| `{transport}_events_nack` | counter |
| `{transport}_events_processed_duration` | histogram |
| `{transport}_events_publish_succeed` | counter |
| `{transport}_events_publish_failed` | counter |
| `{transport}_events_publish_duration` | histogram |

Use `WithRoutingKeyMapper()` to control label cardinality — map high-cardinality routing keys to buckets.

## Step 6: Export Topology

Implement a `Topology()` method that returns the service's declared endpoints without connecting to a broker:

```go
func (c *Connection) Topology() spec.Topology {
    return spec.Topology{
        Transport:   spec.TransportAMQP,
        ServiceName: c.serviceName,
        Endpoints:   c.endpoints,
    }
}
```

This enables `specverify` and `spec.Mermaid()` to work with your transport.

## Step 7: Write a TCK Adapter

The final step: prove conformance by passing the TCK.

1. Implement the `ServiceManager` interface (or JSON-RPC protocol directly)
2. Build an adapter binary
3. Run `tck-runner --adapter ./your-adapter`

See the [TCK Guide](tck.md) for protocol details and the `adapterutil` package.

## Checklist

| | Requirement | Verified by |
|---|-------------|-------------|
| | Naming functions produce correct outputs | Fixture tests |
| | Validation catches structural errors | Fixture tests |
| | Topology correctly generates endpoints from setup intents | Fixture tests |
| | CloudEvents headers set on publish | TCK Phase 4 |
| | CloudEvents headers normalized on consume | TCK Phase 4 |
| | Legacy messages enriched | TCK Phase 4 |
| | Broker resources actually created | TCK Phase 3 |
| | Messages delivered to correct consumers | TCK Phase 4 |
| | Wire format compatible (probe messages work) | TCK Phase 5 |
| | Topology export works | TCK Phase 2 |
| | OpenTelemetry spans created | Manual verification |
| | Prometheus metrics registered | Manual verification |

## Reference Implementations

Study the existing transports for patterns:

| Transport | Language | Repo |
|-----------|----------|------|
| AMQP | Go | [go-messaging-amqp](https://github.com/sparetimecoders/go-messaging-amqp) |
| NATS | Go | [go-messaging-nats](https://github.com/sparetimecoders/go-messaging-nats) |
| AMQP | TypeScript | [nodejs-messaging-amqp](https://github.com/sparetimecoders/nodejs-messaging-amqp) |
| NATS | TypeScript | [nodejs-messaging-nats](https://github.com/sparetimecoders/nodejs-messaging-nats) |
