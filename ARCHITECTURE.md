# gomessaging Architecture

gomessaging is a multi-transport messaging library with a shared specification layer. It provides consistent naming conventions, topology validation, CloudEvents support, and observability across different message brokers.

## Repository Structure

The project is split across five repositories:

| Repository | Contents | Module / Package |
|------------|----------|------------------|
| [`messaging`](https://github.com/sparetimecoders/messaging) (this repo) | Go + TypeScript shared library, TCK, testdata, specverify, docs | `github.com/sparetimecoders/messaging`, `@gomessaging/spec` |
| [`go-messaging-amqp`](https://github.com/sparetimecoders/go-messaging-amqp) | Go AMQP transport + TCK adapter | `github.com/sparetimecoders/go-messaging-amqp` |
| [`go-messaging-nats`](https://github.com/sparetimecoders/go-messaging-nats) | Go NATS transport + TCK adapter | `github.com/sparetimecoders/go-messaging-nats` |
| [`nodejs-messaging-amqp`](https://github.com/sparetimecoders/nodejs-messaging-amqp) | Node AMQP transport + TCK adapter | `@gomessaging/amqp` |
| [`nodejs-messaging-nats`](https://github.com/sparetimecoders/nodejs-messaging-nats) | Node NATS transport + TCK adapter | `@gomessaging/nats` |

### This Repo

```
.
├── *.go                  Go shared library (naming, topology, validation, CloudEvents, visualization)
├── spectest/             Conformance test helpers and assertion functions
├── tck/                  Technology Compatibility Kit (runner, protocol, broker access)
│   ├── adapterutil/      Reusable JSON-RPC handler for adapter implementations
│   └── cmd/tck-runner/   TCK CLI runner
├── specverify/           CLI for topology validation and visualization
├── testdata/             Shared JSON test fixtures (canonical specification)
└── typescript/           TypeScript shared messaging library (mirrors Go)
```

### Dependency Graph

```
messaging (spec + tck)
  ├── go-messaging-amqp     (Go module dep on spec + tck)
  ├── go-messaging-nats     (Go module dep on spec + tck)
  ├── nodejs-messaging-amqp (npm dep on @gomessaging/spec)
  └── nodejs-messaging-nats (npm dep on @gomessaging/spec)
```

The `messaging` module has zero transport dependencies. Transport modules import `messaging` for naming functions, types, and validation. This separation allows any language to implement a conformant transport by following the spec.

## Communication Patterns

gomessaging supports five messaging patterns:

| Pattern | Exchange Kind | Use Case |
|---------|--------------|----------|
| **event-stream** | topic | Publish domain events to the default `events` exchange; multiple services subscribe by routing key |
| **custom-stream** | topic | Same as event-stream but on a named exchange (e.g. `audit`) |
| **service-request** | direct | Send RPC-style requests to a specific service |
| **service-response** | headers | Receive RPC responses from a target service |
| **queue-publish** | (direct) | Publish directly to a named queue (sender-selected distribution) |

### Event Stream (Pub/Sub)

The most common pattern. A service publishes events to a topic exchange. Other services create durable queues bound with routing key filters.

```
Producer ──publish──> [events.topic.exchange] ──routing key──> Queue ──> Consumer
                                               ──routing key──> Queue ──> Consumer
```

- **Durable consumers** survive restarts (quorum queues, 5-day TTL in AMQP; durable consumers in NATS)
- **Transient consumers** auto-delete on disconnect (UUID-suffixed queues in AMQP; ephemeral subscriptions in NATS)
- **Wildcard routing**: `Order.*` matches `Order.Created`, `Order.Updated`; `Order.#` matches any depth

### Service Request-Response (RPC)

For synchronous request-reply between services:

```
Caller ──publish──> [target.direct.exchange.request] ──> Request Queue ──> Handler
                                                                              │
Caller <──consume── [target.headers.exchange.response] <──publish────────────┘
```

In NATS, this uses Core NATS request-reply with built-in response routing.

## Naming Conventions

All exchange and queue names follow deterministic patterns derived from the service name and pattern type. This enables topology discovery, validation, and visualization without additional configuration.

### AMQP Names

| Resource | Pattern |
|----------|---------|
| Topic exchange | `{name}.topic.exchange` |
| Event queue | `{exchange}.queue.{service}` |
| Request exchange | `{service}.direct.exchange.request` |
| Request queue | `{service}.direct.exchange.request.queue` |
| Response exchange | `{service}.headers.exchange.response` |
| Response queue | `{targetService}.headers.exchange.response.queue.{service}` |

### NATS Names

NATS uses simplified names. The `NATSStreamName()` function strips the `.topic.exchange` suffix when present:

| Resource | Pattern |
|----------|---------|
| Stream name | `{name}` (base name without AMQP suffix) |
| Subject | `{stream}.{routingKey}` |
| Consumer name | `{service}` |
| Request subject | `{service}.request.{routingKey}` |

Wildcard translation: AMQP `#` (multi-level) maps to NATS `>`.

## CloudEvents

All messages carry [CloudEvents 1.0](https://cloudevents.io/) metadata in binary content mode (attributes as headers, payload as body). Required headers are set automatically on publish:

| Header | Auto-set Value |
|--------|---------------|
| `ce-specversion` | `1.0` |
| `ce-type` | routing key |
| `ce-source` | service name |
| `ce-id` | UUID |
| `ce-time` | RFC 3339 UTC timestamp |
| `ce-datacontenttype` | `application/json` |

On consume, `ValidateCEHeaders()` checks for required attributes and logs warnings for missing or malformed values.

AMQP uses the `cloudEvents:` prefix per the [AMQP binding spec](https://github.com/cloudevents/spec/blob/main/cloudevents/bindings/amqp-protocol-binding.md). Consumers normalize all prefix variants (`cloudEvents:*`, `cloudEvents_*`, `ce-*`) to the canonical `ce-` form.

## Observability

### Tracing (OpenTelemetry)

Both transports integrate with OpenTelemetry:
- Consumer spans with `messaging.system`, `messaging.operation`, `messaging.destination.name`, routing key, message ID, and body size attributes
- Publisher spans with exchange/stream and routing key attributes
- Context propagation via `TextMapPropagator` (injected into message headers)
- Configurable span names via `WithSpanNameFn()` and `WithPublishSpanNameFn()`

### Metrics (Prometheus)

Each transport registers Prometheus metrics via `InitMetrics(registerer)`:

**Consumer metrics:**
- `{transport}_events_received` - events received (counter)
- `{transport}_events_ack` - events acknowledged (counter)
- `{transport}_events_nack` / `{transport}_events_nak` - events rejected (counter)
- `{transport}_events_without_handler` - unhandled events (counter)
- `{transport}_events_not_parsable` - parse failures (counter)
- `{transport}_events_processed_duration` - processing time histogram

**Publisher metrics:**
- `{transport}_events_publish_succeed` - successful publishes (counter)
- `{transport}_events_publish_failed` - failed publishes (counter)
- `{transport}_events_publish_duration` - publish time histogram

### Logging

Go transports use `log/slog`. Node.js transports accept a logger interface compatible with `Pick<Console, "info" | "warn" | "error" | "debug">`.

## Topology Validation and Visualization

The messaging library provides tools for static analysis of messaging topologies:

- **`Validate(topology)`** - checks a single service's topology for structural correctness
- **`ValidateTopologies([]topology)`** - cross-service validation (consumers have matching publishers)
- **`Mermaid([]topology)`** - generates Mermaid flowchart diagrams
- **`DiscoverTopologies(brokerConfig)`** - reconstructs topologies from a running RabbitMQ broker

The `specverify` CLI wraps these for command-line use:

```sh
specverify validate topology.json
specverify cross-validate order-service.json notification-service.json
specverify visualize order-service.json notification-service.json
specverify discover --url http://localhost:15672
```

## Technology Compatibility Kit (TCK)

The TCK verifies transport implementations against the specification using real brokers. It communicates with adapters via a JSON-RPC subprocess protocol and runs five verification phases:

1. **Setup** - start services with randomized names
2. **Topology** - verify declared exchanges, queues, streams
3. **Broker State** - query broker directly to confirm resource creation
4. **Delivery** - publish messages and verify correct delivery
5. **Probes** - cross-validate with raw broker access

Anti-tampering measures include randomized service names, nonce-injected payloads, and direct broker verification.

## Conformance Testing

Shared JSON fixtures in `testdata/` define expected behavior for all implementations:

| File | Purpose |
|------|---------|
| `constants.json` | Verifies constant values (exchange kinds, patterns, CE headers) |
| `naming.json` | Verifies naming function outputs |
| `validate.json` | Verifies single and cross-topology validation rules |
| `topology.json` | Verifies endpoint generation from setup intents |
| `cloudevents.json` | Verifies CE header validation, metadata extraction, and message format |
| `tck.json` | Multi-service integration scenarios with message flows |

All fixtures are generated from Go source code by `TestGenerateFixtures` and are the canonical specification. Regenerate with:

```sh
go test -run TestGenerateFixtures ./...
```

## Implementing a New Transport

To create a transport implementation in any language:

1. **Implement the spec** - Port the naming functions and types from this module (or use the shared fixture files)
2. **Pass conformance tests** - Load `testdata/*.json` fixtures and verify your implementation produces identical outputs
3. **Map patterns to transport primitives**:
   - Event stream: topic exchange / JetStream stream with subject filtering
   - Service request: direct exchange / Core NATS request-reply
   - Service response: headers exchange / Core NATS reply
4. **Add CloudEvents** - Set required CE headers on publish, validate on consume
5. **Add observability** - OpenTelemetry tracing and Prometheus metrics following the same attribute conventions
6. **Export topology** - Implement `Topology()` so service topologies can be validated and visualized without connecting to a broker
7. **Write a TCK adapter** - Implement the subprocess protocol and pass all scenarios
