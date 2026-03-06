# Naming Conventions

All resource names in messaging are derived deterministically from the service name and messaging pattern. This enables services to discover each other without configuration, and makes topologies fully reconstructable from declarations alone.

## Design Principle

A service named `"orders"` consuming `"Order.Created"` events will always connect to the same exchange and queue, regardless of transport, language, or deployment environment. There is no configuration file mapping services to broker resources — the names **are** the configuration.

## AMQP Naming

### Exchanges

Exchanges follow the pattern `{name}.{kind}.exchange`:

| Pattern | Exchange Name | Kind |
|---------|--------------|------|
| Event stream | `events.topic.exchange` | topic |
| Custom stream | `{name}.topic.exchange` | topic |
| Service request | `{service}.direct.exchange.request` | direct |
| Service response | `{service}.headers.exchange.response` | headers |

The default event stream always uses `events` as the name.

### Queues

Queues encode both the exchange and the consuming service:

| Pattern | Queue Name | Example |
|---------|-----------|---------|
| Event consumer | `{exchange}.queue.{service}` | `events.topic.exchange.queue.notifications` |
| Request inbox | `{service}.direct.exchange.request.queue` | `billing.direct.exchange.request.queue` |
| Response inbox | `{target}.headers.exchange.response.queue.{caller}` | `billing.headers.exchange.response.queue.orders` |

### Transient Queues

Transient (ephemeral) consumers append a unique suffix to the queue name, so each instance gets its own auto-deleting queue:

```
events.topic.exchange.queue.dashboard-a1b2c3d4
```

This prevents competing-consumer behavior for transient subscribers.

## NATS Naming

NATS uses a flatter naming scheme without the `.topic.exchange` suffix:

| Resource | Pattern | Example |
|----------|---------|---------|
| Stream | `{name}` | `events` |
| Subject | `{stream}.{routingKey}` | `events.Order.Created` |
| Consumer | `{service}` | `notifications` |

### Stream Name Derivation

When converting from the spec's exchange-centric model to NATS, the stream name is extracted by stripping the `.topic.exchange` suffix:

```
events.topic.exchange  →  events
audit.topic.exchange   →  audit
```

### Wildcard Translation

AMQP and NATS use different wildcard syntax. The spec translates automatically:

| Meaning | AMQP | NATS |
|---------|------|------|
| Match one segment | `*` | `*` |
| Match one or more segments | `#` | `>` |

Examples:
- `Order.*` matches `Order.Created`, `Order.Updated` (both transports)
- `Order.#` (AMQP) = `Order.>` (NATS) matches `Order.Created`, `Order.Item.Added`

## Naming Functions

The messaging library exports these functions in both Go and TypeScript:

| Function | Input | Output | Example |
|----------|-------|--------|---------|
| `TopicExchangeName` | name | `{name}.topic.exchange` | `"events"` → `"events.topic.exchange"` |
| `ServiceEventQueueName` | exchange, service | `{exchange}.queue.{service}` | `"events.topic.exchange", "orders"` → `"events.topic.exchange.queue.orders"` |
| `ServiceRequestExchangeName` | service | `{service}.direct.exchange.request` | `"billing"` → `"billing.direct.exchange.request"` |
| `ServiceResponseExchangeName` | service | `{service}.headers.exchange.response` | `"billing"` → `"billing.headers.exchange.response"` |
| `ServiceRequestQueueName` | service | `{service}.direct.exchange.request.queue` | `"billing"` → `"billing.direct.exchange.request.queue"` |
| `ServiceResponseQueueName` | target, service | `{target}.headers.exchange.response.queue.{service}` | `"billing", "orders"` → `"billing.headers.exchange.response.queue.orders"` |
| `NATSStreamName` | exchange name | stream name (strip suffix) | `"audit.topic.exchange"` → `"audit"` |
| `NATSSubject` | stream, routing key | `{stream}.{routingKey}` | `"events", "Order.Created"` → `"events.Order.Created"` |
| `TranslateWildcard` | routing key | NATS-compatible key | `"Order.#"` → `"Order.>"` |

## Validation Rules

The spec validates that names follow transport-specific conventions:

### AMQP
- Topic exchanges must end with `.topic.exchange`
- Direct exchanges must end with `.direct.exchange.request`
- Headers exchanges must end with `.headers.exchange.response`

### NATS
- Stream names must not contain AMQP suffixes (`.topic.exchange`, etc.)
- Names must not contain whitespace

See [Topology Tools](topology.md) for how to run validation.

## Examples

### Three Services, One Broker

Given services `orders`, `notifications`, and `analytics` all using the default event stream:

**AMQP resources created:**

```
Exchanges:
  events.topic.exchange (topic)

Queues:
  events.topic.exchange.queue.orders
  events.topic.exchange.queue.notifications
  events.topic.exchange.queue.analytics

Bindings:
  events.topic.exchange → Order.Created → events.topic.exchange.queue.notifications
  events.topic.exchange → Order.Created → events.topic.exchange.queue.analytics
  events.topic.exchange → Order.*       → events.topic.exchange.queue.orders
```

**NATS resources created:**

```
Stream:
  events (subjects: events.>)

Consumers:
  notifications (filter: events.Order.Created)
  analytics     (filter: events.Order.Created)
  orders        (filter: events.Order.*)
```

### Request-Response Between Two Services

`orders` calls `billing` to look up an invoice:

**AMQP resources created:**

```
Exchanges:
  billing.direct.exchange.request (direct)
  billing.headers.exchange.response (headers)

Queues:
  billing.direct.exchange.request.queue
  billing.headers.exchange.response.queue.orders
```
