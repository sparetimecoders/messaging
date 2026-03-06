# Technology Compatibility Kit (TCK)

The TCK is a rigorous, tamper-resistant conformance test suite. It proves that transport implementations correctly handle all messaging patterns by running real messages through a real broker.

## Architecture

The TCK runner (Go) communicates with transport adapters via a JSON-RPC subprocess protocol:

```
TCK Runner (Go)                    Transport Adapter (any language)
      │                                       │
      │──── hello ───────────────────────────>│
      │<─── {protocolVersion, brokerConfig} ──│
      │                                       │
      │──── start_service ───────────────────>│
      │<─── {topology, publisherKeys} ────────│
      │                                       │
      │──── publish ─────────────────────────>│
      │──── received ────────────────────────>│
      │<─── {messages} ──────────────────────│
      │                                       │
      │──── shutdown ────────────────────────>│
```

The adapter is a simple CLI process. Requests arrive on stdin as JSON-RPC; responses go to file descriptor 3. This separation keeps adapter stdout/stderr free for logging.

## Five-Phase Verification

Each scenario runs through five phases:

### Phase 1: Setup

Start services with **randomized names** (prevents hardcoding in adapter implementations). The runner sends `start_service` for each service and receives the declared topology.

### Phase 2: Topology

Compare the adapter's declared endpoints against expected values from the test fixtures. Verifies that naming functions and pattern-to-broker mapping are correct.

### Phase 3: Broker State

Query the broker directly (RabbitMQ Management API or NATS JetStream API) to confirm that exchanges, queues, streams, and bindings actually exist. This catches adapters that report correct topology but fail to create broker resources.

### Phase 4: Delivery

Publish messages through the adapter and verify correct delivery:

- **Payload matching** — deserialized content matches expected values
- **Metadata matching** — CloudEvents headers present and correct
- **Routing correctness** — messages arrive at expected consumers only
- **Negative testing** — messages do NOT arrive at consumers with non-matching routing keys

### Phase 5: Probes

Cross-validation by bypassing the adapter entirely:

- **Inject probe**: TCK publishes a raw message directly to the broker → adapter consumer must receive it
- **Extract probe**: adapter publishes → TCK reads directly from the broker to verify

This phase catches adapters that work in isolation but use non-standard wire formats.

## Anti-Tampering Measures

The TCK prevents adapter implementations from cheating:

| Measure | How |
|---------|-----|
| **Randomized names** | Service names get a nonce suffix at runtime (`orders` → `orders-a1b2c3d4`) |
| **Payload nonces** | Each message includes a unique `_tckNonce` field in the payload |
| **Direct broker probes** | Phase 5 bypasses the adapter to verify wire-level compatibility |
| **Broker state queries** | Phase 3 confirms resources exist on the broker, not just in the adapter's memory |

## Running the TCK

### Prerequisites

- A running broker (RabbitMQ or NATS with JetStream)
- A compiled adapter binary for your transport

### From the Command Line

```sh
cd tck
go run ./cmd/tck-runner/ --adapter /path/to/adapter-binary
```

### From Go Tests

```go
func TestTCK(t *testing.T) {
    tck.RunTCK(t, myAdapter)
}
```

### Environment Variables

The adapter receives broker connection details during the `hello` handshake, but you can also configure via environment:

| Variable | Description |
|----------|-------------|
| `RABBITMQ_URL` | AMQP broker URL |
| `RABBITMQ_MANAGEMENT_URL` | RabbitMQ Management API URL |
| `NATS_URL` | NATS server URL |

## Writing a TCK Adapter

### The ServiceManager Interface

Your adapter implements this interface (or its equivalent in your language):

```go
type ServiceManager interface {
    StartService(ctx context.Context, name string, intents []SetupIntent) (
        topology messaging.Topology,
        publishers map[string]Publisher,
        err error,
    )
    CloseService(ctx context.Context, name string) error
}
```

- `StartService`: connect to the broker, declare topology, set up consumers, return the declared topology and publisher handles
- `CloseService`: disconnect and clean up

### Using adapterutil (Go)

The `adapterutil` package handles all JSON-RPC protocol details:

```go
package main

import (
    "os"
    "github.com/sparetimecoders/messaging/tck/adapterutil"
)

func main() {
    mgr := &myServiceManager{url: os.Getenv("RABBITMQ_URL")}
    adapterutil.Serve("amqp", brokerConfig, mgr)
}
```

`Serve()` reads JSON-RPC from stdin, dispatches to your `ServiceManager`, and writes responses to fd 3. Your adapter binary is a simple CLI.

### Non-Go Adapters

Implement the JSON-RPC protocol directly. See [TCK-PROTOCOL.md](../testdata/TCK-PROTOCOL.md) for the full protocol specification.

The protocol methods:

| Method | Direction | Description |
|--------|-----------|-------------|
| `hello` | runner → adapter | Handshake, adapter returns broker config |
| `start_service` | runner → adapter | Start a service with setup intents |
| `publish` | runner → adapter | Publish a message |
| `received` | runner → adapter | Query received messages |
| `close_service` | runner → adapter | Shut down a service |
| `shutdown` | runner → adapter | Terminate the adapter process |

### Example Adapters

- [Go AMQP adapter](https://github.com/sparetimecoders/go-messaging-amqp/tree/main/cmd/tck-adapter)
- [Go NATS adapter](https://github.com/sparetimecoders/go-messaging-nats/tree/main/cmd/tck-adapter)
- [Node AMQP adapter](https://github.com/sparetimecoders/nodejs-messaging-amqp/tree/main/tck-adapter)
- [Node NATS adapter](https://github.com/sparetimecoders/nodejs-messaging-nats/tree/main/tck-adapter)

## Coverage Matrix

The TCK tracks which (pattern, direction, ephemeral) combinations are exercised across all scenarios. After a full run, the coverage report shows:

```
Pattern              Direction  Ephemeral  Covered
event-stream         publish    false      YES
event-stream         consume    false      YES
event-stream         consume    true       YES
custom-stream        publish    false      YES
custom-stream        consume    false      YES
service-request      publish    false      YES
service-request      consume    false      YES
service-response     publish    false      YES
service-response     consume    false      YES
queue-publish        publish    false      NO
```

Gaps indicate scenarios that should be added to the test fixtures.

## Test Scenarios

Scenarios are defined in [`testdata/tck.json`](../testdata/tck.json) and cover multi-service interactions:

- Event stream pub/sub with multiple consumers
- Custom stream isolation
- Service request-response round trips
- Wildcard routing
- Transient (ephemeral) consumers
- Mixed patterns within a single service

Each scenario specifies:
- Services to start (with setup intents)
- Expected endpoints per service
- Expected broker state (exchanges, queues, bindings)
- Messages to publish with expected deliveries
- Probe messages for cross-validation
