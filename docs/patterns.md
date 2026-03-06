# Communication Patterns

messaging supports five messaging patterns. Each pattern maps to specific broker primitives (exchanges and queues in AMQP, streams and subjects in NATS) using [deterministic naming](naming.md).

## Overview

| Pattern | Style | Use Case |
|---------|-------|----------|
| [Event Stream](#event-stream) | Pub/sub | Domain events, notifications, audit trails |
| [Custom Stream](#custom-stream) | Pub/sub | Isolated event domains (audit, telemetry) |
| [Service Request](#service-request) | RPC | Synchronous queries between services |
| [Service Response](#service-response) | RPC | Reply channel for service requests |
| [Queue Publish](#queue-publish) | Point-to-point | Work queues, task distribution |

## Event Stream

The default and most common pattern. Publish domain events to a shared topic exchange; any number of services subscribe by routing key.

```
                              ┌─────────────────┐
orders ──publish──>           │ events          │ ──Order.Created──> notifications
                              │ .topic.exchange │ ──Order.Created──> analytics
                              │                 │ ──Order.*───────> audit
                              └─────────────────┘
```

### Broker Mapping

| | AMQP | NATS |
|---|------|------|
| **Exchange/Stream** | `events.topic.exchange` (topic) | `events` (JetStream stream) |
| **Publish to** | Exchange with routing key | Subject `events.Order.Created` |
| **Consumer** | Durable queue `events.topic.exchange.queue.{service}` | Durable consumer `{service}` |
| **Routing** | AMQP topic routing (`*` single, `#` multi) | NATS wildcards (`*` single, `>` multi) |

### Consumer Types

**Durable consumers** survive restarts. In AMQP, these use quorum queues. In NATS, durable pull consumers.

**Transient consumers** auto-delete on disconnect. Useful for temporary subscriptions, live dashboards, or debugging.

### Wildcard Routing

Subscribe to multiple event types with wildcards:

| Pattern | Matches |
|---------|---------|
| `Order.Created` | Exactly `Order.Created` |
| `Order.*` | `Order.Created`, `Order.Updated`, `Order.Deleted` |
| `Order.#` (AMQP) / `Order.>` (NATS) | `Order.Created`, `Order.Item.Added`, any depth |
| `#` (AMQP) / `>` (NATS) | Everything |

The spec handles wildcard translation between transports automatically.

## Custom Stream

Same as event-stream but on a named exchange instead of the default `events` exchange. Use this when events belong to a separate domain.

```
                              ┌─────────────────┐
audit-writer ──publish──>     │ audit           │ ──*──> audit-reader
                              │ .topic.exchange │
                              └─────────────────┘
```

### When to Use

- Events that don't belong in the main `events` stream
- Separate retention policies (e.g., audit logs kept longer)
- Isolation between bounded contexts
- High-throughput streams that shouldn't compete with general events

## Service Request

Synchronous request-reply between services. The caller sends a request and waits for a response.

```
                     ┌────────────────────────────────────┐
caller ──request──>  │ billing.direct.exchange.request    │ ──> billing handler
                     └────────────────────────────────────┘          │
                     ┌────────────────────────────────────┐          │
caller <──response── │ billing.headers.exchange.response  │ <───────┘
                     └────────────────────────────────────┘
```

### Broker Mapping

| | AMQP | NATS |
|---|------|------|
| **Request exchange** | `{service}.direct.exchange.request` (direct) | Core NATS request-reply |
| **Request queue** | `{service}.direct.exchange.request.queue` | — |
| **Response exchange** | `{service}.headers.exchange.response` (headers) | Built-in reply subject |
| **Response queue** | `{service}.headers.exchange.response.queue.{caller}` | — |

In NATS, request-reply uses built-in response routing — no explicit response exchange needed.

## Service Response

The response half of a service request. This is declared automatically when you set up a request handler — you rarely configure it directly.

The response exchange uses `headers` type in AMQP, which routes replies back to the correct caller based on header matching.

## Queue Publish

Direct publish to a named queue. The sender picks the destination explicitly. No exchange routing involved.

```
task-creator ──publish──> [email-queue] ──> email-worker-1
                                        ──> email-worker-2
```

Use for work queues where:
- The sender knows the destination
- Multiple workers compete for messages (competing consumers)
- No fan-out needed

## Choosing a Pattern

```
Do multiple services need this event?
  ├── Yes → Event Stream (or Custom Stream for isolated domains)
  └── No
       ├── Need a response? → Service Request/Response
       └── Fire-and-forget to a specific queue? → Queue Publish
```

## Pattern Comparison

| | Fan-out | Response | Routing | Durability |
|---|---------|----------|---------|------------|
| **Event Stream** | Yes | No | Topic wildcards | Durable or transient |
| **Custom Stream** | Yes | No | Topic wildcards | Durable or transient |
| **Service Request** | No | Yes | Direct | Durable |
| **Queue Publish** | No | No | None (direct queue) | Durable |
