# Integration TCK (Technology Compatibility Kit)

The integration TCK validates that transport implementations (AMQP, NATS) correctly set up broker topology, route messages, and preserve CloudEvents metadata. Tests run against real brokers -- embedded NATS server and external RabbitMQ.

The fixture file `tck.json` is shared across Go and Node.js -- both implementations run the same scenarios.

## Architecture

```
specification/spec/testdata/
  tck.json                              <- shared fixture (scenarios, assertions)
  topology.json                         <- single-service topology conformance

specification/tck/                      <- TCK module (tamper-resistant runner)
  tck.go                                <- Adapter interface, RunTCK(), RunScenario()
  broker.go                             <- BrokerClient interface + BrokerConfig
  broker_amqp.go                        <- TCK-owned AMQP broker client
  broker_nats.go                        <- TCK-owned NATS broker client
  randomize.go                          <- NameMapper + InjectNonce
  topology_expect.go                    <- Compute expected topology from intents
  subprocess.go                         <- SubprocessAdapter (JSON-RPC over stdin/fd3)
  protocol.go                           <- Wire format types (Request, Response, etc.)
  cmd/tck-runner/                       <- Standalone runner binary
    main.go                             <- CLI entry point (flag parsing, scenario loop)
    cli_runner.go                       <- spectest.T impl using panic/recover
  adapterutil/                          <- Reusable adapter-side protocol handler
    adapterutil.go                      <- Serve(), ServiceManager interface
  docker-compose.yml                    <- NATS + RabbitMQ for local testing

specification/spec/spectest/
  t.go                                  <- T interface, WrapT(), assertion helpers
  spectest.go                           <- shared types + topology assertion helpers
  integration.go                        <- legacy runner (deprecated, superseded by tck/)

golang/
  tck-adapters/                         <- TCK adapter binaries (subprocess protocol)
    cmd/nats-adapter/main.go            <- Reference NATS adapter binary
    cmd/amqp-adapter/main.go            <- Reference AMQP adapter binary
  nats/integration_tck_test.go          <- Go NATS adapter (embedded server)
  amqp/integration_tck_test.go          <- Go AMQP adapter (real RabbitMQ)

nodejs/
  packages/nats/__tests__/tck.test.ts   <- Node.js NATS adapter (planned)
  packages/amqp/__tests__/tck.test.ts   <- Node.js AMQP adapter (planned)
```

### Tamper resistance

The TCK is hardened against implementations that hardcode responses:

1. **Randomized service names** -- each scenario run generates a random 8-hex-char suffix. `"orders"` becomes `"orders-a8f3b2c1"`, making queue/exchange/subject names unpredictable.
2. **Nonce injection** -- every message payload gets a unique `"_tckNonce"` UUID field injected at runtime. The TCK asserts the exact nonce appears in received messages.
3. **TCK-owned broker validation** -- all broker state queries, raw publish/consume are done by the TCK module directly, not delegated to the adapter.
4. **Computed expectations** -- expected topology, broker state, and probe targets are computed at runtime from service intents + randomized names (not read from static fixtures).

### Adapter contract

Each implementation provides a transport adapter with three operations:

| Operation | Go interface | What it does |
|-----------|-------------|--------------|
| **Transport key** | `TransportKey() string` | Returns `"amqp"` or `"nats"` to select transport-specific behavior |
| **Broker config** | `BrokerConfig() tck.BrokerConfig` | Returns connection URLs so the TCK can access the broker directly |
| **Start service** | `StartService(t spectest.T, name, intents) *ServiceHandle` | Creates a real connection with randomized name, maps intents to publishers/consumers |

> **Note:** The TCK uses `spectest.T` (not `*testing.T`) so it can run both in Go tests (via `spectest.WrapT(t)`) and in the standalone `tck-runner` binary. Adapter implementations in `_test.go` files can use `*testing.T` internally — only the `StartService` signature uses `spectest.T`.

The `ServiceHandle` returned by `StartService` exposes:

| Field | Type | Purpose |
|-------|------|---------|
| `Topology` | `func() Topology` | Returns the declared topology endpoints |
| `Publishers` | `map[string]PublishFunc` | Keyed by pattern (`event-stream`, `custom-stream:audit`, etc.) |
| `Received` | `func() []ReceivedMessage` | Goroutine-safe snapshot of all received messages |
| `Close` | `func() error` | Tears down the connection |

## Test execution phases

Each scenario runs six phases:

