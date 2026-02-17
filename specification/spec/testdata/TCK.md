# Integration TCK (Technology Compatibility Kit)

The integration TCK validates that transport implementations (AMQP, NATS) correctly set up broker topology, route messages, and preserve CloudEvents metadata. Tests run against real brokers -- embedded NATS server and external RabbitMQ.

The fixture file `tck.json` is shared across Go and Node.js -- both implementations run the same scenarios.

## Architecture

```
specification/spec/testdata/
  tck.json                              <- shared fixture (scenarios, assertions)
  topology.json                         <- single-service topology conformance

specification/spec/spectest/
  spectest.go                           <- shared types + assertion helpers
  integration.go                        <- Go TCK runner + IntegrationAdapter interface

golang/
  nats/integration_tck_test.go          <- Go NATS adapter (embedded server)
  amqp/integration_tck_test.go          <- Go AMQP adapter (real RabbitMQ)

nodejs/
  packages/nats/__tests__/tck.test.ts   <- Node.js NATS adapter (planned)
  packages/amqp/__tests__/tck.test.ts   <- Node.js AMQP adapter (planned)
```

### Adapter contract

Each implementation provides a transport adapter with three operations:

| Operation | Go interface | What it does |
|-----------|-------------|--------------|
| **Transport key** | `TransportKey() string` | Returns `"amqp"` or `"nats"` to select transport-specific fixtures |
| **Start service** | `StartService(t, name, intents) *ServiceHandle` | Creates a real connection, maps intents to publishers/consumers, starts the connection |
| **Query broker** | `QueryBrokerState(t) BrokerState` | Queries the running broker for actual exchanges/queues/streams/consumers |

The `ServiceHandle` returned by `StartService` exposes:

| Field | Type | Purpose |
|-------|------|---------|
| `Topology` | `func() Topology` | Returns the declared topology endpoints |
| `Publishers` | `map[string]PublishFunc` | Keyed by pattern (`event-stream`, `custom-stream:audit`, etc.) |
| `Received` | `func() []ReceivedMessage` | Goroutine-safe snapshot of all received messages |
| `Close` | `func() error` | Tears down the connection |

### Probe adapter contract (optional)

Adapters can optionally implement `ProbeAdapter` to enable Phase 5 cross-validation:

| Operation | Go interface | What it does |
|-----------|-------------|--------------|
| **Publish raw** | `PublishRaw(t, target, payload, headers) error` | Publishes a raw message to the broker, bypassing the implementation |
| **Create probe consumer** | `CreateProbeConsumer(t, target) *ProbeConsumer` | Sets up a raw consumer that reads directly from the broker |

## Test execution phases

Each scenario runs up to five phases:

1. **Start services** -- create real connections for each service in the scenario
2. **Assert topology** -- verify each service's `Topology()` endpoints match expected
3. **Assert broker state** -- query the live broker and compare against expected declarations
4. **Publish and assert delivery** -- send real messages and verify correct routing, payload integrity, and CloudEvents metadata
5. **Cross-validation probes** -- (if adapter implements `ProbeAdapter`) verify that messages actually flow through the broker using independent raw clients

## Scenarios

### 1. Event stream publish and consume

Two-service pub/sub: `orders` publishes `Order.Created`, `notifications` consumes it.

| Assertion | Detail |
|-----------|--------|
| Topology | Topic exchange/stream created; durable queue/consumer bound with routing key |
| Broker | AMQP: quorum queue with 5-day TTL; NATS: file-backed stream, durable consumer |
| Delivery | Message routed to notifications |
| Payload | `orderId=test-123`, `amount=42` verified on receiver |
| Metadata | `type=Order.Created`, `source=orders`, `specVersion=1.0` |

### 2. Event stream fan-out

Three-service fan-out: `orders` publishes, both `notifications` and `analytics` consume `Order.Created`.

| Assertion | Detail |
|-----------|--------|
| Topology | Single exchange/stream, two independent consumer queues |
| Broker | Two separate durable queues/consumers, both on same exchange/stream |
| Delivery | Same message delivered to both consumers independently |
| Payload | `orderId` verified on notifications, `amount` verified on analytics |

### 3. Custom stream publish and consume

Single-service custom stream: `analytics` publishes and consumes on the `audit` stream.

| Assertion | Detail |
|-----------|--------|
| Topology | Custom-named exchange/stream (`audit.topic.exchange` / `audit`) |
| Broker | Separate stream from default `events`; own consumer |
| Delivery | Self-publish-consume loop |
| Payload | `action=login`, `userId=user-456` round-trip verified |

### 4. Transient consumer

Two-service with ephemeral consumer: `orders` publishes, `dashboard` consumes transiently.

| Assertion | Detail |
|-----------|--------|
| Topology | Ephemeral endpoint with prefix-matched queue name |
| Broker | AMQP: 1s TTL quorum queue, random suffix; NATS: ephemeral consumer (no durable) |
| Delivery | Message delivered despite ephemeral consumer |
| Payload | `orderId=test-789` verified |

