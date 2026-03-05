# Test Fixtures

The `testdata/` directory contains JSON fixtures that define expected behavior for all gomessaging implementations. These fixtures are the **canonical specification** — if the code and fixtures disagree, the fixtures are authoritative.

## Fixture Files

| File | Purpose |
|------|---------|
| [`constants.json`](#constantsjson) | Enum values, default names, CloudEvents header keys |
| [`naming.json`](#namingjson) | Naming function inputs and expected outputs |
| [`validate.json`](#validatejson) | Single-service and cross-service validation rules |
| [`topology.json`](#topologyjson) | Setup intents to endpoint generation |
| [`cloudevents.json`](#cloudeventsjson) | CE header validation, metadata extraction, wire format |
| [`tck.json`](#tckjson) | Multi-service integration scenarios |

## constants.json

Defines all enum values and well-known constants. Use this to verify your type definitions match the spec.

```json
{
  "defaultExchangeName": "events",
  "exchangeKinds": ["topic", "direct", "headers"],
  "directions": ["publish", "consume"],
  "patterns": ["event-stream", "custom-stream", "service-request",
               "service-response", "queue-publish"],
  "transports": ["amqp", "nats"],
  "ceAttributes": {
    "specversion": "ce-specversion",
    "type": "ce-type",
    "source": "ce-source",
    ...
  }
}
```

## naming.json

Input/output pairs for every naming function. Each entry has an `input` (function arguments) and `expected` (return value).

```json
{
  "topicExchangeName": [
    { "input": ["events"], "expected": "events.topic.exchange" },
    { "input": ["audit"], "expected": "audit.topic.exchange" }
  ],
  "serviceEventQueueName": [
    {
      "input": ["events.topic.exchange", "notifications"],
      "expected": "events.topic.exchange.queue.notifications"
    }
  ],
  "natsStreamName": [
    { "input": ["audit.topic.exchange"], "expected": "audit" },
    { "input": ["events"], "expected": "events" }
  ]
}
```

### How to Test

```go
// Go
for _, tc := range fixtures.TopicExchangeName {
    got := spec.TopicExchangeName(tc.Input[0])
    assert.Equal(t, tc.Expected, got)
}
```

```typescript
// TypeScript
for (const tc of fixtures.topicExchangeName) {
  expect(topicExchangeName(tc.input[0])).toBe(tc.expected);
}
```

## validate.json

Two sections: `single` (one service) and `cross` (multiple services).

### Single-Service Validation

Each test case has a topology and an expected error (or `null` for valid):

```json
{
  "single": [
    {
      "name": "valid event stream consumer",
      "topology": {
        "transport": "amqp",
        "serviceName": "orders",
        "endpoints": [...]
      },
      "expectedError": null
    },
    {
      "name": "missing queue name on consumer",
      "topology": { ... },
      "expectedError": "queue name required"
    }
  ]
}
```

### Cross-Service Validation

Multiple topologies validated together:

```json
{
  "cross": [
    {
      "name": "consumer with no matching publisher",
      "topologies": [
        { "serviceName": "consumer-svc", "endpoints": [...] },
        { "serviceName": "publisher-svc", "endpoints": [...] }
      ],
      "expectedError": "no publisher found for routing key"
    }
  ]
}
```

## topology.json

Maps setup intents (what the user declares) to expected endpoints (what the transport creates). Includes both AMQP and NATS expected outputs and broker state expectations.

```json
{
  "scenarios": [
    {
      "name": "event stream publisher and consumer",
      "service": "orders",
      "intents": [
        { "pattern": "event-stream", "direction": "publish", "routingKey": "Order.Created" },
        { "pattern": "event-stream", "direction": "consume", "routingKey": "Payment.Completed" }
      ],
      "expectedEndpoints": {
        "amqp": [...],
        "nats": [...]
      },
      "brokerState": {
        "amqp": {
          "exchanges": [...],
          "queues": [...],
          "bindings": [...]
        }
      }
    }
  ]
}
```

## cloudevents.json

Three sections covering CE header handling:

### validateHeaders

```json
{
  "validateHeaders": [
    {
      "name": "all required headers present",
      "headers": {
        "ce-specversion": "1.0",
        "ce-type": "Order.Created",
        "ce-source": "orders",
        "ce-id": "...",
        "ce-time": "2026-01-01T00:00:00Z"
      },
      "expectedWarnings": []
    },
    {
      "name": "missing ce-source",
      "headers": { ... },
      "expectedWarnings": ["missing required CloudEvents attribute: ce-source"]
    }
  ]
}
```

### metadataFromHeaders

Verifies parsing of headers into structured metadata.

### messageFormat

Declarative specification of the wire format — binary content mode, required and optional headers, body encoding rules.

## tck.json

Multi-service integration scenarios used by the [TCK](tck.md). Each scenario defines:

- **Services**: names and setup intents
- **Expected endpoints**: per service, per transport
- **Broker state**: expected exchanges, queues, bindings
- **Messages**: what to publish, where it should be delivered, payload assertions
- **Probe messages**: raw broker messages for cross-validation

These scenarios are loaded by the TCK runner and executed against transport adapters.

## Generating Fixtures

Fixtures are generated from Go source code:

```sh
go test -run TestGenerateFixtures ./...
```

This ensures fixtures stay in sync with the spec implementation. After modifying spec logic, regenerate fixtures and update all transport implementations to pass the new expectations.

## Using Fixtures in Your Language

The fixtures are plain JSON — load them with any JSON parser. The general testing pattern:

1. Load the fixture file
2. Iterate over test cases
3. Call your implementation with the input
4. Assert the output matches expected

The `spectest` package (Go) provides helper functions for loading and asserting against fixtures. For other languages, parse the JSON directly.