1. **Setup** -- clean broker state, generate `NameMapper` with random suffix, map service names + intents
2. **Start services** -- call `adapter.StartService()` with randomized names
3. **Assert topology** -- compute expected endpoints from intents, validate `ServiceHandle.Topology()`
4. **Assert broker state** -- TCK queries broker directly via its own client, compare with computed expectations
5. **Publish and assert delivery** -- inject nonces into payloads, publish, assert nonces + metadata + payload in received messages (metadata.source mapped to runtime name)
6. **Cross-validation probes** -- TCK-owned raw publish/consume with computed targets and nonces

## Scenarios

### 1. Event stream publish and consume

Two-service pub/sub: `orders` publishes `Order.Created`, `notifications` consumes it.

| Assertion | Detail |
|-----------|--------|
| Topology | Topic exchange/stream created; durable queue/consumer bound with routing key |
| Broker | AMQP: quorum queue with 5-day TTL; NATS: file-backed stream, durable consumer |
| Delivery | Message routed to notifications |
| Payload | `orderId=test-123`, `amount=42` verified on receiver |
| Metadata | `type=Order.Created`, `source=<runtime-name>`, `specVersion=1.0` |
| Nonce | `_tckNonce` injected and verified |

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

### 5. Service request message flow

Two-service request-reply setup: `email-svc` consumes requests, `web-app` publishes and consumes responses.

| Assertion | Detail |
|-----------|--------|
| Topology | AMQP: direct exchange for requests, headers exchange for responses; NATS: Core subscription |
| Broker | AMQP: request queue + response queue; NATS: no JetStream resources (Core only) |
| Delivery | `web-app` publishes `email.send` request, `email-svc` receives it |
| Payload | `to=user@example.com`, `subject=Welcome` verified on receiver |
| Metadata | `type=email.send`, `source=<runtime-name>`, `specVersion=1.0` |

### 6. Wildcard routing

Three-service wildcard routing: `orders` publishes, `notifications` subscribes to `Order.*` (wildcard), `billing` subscribes to `Order.Created` (exact).

| Assertion | Detail |
|-----------|--------|
| Topology | One publisher, two consumers with different routing key patterns |
| Broker | AMQP: two queues bound with wildcard and exact routing keys; NATS: two durable consumers with different filter subjects |
| Delivery (positive) | `Order.Created` delivered to both `notifications` and `billing` |
| Delivery (negative) | `Order.Updated` delivered to `notifications` only, NOT to `billing` |
| Payload | `orderId` verified on both consumers for first message; `status=shipped` on notifications for second |

## Cross-validation probes (Phase 6)

Cross-validation probes prevent implementations from passing the TCK by hardcoding responses. Without probes, an implementation could:
- Fake `Topology()` output without creating real broker resources
- Return success from `Publish()` without sending anything
- Pre-populate `Received()` with expected messages

Probes verify the broker is actually involved by using the TCK's own broker client:

### Outbound probes

The implementation publishes a message via its API. The TCK independently consumes from the broker using its own raw client to verify:
- The message actually arrived on the broker
- CloudEvents headers are set correctly (`type`, `source`, `specversion`)
- Payload matches expected content + contains the injected nonce

### Inbound probes

The TCK publishes a raw message directly to the broker with a unique `source` (`tck-probe`) and injected nonce. The implementation's consumer should receive it. The TCK verifies via `Received()` that:
- The message was actually consumed from the broker
- CloudEvents metadata was extracted correctly
- Payload was preserved including the nonce

Scenarios 1 (event stream), 2 (fan-out), 3 (custom stream), and 4 (transient consumer) include both outbound and inbound probes. Scenarios 5 (service request) and 6 (wildcard routing) do not have probes -- service requests use Core NATS (no JetStream) and wildcard routing is covered by the delivery assertions.

## Running the tests

### Prerequisites — Start brokers

A `docker-compose.yml` is provided in `specification/tck/` with NATS (JetStream enabled) and RabbitMQ (management plugin):

```bash
cd specification/tck && docker compose up -d
```

This starts:
- **NATS** on `localhost:4222` with JetStream (`-js`)
- **RabbitMQ** on `localhost:5672` (AMQP) and `localhost:15672` (management API)

> The Go NATS tests use an embedded server and don't need external NATS. External brokers are only required for the standalone runner and AMQP tests.

### Standalone runner (any language)

The standalone `tck-runner` binary runs TCK scenarios against any adapter binary that implements the [subprocess protocol](TCK-PROTOCOL.md). No Go test harness needed.

**Build the runner:**

```bash
cd specification/tck && go build -o tck-runner ./cmd/tck-runner/
```

**Build an adapter** (e.g. the reference NATS adapter):

```bash
cd golang/tck-adapters && go build -o nats-tck-adapter ./cmd/nats-adapter/
```

**Build the AMQP adapter:**

