# @gomessaging/spec

TypeScript implementation of the [messaging specification](https://github.com/sparetimecoders/messaging). This package mirrors the Go messaging library, providing identical naming functions, validation, CloudEvents handling, routing, and visualization for Node.js/TypeScript transport implementations.

## Installation

```sh
npm install @gomessaging/spec
```

## Usage

```typescript
import {
  // Naming
  topicExchangeName,
  serviceEventQueueName,
  natsStreamName,
  natsSubject,

  // CloudEvents
  validateCEHeaders,
  metadataFromHeaders,
  normalizeCEHeaders,
  hasCEHeaders,
  enrichLegacyMetadata,

  // Validation
  validate,
  validateTopologies,

  // Visualization
  mermaid,

  // Routing
  matchRoutingKey,
  routingKeyOverlaps,

  // Types
  type Topology,
  type Endpoint,
  type ConsumableEvent,
  type Metadata,
  type EventHandler,
} from "@gomessaging/spec";
```

### Naming

Deterministic resource names for AMQP and NATS:

```typescript
topicExchangeName("events");
// "events.topic.exchange"

serviceEventQueueName("events.topic.exchange", "notifications");
// "events.topic.exchange.queue.notifications"

natsStreamName("audit.topic.exchange");
// "audit"

natsSubject("events", "Order.Created");
// "events.Order.Created"
```

### Validation

Validate service topologies statically:

```typescript
const errors = validate(topology);

// Cross-validate multiple services
const errors = validateTopologies([orderTopology, notificationTopology]);
```

### CloudEvents

Parse and validate CloudEvents headers:

```typescript
const warnings = validateCEHeaders(headers);
const metadata = metadataFromHeaders(headers);

// Normalize AMQP prefixes (cloudEvents:type → ce-type)
const normalized = normalizeCEHeaders(headers);

// Enrich legacy messages without CE headers
const enriched = enrichLegacyMetadata(metadata, deliveryInfo, () => crypto.randomUUID());
```

### Visualization

Generate Mermaid diagrams:

```typescript
const diagram = mermaid([orderTopology, notificationTopology]);
// Returns a Mermaid flowchart string
```

### Routing

Match routing keys with AMQP-style wildcards:

```typescript
matchRoutingKey("Order.*", "Order.Created");   // true
matchRoutingKey("Order.*", "Order.Item.Added"); // false
routingKeyOverlaps("Order.*", "Order.Created"); // true
```

## API Reference

### Naming Functions

| Function | Signature | Description |
|----------|-----------|-------------|
| `topicExchangeName` | `(name: string) => string` | Topic exchange name |
| `serviceEventQueueName` | `(exchange: string, service: string) => string` | Event consumer queue |
| `serviceRequestExchangeName` | `(service: string) => string` | Request exchange |
| `serviceResponseExchangeName` | `(service: string) => string` | Response exchange |
| `serviceRequestQueueName` | `(service: string) => string` | Request queue |
| `serviceResponseQueueName` | `(target: string, service: string) => string` | Response queue |
| `natsStreamName` | `(name: string) => string` | NATS stream name |
| `natsSubject` | `(stream: string, routingKey: string) => string` | NATS subject |
| `translateWildcard` | `(routingKey: string) => string` | AMQP `#` to NATS `>` |

### Validation Functions

| Function | Signature | Description |
|----------|-----------|-------------|
| `validate` | `(topology: Topology) => string \| null` | Single-service validation |
| `validateTopologies` | `(topologies: Topology[]) => string \| null` | Cross-service validation |

### CloudEvents Functions

| Function | Signature | Description |
|----------|-----------|-------------|
| `validateCEHeaders` | `(headers: Headers) => string[]` | Validate CE headers, returns warnings |
| `metadataFromHeaders` | `(headers: Headers) => Metadata` | Parse headers into Metadata |
| `normalizeCEHeaders` | `(headers: Headers) => Headers` | Normalize prefix to `ce-` |
| `hasCEHeaders` | `(headers: Headers) => boolean` | Check if any CE headers present |
| `enrichLegacyMetadata` | `(m: Metadata, d: DeliveryInfo, idGen: () => string) => Metadata` | Enrich missing fields |

### Core Types

```typescript
type Transport = "amqp" | "nats";
type EndpointDirection = "publish" | "consume";
type ExchangeKind = "topic" | "direct" | "headers";
type Pattern = "event-stream" | "custom-stream" | "service-request" | "service-response" | "queue-publish";

interface Endpoint {
  direction: EndpointDirection;
  pattern: Pattern;
  exchangeName: string;
  exchangeKind: ExchangeKind;
  queueName?: string;
  routingKey?: string;
  messageType?: string;
  ephemeral?: boolean;
}

interface Topology {
  transport: Transport;
  serviceName: string;
  endpoints: Endpoint[];
}

interface ConsumableEvent<T> extends Metadata {
  deliveryInfo: DeliveryInfo;
  payload: T;
}
```

## Conformance

This package is tested against the shared JSON fixtures in [`testdata/`](../testdata/). The same fixtures are used by the Go messaging library, ensuring both implementations produce identical outputs.

```sh
npm test
```

## License

MIT
