# messaging Specification Fixtures

Language-agnostic test fixtures for validating implementations of the messaging specification. Any transport implementation (Go, Node.js, Python, etc.) can load these JSON files and run conformance tests.

## Conformance Levels

Following RFC 2119 terminology:

| Level | Meaning |
|-------|---------|
| **MUST** | Absolute requirement. Implementations that do not meet this are non-conformant. |
| **SHOULD** | Recommended. May be omitted for valid reasons, but implications must be understood. |

### MUST requirements

- All naming functions (exact string output matching)
- Single-topology validation (error detection for structural issues)
- Cross-topology validation (publisher/consumer matching)
- CloudEvents required header validation (5 required attributes)
- Metadata extraction from headers
- Binary content mode for CloudEvents (headers carry CE attributes, body carries data)
- JSON message body encoding (`application/json`)

### SHOULD requirements

- Optional CloudEvents headers (`ce-datacontenttype`, `ce-subject`, `ce-dataschema`)
- Extension headers (`ce-correlationid`)
- Topology export (the Topology struct for visualization/validation tooling)
- AMQP broker state verification (exchange/queue/binding declarations)

## Files

### constants.json

Canonical string values for all spec enums and CloudEvents header keys. Implementations MUST use these exact values.

```json
{
  "defaultEventExchangeName": "events",
  "exchangeKinds": { "topic": "topic", "direct": "direct", "headers": "headers" },
  "directions": { "publish": "publish", "consume": "consume" },
  "patterns": {
    "eventStream": "event-stream",
    "customStream": "custom-stream",
    "serviceRequest": "service-request",
    "serviceResponse": "service-response",
    "queuePublish": "queue-publish"
  },
  "transports": { "amqp": "amqp", "nats": "nats" },
  "cloudEvents": {
    "specVersion": "ce-specversion",
    "type": "ce-type",
    "source": "ce-source",
    "id": "ce-id",
    "time": "ce-time",
    "specVersionValue": "1.0",
    "dataContentType": "ce-datacontenttype",
    "subject": "ce-subject",
    "dataSchema": "ce-dataschema",
    "correlationId": "ce-correlationid"
  }
}
```

**How to test:** Assert each constant in your implementation matches the expected value.

### naming.json

Test cases for all naming functions. Each key maps to a function name, with an array of `{input, expected}` pairs.

| Function | Input | Output | Description |
|----------|-------|--------|-------------|
| `topicExchangeName` | `{name}` | `<name>.topic.exchange` | AMQP topic exchange name |
| `serviceEventQueueName` | `{exchangeName, service}` | `<exchangeName>.queue.<service>` | Durable event queue |
| `serviceRequestExchangeName` | `{service}` | `<service>.direct.exchange.request` | Direct request exchange |
| `serviceResponseExchangeName` | `{service}` | `<service>.headers.exchange.response` | Headers response exchange |
| `serviceRequestQueueName` | `{service}` | `<service>.direct.exchange.request.queue` | Request queue |
| `serviceResponseQueueName` | `{targetService, serviceName}` | `<target>.headers.exchange.response.queue.<service>` | Response queue |
| `natsStreamName` | `{name}` | strips `.topic.exchange` suffix if present | NATS stream name |
| `natsSubject` | `{stream, routingKey}` | `<stream>.<routingKey>` | NATS subject |
| `translateWildcard` | `{routingKey}` | replaces `#` with `>` | AMQP to NATS wildcard |

**How to test:** For each function and test case, call the function with the input fields and assert the output equals `expected`.

### validate.json

Topology validation test cases for both single-service and cross-service validation.

**`single`** — Single-service validation. Each case has a topology and an expected error (`null` = valid).

```json
{
  "name": "descriptive name",
  "topology": {
    "transport": "amqp",
    "serviceName": "orders",
    "endpoints": [...]
  },
  "expectedError": null
}
```

**`cross`** — Cross-service validation. Each case has multiple topologies and checks inter-service consistency.

```json
{
  "name": "missing publisher",
  "topologies": [...],
  "expectedError": "no service publishes it"
}
```

**Error matching:** When `expectedError` is a string, the implementation's error message MUST contain that substring. When `null`, validation MUST succeed with no error.

**How to test:**
- For `single` cases: call `Validate(topology)` and check error/no-error. If `expectedError` is set, verify the error message contains that substring.
- For `cross` cases: call `ValidateTopologies(topologies)` with the same logic.