```bash
cd golang/tck-adapters && go build -o amqp-tck-adapter ./cmd/amqp-adapter/
```

**Run it:**

```bash
NATS_URL=nats://localhost:4222 ./tck-runner \
  -adapter ./nats-tck-adapter \
  -fixtures ./specification/spec/testdata/tck.json
```

**Flags:**

| Flag | Required | Description |
|------|----------|-------------|
| `-adapter PATH` | yes | Path to adapter binary |
| `-fixtures PATH` | yes | Path to `tck.json` fixture file |
| `-scenario NAME` | no | Run only scenarios matching substring |
| `-v` | no | Verbose logging (shows name mappings, adapter output) |

**Output** mirrors `go test -v`:

```
=== RUN   event stream publish and consume
--- PASS: event stream publish and consume (0.15s)
=== RUN   event stream fan-out
--- PASS: event stream fan-out (0.12s)

PASS (6/6 scenarios)
```

Exit code `0` on success, `1` on any failure, `2` on usage error.

### Go tests

```bash
# NATS -- no external dependencies (uses embedded server, one per scenario)
cd golang && go test -race -count=1 -run TestIntegrationTCK ./nats/...

# NATS -- verbose (shows randomized names per scenario)
cd golang && go test -race -count=1 -run TestIntegrationTCK -v ./nats/...

# NATS -- subprocess mode (builds and spawns tck-adapter binary)
cd golang && go test -race -count=1 -run TestIntegrationTCKSubprocess -v ./nats/...

# AMQP -- requires running RabbitMQ
cd golang && \
  RABBITMQ_URL=amqp://guest:guest@localhost:5672/ \
  go test -race -count=1 -run TestIntegrationTCK ./amqp/...

# AMQP -- with custom management URL
cd golang && \
  RABBITMQ_URL=amqp://guest:guest@localhost:5672/ \
  RABBITMQ_MANAGEMENT_URL=http://guest:guest@localhost:15672 \
  go test -race -count=1 -run TestIntegrationTCK -v ./amqp/...

# TCK module unit tests (randomization, topology computation)
cd specification && go test -race -count=1 ./tck/...

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
| `NATS_URL` | *(adapter-specific)* | NATS connection URL (used by standalone runner and tck-adapter) |

### Node.js

The Node.js TCK adapters are not yet implemented. With the standalone runner, a Node.js adapter just needs to implement the [subprocess protocol](TCK-PROTOCOL.md):

```bash
# Build the adapter
cd nodejs/packages/nats && npm run build:tck-adapter

# Run using the standalone runner
NATS_URL=nats://localhost:4222 tck-runner \
  -adapter ./nodejs/packages/nats/tck-adapter \
  -fixtures ./specification/spec/testdata/tck.json
```

## Adding a new scenario

1. Add a scenario object to `specification/spec/testdata/tck.json`
2. Define `services` with setup intents, `messages` with expected deliveries, and optional `probeMessages`
3. Expected topology and broker state are computed automatically from the intents
4. No adapter changes needed -- adapters already map all supported intent patterns

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
  "messages": [
    {
      "from": "<service>",
      "routingKey": "Order.Created",
      "payload": { "any": "json" },
      "expectedDeliveries": [
        {
          "to": "<service>",
          "metadata": { "type": "...", "source": "...", "specVersion": "..." },
          "payloadMatch": { "subset": "of payload fields" }
        }
      ],
      "unexpectedDeliveries": [
        {
          "to": "<service>",
          "metadataType": "Order.Updated"
        }
      ]
    }
  ],
  "probeMessages": [
    {
      "direction": "outbound|inbound",
      "publishVia": "<service>",               // outbound only
      "expectReceivedBy": "<service>",         // inbound only
      "routingKey": "...",
      "payload": { "any": "json" },
      "ceAttributes": { "type": "...", "source": "...", "specversion": "..." },
      "payloadMatch": { "subset": "of payload fields" }
    }
  ]
}
```

## Adding a new transport

1. Create an adapter test file for the new transport
2. Implement the adapter contract (`TransportKey`, `BrokerConfig`, `StartService`)
3. Add transport-specific logic to `specification/tck/broker_<transport>.go` and `topology_expect.go`
4. The TCK handles broker validation and probe execution automatically

## Future extensions

- **Dead letter routing** -- verify messages are routed to DLX after consumer rejection
- **Redelivery / backoff** -- verify MaxDeliver and BackOff consumer settings
- **Request-reply response assertions** -- verify the response payload returned to the publisher
- **Node.js adapters** -- implement `tck.test.ts` for `@gomessaging/nats` and `@gomessaging/amqp`
