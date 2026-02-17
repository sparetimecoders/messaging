# gomessaging for Node.js

Node.js/TypeScript implementation of the gomessaging specification. Provides the same naming conventions, topology model, CloudEvents support, and validation as the Go implementation.

Requires **Node.js >= 22 LTS** (recommended; minimum >= 21.0.0 due to `import.meta.dirname`).

## Packages

| Package | Description | Status |
|---------|-------------|--------|
| `@gomessaging/spec` | Naming, types, validation, CloudEvents | Complete (123 tests passing) |
| `@gomessaging/amqp` | AMQP transport (RabbitMQ) | Topology registration complete; broker wiring planned |
| `@gomessaging/nats` | NATS transport (JetStream + Core) | Topology registration complete; broker wiring planned |

## Quick Start

```bash
npm install @gomessaging/amqp
# or
npm install @gomessaging/nats
```

### AMQP

```typescript
import { Connection } from "@gomessaging/amqp";
import type { ConsumableEvent } from "@gomessaging/spec";

interface OrderCreated {
  orderId: string;
  customer: string;
  total: number;
}

const conn = new Connection({
  url: "amqp://localhost",
  serviceName: "order-service",
});

// Register consumers and publishers
conn.addEventPublisher();
conn.addEventConsumer<OrderCreated>("Order.Created", async (event) => {
  console.log("received order", event.payload.orderId);
});

await conn.start();
// ... publish events, handle messages ...
await conn.close();
```

### NATS

```typescript
import { Connection } from "@gomessaging/nats";
import type { ConsumableEvent } from "@gomessaging/spec";

interface OrderCreated {
  orderId: string;
  customer: string;
  total: number;
}

const conn = new Connection({
  url: "nats://localhost:4222",
  serviceName: "order-service",
});

conn.addEventPublisher();
conn.addEventConsumer<OrderCreated>("Order.Created", async (event) => {
  console.log("received order", event.payload.orderId);
});

await conn.start();
// ... publish events, handle messages ...
await conn.close();
```

## Spec Package

The `@gomessaging/spec` package provides the shared specification:

### Naming Functions

```typescript
import {
  topicExchangeName,
  serviceEventQueueName,
  serviceRequestExchangeName,
  serviceResponseExchangeName,
  serviceRequestQueueName,
  serviceResponseQueueName,
  natsStreamName,
  natsSubject,
  translateWildcard,
} from "@gomessaging/spec";

topicExchangeName("events");              // "events.topic.exchange"
serviceEventQueueName("events.topic.exchange", "orders"); // "events.topic.exchange.queue.orders"
natsStreamName("events.topic.exchange");  // "events"
natsSubject("events", "Order.Created");   // "events.Order.Created"
translateWildcard("Order.#");             // "Order.>"
```

### CloudEvents

```typescript
import {
  metadataFromHeaders,
  validateCEHeaders,
  CESpecVersion,
  CEType,
  CESource,
  CEID,
  CETime,
} from "@gomessaging/spec";

// Extract metadata from message headers
const metadata = metadataFromHeaders(headers);

// Validate required CE attributes
const warnings = validateCEHeaders(headers);
```

### Validation

```typescript
import { validate, validateTopologies } from "@gomessaging/spec";

// Validate single service topology — returns string on error, null on success
const error = validate(topology);   // string | null

// Cross-validate multiple services
const crossError = validateTopologies([topo1, topo2, topo3]);  // string | null
```

### Types

```typescript
import type {
  Transport,           // "amqp" | "nats"
  EndpointDirection,   // "publish" | "consume"
  ExchangeKind,        // "topic" | "direct" | "headers"
  Pattern,             // "event-stream" | "custom-stream" | "service-request" | "service-response" | "queue-publish"
  Headers,             // Record<string, unknown>
  Metadata,
  DeliveryInfo,
  ConsumableEvent,
  Endpoint,
  Topology,
  EventHandler,
  RequestResponseEventHandler,
} from "@gomessaging/spec";
```

## AMQP Connection API

```typescript
const conn = new Connection({ url: "amqp://localhost", serviceName: "my-service" });

// Event stream (default "events" exchange)
conn.addEventPublisher();
conn.addEventConsumer<T>(routingKey, handler, { ephemeral?: boolean });

// Custom stream (named exchange)
conn.addCustomStreamPublisher(exchange);
conn.addCustomStreamConsumer<T>(exchange, routingKey, handler);

// Service request-response
conn.addServiceRequestConsumer<T, R>(routingKey, handler);
conn.addServiceRequestPublisher(targetService);
conn.addServiceResponseConsumer<T>(targetService, routingKey, handler);

// Topology export
const topology = conn.topology();

// Lifecycle
await conn.start();
await conn.close();
```

## NATS Connection API

```typescript
const conn = new Connection({ url: "nats://localhost:4222", serviceName: "my-service" });

// Event stream (JetStream)
conn.addEventPublisher();
conn.addEventConsumer<T>(routingKey, handler, { ephemeral?: boolean });

// Custom stream (JetStream)
conn.addCustomStreamPublisher(exchange);
conn.addCustomStreamConsumer<T>(exchange, routingKey, handler);

// Service request-response (Core NATS)
conn.addServiceRequestConsumer<T, R>(routingKey, handler);
conn.addServiceRequestPublisher(targetService);
conn.addServiceResponseConsumer<T>(targetService, routingKey, handler);

// Topology export
const topology = conn.topology();

// Lifecycle
await conn.start();
await conn.close();
```

## Differences from Go

| Aspect | Go | TypeScript |
|--------|-----|-----------|
| Naming style | `TopicExchangeName()` (PascalCase) | `topicExchangeName()` (camelCase) |
| Validation return | `error` (nil = valid) | `string \| null` (null = valid) |
| Timestamps | `time.Time` | ISO 8601 string |
| Event type | `ConsumableEvent[T]` (struct embedding) | `ConsumableEvent<T>` (interface extension) |
| Handler return | `error` | `Promise<void>` (async) |
| Connection constructor | `NewFromURL(name, url)` | `new Connection({ url, serviceName })` |

## Development

```bash
# Install dependencies
npm install

# Run tests
npm test

# Watch mode
npm run test:watch

# Build all packages
npm run build
```

## Conformance Testing

The spec package passes the same conformance tests as the Go implementation, using shared JSON fixtures from `../spec/testdata/`:

- `constants.json` - Constant values
- `naming.json` - Naming function outputs
- `validate.json` - Validation rules
- `topology.json` - Topology endpoint generation
- `cloudevents.json` - CE header validation, metadata extraction, and message format

See [../spec/testdata/README.md](../spec/testdata/README.md) for the fixture file format.
