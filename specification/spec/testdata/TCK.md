# Integration TCK (Technology Compatibility Kit)

The integration TCK validates that transport implementations (AMQP, NATS) correctly set up broker topology, route messages, and preserve CloudEvents metadata. Tests run against real brokers -- embedded NATS server and external RabbitMQ.

## Architecture

```
specification/spec/testdata/tck.json    <- shared fixture (scenarios, assertions)
specification/spec/spectest/integration.go  <- runner + types (transport-agnostic)
golang/nats/integration_tck_test.go     <- NATS adapter (embedded server)
golang/amqp/integration_tck_test.go     <- AMQP adapter (real RabbitMQ)
```

Implementations provide an `IntegrationAdapter` with three methods:

| Method | Purpose |
|--------|---------|
| `TransportKey()` | Returns `"amqp"` or `"nats"` to select transport-specific fixtures |
| `StartService(t, name, intents)` | Creates a real connection, sets up publishers/consumers, returns a `ServiceHandle` |
| `QueryBrokerState(t)` | Queries the running broker for actual exchanges/queues/streams/consumers |

## Test execution phases

Each scenario runs four phases:

1. **Start services** -- create real connections for each service in the scenario
2. **Assert topology** -- verify each service's `Topology()` endpoints match expected
3. **Assert broker state** -- query the live broker and compare against expected declarations
4. **Publish and assert delivery** -- send real messages and verify correct routing, payload integrity, and CloudEvents metadata

## Scenarios

### 1. Event stream publish and consume

Two-service pub/sub: `orders` publishes `Order.Created`, `notifications` consumes it.

**What it validates:**
- Topic exchange/stream is created with correct properties
- Durable queue/consumer is created and bound with the correct routing key
- Published message is routed to the consumer
- Payload arrives intact (`orderId`, `amount` fields verified)
- CloudEvents metadata is populated (`type=Order.Created`, `source=orders`, `specVersion=1.0`)

### 2. Event stream fan-out

Three-service fan-out: `orders` publishes, both `notifications` and `analytics` consume `Order.Created`.

**What it validates:**
- Single exchange/stream serves multiple independent consumers
- Each consumer gets its own durable queue/consumer with separate naming
- The same message is delivered to both consumers independently
- Payload fields are verified per-consumer (`orderId` on notifications, `amount` on analytics)

### 3. Custom stream publish and consume

Single-service custom stream: `analytics` publishes and consumes on the `audit` stream.

**What it validates:**
- Named/custom stream creation (not the default `events` stream)
- Stream and consumer use the custom exchange name (`audit.topic.exchange` / `audit`)
- Self-publish-consume loop works (same service publishes and consumes)
- Full payload round-trip verification (`action`, `userId` fields)

### 4. Transient consumer

Two-service with ephemeral consumer: `orders` publishes, `dashboard` consumes transiently.

**What it validates:**
- Ephemeral/transient queue creation (AMQP: short TTL, random suffix; NATS: no durable name)
- Queue name uses prefix matching (AMQP: `events.topic.exchange.queue.dashboard-*`)
- Messages are still delivered to ephemeral consumers
- Payload verification on transient delivery path

### 5. Service request topology

Two-service request-reply setup: `email-svc` consumes requests, `web-app` publishes requests and consumes responses.

**What it validates:**
- Direct exchange creation for requests (AMQP) / Core subscription (NATS)
- Headers exchange creation for responses (AMQP only)
- Correct queue naming for request and response paths
- Topology-only -- no message delivery (request-reply is synchronous, deferred to follow-up)

## Assertion details

### Topology assertions

Each service's `Topology()` output is checked against `expectedEndpoints[transport][service]`:

| Field | Checked |
|-------|---------|
| `direction` | publish or consume |
| `pattern` | event-stream, custom-stream, service-request, service-response |
| `exchangeName` | exact match |
| `exchangeKind` | topic, direct, or headers |
| `queueName` | exact match (durable) or prefix match (ephemeral) |
| `routingKey` | exact match |
| `ephemeral` | boolean flag |

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

1. Publish via the source service's `Publisher.Publish()` with real serialization
2. Wait (up to 5s) for each expected consumer to receive the message
3. Assert CloudEvents metadata fields: `type`, `source`, `specVersion`, `dataContentType`
4. Assert payload fields match expected values (subset matching -- extra fields are allowed)

## Running the tests

```bash
# NATS integration TCK (no external dependencies -- uses embedded server)
cd golang && go test -race -count=1 -run TestIntegrationTCK ./nats/...

# AMQP integration TCK (requires RabbitMQ)
cd demo && docker compose up -d rabbitmq
cd golang && RABBITMQ_URL=amqp://guest:guest@localhost:5672/ go test -race -count=1 -run TestIntegrationTCK ./amqp/...

# Optionally set management URL (defaults to http://guest:guest@localhost:15672)
RABBITMQ_MANAGEMENT_URL=http://guest:guest@localhost:15672 \
RABBITMQ_URL=amqp://guest:guest@localhost:5672/ \
go test -race -count=1 -run TestIntegrationTCK -v ./amqp/...
```

## Adding a new scenario

1. Add a scenario object to `specification/spec/testdata/tck.json`
2. Define `services` with setup intents, `expectedEndpoints` per transport/service, `broker` state, and `messages`
3. No adapter changes needed -- the adapters map all supported intent patterns already

## Adding a new transport

1. Create `<transport>/integration_tck_test.go`
2. Implement `IntegrationAdapter` with `TransportKey()`, `StartService()`, `QueryBrokerState()`
3. Add transport-specific endpoints and broker state to each scenario in `tck.json`

## Future extensions

- **Wildcard routing** -- consumer subscribes to `Order.*`, receives both `Order.Created` and `Order.Updated`
- **Negative routing** -- verify messages do NOT arrive at consumers with non-matching routing keys
- **Multiple messages per scenario** -- publish several messages and assert ordering/completeness
- **Request-reply message flow** -- end-to-end service request with synchronous response
- **Dead letter routing** -- verify messages are routed to DLX after consumer rejection