### 5. Service request topology

Two-service request-reply setup: `email-svc` consumes requests, `web-app` publishes and consumes responses.

| Assertion | Detail |
|-----------|--------|
| Topology | AMQP: direct exchange for requests, headers exchange for responses; NATS: Core subscription |
| Broker | AMQP: request queue + response queue; NATS: no JetStream resources (Core only) |
| Delivery | Topology-only, no messages (request-reply is synchronous, deferred to follow-up) |

## Cross-validation probes (Phase 5)

Cross-validation probes prevent implementations from passing the TCK by hardcoding responses. Without probes, an implementation could:
- Fake `Topology()` output without creating real broker resources
- Return success from `Publish()` without sending anything
- Pre-populate `Received()` with expected messages

Probes verify the broker is actually involved by using independent raw broker clients:

### Outbound probes

The implementation publishes a message via its API. The TCK independently consumes from the broker using a raw client (NATS subscription or temporary AMQP queue) to verify:
- The message actually arrived on the broker
- CloudEvents headers are set correctly (`type`, `source`, `specversion`)
- Payload matches expected content

### Inbound probes

The TCK publishes a raw message directly to the broker with a unique `source` (`tck-probe`). The implementation's consumer should receive it. The TCK verifies via `Received()` that:
- The message was actually consumed from the broker
- CloudEvents metadata was extracted correctly
- Payload was preserved

### Probe fixture format

```jsonc
"probeMessages": [
  {
    "direction": "outbound",
    "publishVia": "orders",                    // service that publishes
    "routingKey": "Probe.Outbound",
    "payload": {"probeId": "out-1"},
    "rawTarget": {
      "nats": {"stream": "events", "subject": "events.Probe.Outbound"},
      "amqp": {"exchange": "events.topic.exchange", "routingKey": "Probe.Outbound"}
    },
    "ceAttributes": {"type": "Probe.Outbound", "source": "orders", "specversion": "1.0"},
    "payloadMatch": {"probeId": "out-1"}
  },
  {
    "direction": "inbound",
    "expectReceivedBy": "notifications",       // service that should receive
    "routingKey": "Order.Created",
    "payload": {"probeId": "in-1", "_tckInjected": true},
    "rawTarget": {
      "nats": {"subject": "events.Order.Created"},
      "amqp": {"exchange": "events.topic.exchange", "routingKey": "Order.Created"}
    },
    "ceAttributes": {"type": "Order.Created", "source": "tck-probe", "specversion": "1.0"},
    "payloadMatch": {"probeId": "in-1", "_tckInjected": true}
  }
]
```

Scenarios 1 (event stream) and 3 (custom stream) include both outbound and inbound probes.

## Assertion details

### Topology assertions

Each service's `Topology()` output is checked against `expectedEndpoints[transport][service]`:

| Field | Match type |
|-------|-----------|
| `direction` | exact (`publish` or `consume`) |
| `pattern` | exact (`event-stream`, `custom-stream`, `service-request`, `service-response`) |
| `exchangeName` | exact |
| `exchangeKind` | exact (`topic`, `direct`, `headers`) |
| `queueName` | exact (durable) or prefix (ephemeral) |
| `routingKey` | exact |
| `ephemeral` | exact (boolean) |

### Broker state assertions

The runner queries the live broker and compares against `broker.amqp` or `broker.nats`:

**AMQP:**
- Exchange count, names, types, durable/autoDelete flags
- Queue count, names (exact or prefix), durable/autoDelete, arguments (`x-queue-type`, `x-expires`)
- Binding count, source/destination/routingKey matching

**NATS:**
- Stream count, names, subject patterns, storage type
- Consumer count, stream assignment, durable name, filter subject(s), ack policy

### Message delivery assertions

For each message in the scenario:

1. Publish via the source service's publisher with real serialization
2. Wait (up to 5s with 50ms polling) for each expected consumer to receive the message
3. Assert CloudEvents metadata: `type`, `source`, `specVersion`, `dataContentType`
4. Assert payload fields via subset matching (expected fields must be present; extra fields allowed)

## Running the tests

### Prerequisites

```bash
# Start external brokers (only needed for AMQP tests)
cd demo && docker compose up -d rabbitmq
```

### Go

```bash
# NATS -- no external dependencies (uses embedded server, one per scenario)
cd golang && go test -race -count=1 -run TestIntegrationTCK ./nats/...

# NATS -- verbose
cd golang && go test -race -count=1 -run TestIntegrationTCK -v ./nats/...

# AMQP -- requires running RabbitMQ
cd golang && \
  RABBITMQ_URL=amqp://guest:guest@localhost:5672/ \
  go test -race -count=1 -run TestIntegrationTCK ./amqp/...

# AMQP -- with custom management URL
cd golang && \
  RABBITMQ_URL=amqp://guest:guest@localhost:5672/ \
  RABBITMQ_MANAGEMENT_URL=http://guest:guest@localhost:15672 \
  go test -race -count=1 -run TestIntegrationTCK -v ./amqp/...

# Both transports
cd golang && \
  RABBITMQ_URL=amqp://guest:guest@localhost:5672/ \
  go test -race -count=1 -run TestIntegrationTCK ./nats/... ./amqp/...
```

