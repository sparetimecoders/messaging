# Getting Started

messaging provides a unified API for event-driven microservices across AMQP/RabbitMQ and NATS/JetStream. The API is nearly identical across transports and languages, so switching brokers requires minimal code changes.

## Installation

### Go

```sh
# AMQP transport
go get github.com/sparetimecoders/go-messaging-amqp

# NATS transport
go get github.com/sparetimecoders/go-messaging-nats

# Messaging library (naming, validation, visualization — no transport dependency)
go get github.com/sparetimecoders/messaging
```

### Node.js / TypeScript

```sh
# AMQP transport
npm install @gomessaging/amqp

# NATS transport
npm install @gomessaging/nats

# Spec library only
npm install @gomessaging/spec
```

## Your First Service

Every service needs a **name** and a **broker connection**. The name determines how all exchanges, queues, and streams are named — services discover each other automatically through these naming conventions.

### Go + AMQP

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/sparetimecoders/go-messaging-amqp"
    "github.com/sparetimecoders/messaging"
)

type OrderCreated struct {
    OrderID string  `json:"orderId"`
    Amount  float64 `json:"amount"`
}

func main() {
    ctx := context.Background()

    // Create a publisher (will be wired during Start)
    pub := amqp.NewPublisher()

    // Connect and declare topology
    conn, err := amqp.NewFromURL("order-service", "amqp://localhost:5672/")
    if err != nil {
        log.Fatal(err)
    }

    err = conn.Start(ctx,
        // Publish events to the default topic exchange
        amqp.EventStreamPublisher(pub),

        // Consume events by routing key
        amqp.EventStreamConsumer("Order.Created",
            func(ctx context.Context, e messaging.ConsumableEvent[OrderCreated]) error {
                fmt.Printf("Received order %s (amount: %.2f) from %s\n",
                    e.Payload.OrderID, e.Payload.Amount, e.Source)
                return nil
            }),
    )
    if err != nil {
        log.Fatal(err)
    }

    // Publish an event
    err = pub.Publish(ctx, "Order.Created", OrderCreated{
        OrderID: "abc-123",
        Amount:  42.00,
    })
    if err != nil {
        log.Fatal(err)
    }
}
```

### Go + NATS

Swap the import and connection — the rest is the same:

```go
conn, err := nats.NewConnection("order-service", "nats://localhost:4222")
if err != nil {
    log.Fatal(err)
}

err = conn.Start(ctx,
    nats.EventStreamPublisher(pub),
    nats.EventStreamConsumer("Order.Created", handler),
)
```

### Node.js + AMQP

```typescript
import { Connection } from "@gomessaging/amqp";

interface OrderCreated {
  orderId: string;
  amount: number;
}

const conn = new Connection({
  url: "amqp://localhost:5672",
  serviceName: "order-service",
});

// Add a publisher
const pub = conn.addEventPublisher();

// Add a consumer
conn.addEventConsumer<OrderCreated>("Order.Created", async (event) => {
  console.log(`Order ${event.payload.orderId} (${event.payload.amount}) from ${event.source}`);
});

await conn.start();

// Publish an event
await pub.publish("Order.Created", { orderId: "abc-123", amount: 42 });
```

## What Happens Under the Hood

When you call `Start()`, messaging:

1. **Declares topology** — creates exchanges, queues, streams, and bindings using [deterministic naming](naming.md)
2. **Sets up consumers** — binds handler functions to queues/subjects with the specified routing keys
3. **Wires publishers** — connects publisher objects to the correct exchange/subject

When you call `Publish()`:

1. **Serializes payload** to JSON
2. **Attaches CloudEvents headers** — `ce-id`, `ce-type`, `ce-source`, `ce-time`, `ce-specversion` ([details](cloudevents.md))
3. **Routes message** through the exchange to matching consumers

## Running a Broker Locally

### RabbitMQ

```sh
docker run -d --name rabbitmq \
  -p 5672:5672 -p 15672:15672 \
  rabbitmq:4-management
```

Management UI at http://localhost:15672 (guest/guest).

### NATS with JetStream

```sh
docker run -d --name nats \
  -p 4222:4222 -p 8222:8222 \
  nats:latest -js -m 8222
```

Monitoring at http://localhost:8222.

## Next Steps

- [Communication Patterns](patterns.md) — learn the five messaging patterns
- [Naming Conventions](naming.md) — understand how resources are named
- [Topology Tools](topology.md) — validate and visualize your message flows