### topology.json

End-to-end topology conformance scenarios that verify endpoint generation from high-level setup intents.

```json
{
  "scenarios": [
    {
      "name": "event stream publisher",
      "serviceName": "orders",
      "setups": [
        { "pattern": "event-stream", "direction": "publish" }
      ],
      "expectedEndpoints": {
        "amqp": [...],
        "nats": [...]
      },
      "broker": {
        "amqp": { "exchanges": [...], "queues": [...], "bindings": [...] }
      }
    }
  ]
}
```

**Setup intent fields:**
| Field | Type | Description |
|-------|------|-------------|
| `pattern` | string | One of: `event-stream`, `custom-stream`, `service-request`, `service-response` |
| `direction` | string | `publish` or `consume` |
| `routingKey` | string | Routing key pattern (for consumers) |
| `exchange` | string | Custom exchange name (for `custom-stream` pattern) |
| `targetService` | string | Target service (for `service-request`/`service-response`) |
| `ephemeral` | boolean | Whether the consumer is transient |

**Expected endpoint fields:**
| Field | Type | Description |
|-------|------|-------------|
| `direction` | string | `publish` or `consume` |
| `pattern` | string | Communication pattern |
| `exchangeName` | string | Exact exchange/stream name |
| `exchangeKind` | string | `topic`, `direct`, or `headers` |
| `queueName` | string | Exact queue name (for durable consumers) |
| `queueNamePrefix` | string | Queue name prefix (for transient consumers with UUID suffix) |
| `routingKey` | string | Routing key or subject filter |
| `ephemeral` | boolean | Whether the endpoint is transient |

**How to test:**
1. For each scenario, interpret the `setups` array as setup actions for your transport
2. Build the topology using your transport's setup functions
3. Compare the resulting endpoints against `expectedEndpoints[yourTransport]`
4. Match endpoints by direction + pattern + exchangeName + routingKey
5. Verify exchangeKind, queueName (or queueNamePrefix), and ephemeral fields

### cloudevents.json

CloudEvents header validation and metadata extraction test cases, plus the message format specification.

**`validateHeaders`** — Tests for CE header validation (conformance level: MUST).

```json
{
  "name": "all required headers present",
  "conformance": "MUST",
  "headers": { "ce-specversion": "1.0", "ce-type": "Order.Created", ... },
  "expectedWarnings": []
}
```

When `expectedWarnings` is empty, validation MUST produce no warnings. When non-empty, the warnings list MUST match exactly (same count, same messages in order).

**`metadataFromHeaders`** — Tests for extracting structured metadata from CE headers.

```json
{
  "name": "all CE headers present",
  "headers": { "ce-id": "abc-123", "ce-time": "2025-06-15T10:30:00Z", ... },
  "expected": {
    "id": "abc-123",
    "timestamp": "2025-06-15T10:30:00Z",
    "source": "orders-svc",
    ...
  }
}
```

Timestamp is an RFC 3339 string. Implementations MUST validate the `ce-time` value as RFC 3339 — if parsing fails (e.g., `"not-a-time"`), the `timestamp` field MUST be empty string. Non-string header values MUST be ignored (treated as missing).

**`messageFormat`** — Declarative specification of the message wire format:
- `contentMode`: always `"binary"` (CE attributes as headers, data as body)
- `required`: headers that MUST be present on every published message
- `optional`: headers that SHOULD be present when applicable
- `extensions`: custom extension headers (e.g., correlation ID)
- `bodyEncoding`: describes the message body format

## Using Fixtures in Go

The `spectest` package provides helpers:

```go
import "github.com/sparetimecoders/messaging/spectest"

scenarios := spectest.LoadScenarios(t, "path/to/topology.json")
for _, sc := range scenarios {
    expected := sc.ExpectedEndpoints["amqp"]
    actual := buildTopology(sc)
    spectest.AssertTopology(t, expected, actual)
}
```

## Using Fixtures in Other Languages

1. Parse the JSON fixture file
2. Iterate over test cases
3. Call your implementation's equivalent function with the input
4. Assert the output matches the expected value

The fixture files are the single source of truth. If your implementation disagrees with a fixture, your implementation has a bug.

## Regenerating Fixtures

Fixtures are generated from Go source code to ensure they stay in sync with the reference implementation:

```bash
go test -run TestGenerateFixtures ./...
```

If a fixture file is out of date, the test will regenerate it and fail. Re-run to confirm stability.
