# TCK Subprocess Protocol

The TCK runner communicates with transport adapters via a newline-delimited JSON-RPC protocol over file descriptors. This allows adapters to be written in any language.

## Wire Format

| FD | Direction | Purpose |
|----|-----------|---------|
| stdin (0) | TCK → adapter | JSON-RPC requests, newline-delimited |
| fd 3 | adapter → TCK | JSON-RPC responses, newline-delimited |
| stdout (1) | adapter → TCK | Diagnostic logs (forwarded to test output) |
| stderr (2) | adapter → TCK | Diagnostic logs (forwarded to test output) |

Using fd 3 for protocol responses means adapter code can freely use `fmt.Println`, `console.log`, `slog`, etc. without corrupting the protocol stream.

### Opening fd 3

- **Go**: `os.NewFile(3, "tck-protocol")`
- **Node.js**: `fs.createWriteStream(null, {fd: 3})` or `new net.Socket({fd: 3})`
- **Python**: `os.fdopen(3, 'w')`

## Envelope

```json
// Request (TCK → adapter via stdin)
{"id": 1, "method": "start_service", "params": {...}}

// Success response (adapter → TCK via fd 3)
{"id": 1, "result": {...}}

// Error response (adapter → TCK via fd 3)
{"id": 1, "error": {"code": -1, "message": "description"}}
```

Synchronous request/response with monotonic integer IDs. One request at a time; the adapter must respond with the matching ID before the next request is sent.

## Methods

### `hello` — Handshake

First request after process start. Establishes protocol version and discovers transport configuration.

```json
// Request
{"id": 1, "method": "hello", "params": {"protocolVersion": 1}}

// Response
{"id": 1, "result": {
  "protocolVersion": 1,
  "transportKey": "nats",
  "brokerConfig": {"natsURL": "nats://localhost:4222"}
}}
```

For AMQP adapters: `"brokerConfig": {"amqpURL": "amqp://...", "managementURL": "http://..."}`.

The TCK uses `brokerConfig` to connect directly to the broker for state validation and cross-validation probes.

### `start_service` — Start a service with intents

```json
// Request
{"id": 2, "method": "start_service", "params": {
  "serviceName": "orders-a8f3b2c1",
  "intents": [
    {"pattern": "event-stream", "direction": "publish"},
    {"pattern": "event-stream", "direction": "consume", "routingKey": "Order.Created"}
  ]
}}

// Response
{"id": 2, "result": {
  "publisherKeys": ["event-stream"],
  "topology": {
    "transport": "nats",
    "serviceName": "orders-a8f3b2c1",
    "endpoints": [...]
  }
}}
```

**Intent fields:**
- `pattern`: `"event-stream"`, `"custom-stream"`, `"service-request"`, `"service-response"`
- `direction`: `"publish"` or `"consume"`
- `routingKey`: Routing/filter key (optional, depends on pattern)
- `exchange`: Custom stream name (for `custom-stream` pattern)
- `targetService`: Target service name (for `service-request`/`service-response`)
- `ephemeral`: `true` for transient consumers

**Publisher keys** identify the publishers created for this service. Use these in subsequent `publish` calls. Keys follow the convention:
- `"event-stream"` for event stream publishers
- `"custom-stream:<exchange>"` for custom stream publishers
- `"service-request:<targetService>"` for service request publishers

### `publish` — Publish a message

```json
// Request
{"id": 3, "method": "publish", "params": {
  "serviceName": "orders-a8f3b2c1",
  "publisherKey": "event-stream",
  "routingKey": "Order.Created",
  "payload": {"orderId": "test-123", "_tckNonce": "uuid"}
}}

// Response
{"id": 3, "result": {}}
```

### `received` — Poll received messages

```json
// Request
{"id": 4, "method": "received", "params": {
  "serviceName": "notifications-a8f3b2c1"
}}

// Response
{"id": 4, "result": {
  "messages": [{
    "routingKey": "Order.Created",
    "payload": {"orderId": "test-123", "_tckNonce": "uuid"},
    "metadata": {
      "id": "...",
      "source": "orders-a8f3b2c1",
      "type": "Order.Created",
      "specVersion": "1.0",
      "dataContentType": "application/json",
      "timestamp": "2026-01-01T00:00:00Z"
    },
    "deliveryInfo": {
      "Destination": "...",
      "Source": "...",
      "Key": "Order.Created"
    }
  }]
}}
```

The adapter must accumulate all messages received by a service's consumers and return them when polled. The TCK may call `received` multiple times, polling until expected messages arrive.

### `close_service` — Tear down a single service

```json
// Request
{"id": 5, "method": "close_service", "params": {
  "serviceName": "orders-a8f3b2c1"
}}

// Response
{"id": 5, "result": {}}
```

### `shutdown` — Exit cleanly

```json
// Request
{"id": 6, "method": "shutdown", "params": {}}

// Response
{"id": 6, "result": {}}
```

The adapter should close all remaining services and exit after responding.

## Timeouts

| Phase | Timeout |
|-------|---------|
| Startup (hello) | 10s |
| Commands | 30s |
| Graceful shutdown | 5s (then SIGKILL) |

## Go Adapter Helper

The `github.com/sparetimecoders/gomessaging/tck/adapterutil` package provides a reusable serve loop. Implement `adapterutil.ServiceManager` and call `adapterutil.Serve()`:

```go
package main

import (
    "github.com/sparetimecoders/gomessaging/tck"
    "github.com/sparetimecoders/gomessaging/tck/adapterutil"
)

type myManager struct { /* ... */ }

func (m *myManager) StartService(name string, intents []spectest.SetupIntent) (*adapterutil.ServiceState, error) {
    // Create connection, set up publishers/consumers based on intents.
    // Return ServiceState with Topology, PublisherKeys, Publish, Received, Close.
}

func main() {
    brokerConfig := tck.BrokerConfig{NATSURL: os.Getenv("NATS_URL")}
    adapterutil.Serve("nats", brokerConfig, &myManager{})
}
```

See `golang/nats/cmd/tck-adapter/main.go` for the reference NATS adapter implementation.