**Environment variables:**

| Variable | Default | Purpose |
|----------|---------|---------|
| `RABBITMQ_URL` | *(none -- test skips if unset)* | AMQP connection URL |
| `RABBITMQ_MANAGEMENT_URL` | `http://guest:guest@localhost:15672` | RabbitMQ HTTP API for broker state queries |

### Node.js

The Node.js TCK adapters are not yet implemented. The planned approach mirrors Go:

```bash
# NATS -- using nats-server npm package or docker
cd nodejs && npm test -- --grep "integration tck"

# AMQP -- requires running RabbitMQ
cd nodejs && \
  RABBITMQ_URL=amqp://guest:guest@localhost:5672/ \
  npm test -- --grep "integration tck"
```

**To implement Node.js adapters:**

1. Read `tck.json` scenarios in each test file
2. For each scenario, create real connections using `@gomessaging/nats` or `@gomessaging/amqp`
3. Map `SetupIntent` objects to `connection.addEventPublisher()`, `connection.addEventConsumer()`, etc.
4. Capture received messages in handler callbacks
5. Query broker state via NATS monitoring API (`http://localhost:8222/jsz`) or RabbitMQ Management API
6. Assert topology, broker state, metadata, and payload using the same fixture expectations

The fixture structure is transport-agnostic JSON, so the Node.js tests read the exact same `tck.json` file and apply the same assertions.

## Adding a new scenario

1. Add a scenario object to `specification/spec/testdata/tck.json`
2. Define `services` with setup intents, `expectedEndpoints` per transport/service, `broker` state, and `messages`
3. No adapter changes needed -- adapters already map all supported intent patterns

### Fixture schema

```jsonc
{
  "name": "scenario name",
  "services": {
    "<service>": {
      "setups": [
        {
          "pattern": "event-stream|custom-stream|service-request|service-response",
          "direction": "publish|consume",
          "routingKey": "Order.Created",       // consume only
          "exchange": "audit",                  // custom-stream only
          "targetService": "email-svc",         // service-request/response only
          "ephemeral": true                     // optional, consume only
        }
      ]
    }
  },
  "expectedEndpoints": {
    "<transport>": {
      "<service>": [
        { "direction": "...", "pattern": "...", "exchangeName": "...", "exchangeKind": "...",
          "queueName": "...", "queueNamePrefix": "...", "routingKey": "...", "ephemeral": false }
      ]
    }
  },
  "broker": {
    "amqp": { "exchanges": [...], "queues": [...], "bindings": [...] },
    "nats": { "streams": [...], "consumers": [...] }
  },
  "messages": [
    {
      "from": "<service>",
      "routingKey": "Order.Created",
      "payload": { "any": "json" },
      "expectedDeliveries": [
        {
          "to": "<service>",
          "metadata": { "type": "...", "source": "...", "specVersion": "..." },
          "payloadMatch": { "subset": "of payload fields" }  // optional
        }
      ]
    }
  ],
  "probeMessages": [                           // optional, requires ProbeAdapter
    {
      "direction": "outbound|inbound",
      "publishVia": "<service>",               // outbound only
      "expectReceivedBy": "<service>",         // inbound only
      "routingKey": "...",
      "payload": { "any": "json" },
      "rawTarget": {
        "nats": { "stream": "...", "subject": "..." },
        "amqp": { "exchange": "...", "routingKey": "..." }
      },
      "ceAttributes": { "type": "...", "source": "...", "specversion": "..." },
      "payloadMatch": { "subset": "of payload fields" }
    }
  ]
}
```

## Adding a new transport

1. Create an adapter test file for the new transport
2. Implement the adapter contract (`TransportKey`, `StartService`, `QueryBrokerState`)
3. Optionally implement `ProbeAdapter` (`PublishRaw`, `CreateProbeConsumer`) for cross-validation
4. Add transport-specific `expectedEndpoints`, `broker` state, and `rawTarget` entries to each scenario in `tck.json`

## Future extensions

- **Wildcard routing** -- consumer subscribes to `Order.*`, receives both `Order.Created` and `Order.Updated`
- **Negative routing** -- verify messages do NOT arrive at consumers with non-matching routing keys
- **Multiple messages per scenario** -- publish several messages and assert ordering/completeness
- **Request-reply message flow** -- end-to-end service request with synchronous response
- **Dead letter routing** -- verify messages are routed to DLX after consumer rejection
- **Node.js adapters** -- implement `tck.test.ts` for `@gomessaging/nats` and `@gomessaging/amqp`
