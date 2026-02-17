# gomessaging/spec

The `spec` module defines the shared specification for all gomessaging transport implementations. It contains naming conventions, topology types, validation, CloudEvents support, and visualization — with zero transport dependencies.

```
go get github.com/sparetimecoders/gomessaging/spec
```

## Naming Conventions

All resource names are derived deterministically from service names and pattern types.

### AMQP-style Names

```go
spec.TopicExchangeName("events")              // "events.topic.exchange"
spec.ServiceEventQueueName("events.topic.exchange", "orders") // "events.topic.exchange.queue.orders"
spec.ServiceRequestExchangeName("email-svc")   // "email-svc.direct.exchange.request"
spec.ServiceRequestQueueName("email-svc")      // "email-svc.direct.exchange.request.queue"
spec.ServiceResponseExchangeName("email-svc")  // "email-svc.headers.exchange.response"
spec.ServiceResponseQueueName("email-svc", "web-app") // "email-svc.headers.exchange.response.queue.web-app"
```

### NATS Extensions

```go
spec.NATSStreamName("events")              // "events"
spec.NATSStreamName("events.topic.exchange") // "events" (AMQP suffix stripped)
spec.NATSSubject("events", "Order.Created") // "events.Order.Created"
spec.TranslateWildcard("Order.#")           // "Order.>" (AMQP # -> NATS >)
```

### Complete Naming Reference

| Function | Input | Output |
|----------|-------|--------|
| `TopicExchangeName(name)` | `"events"` | `"events.topic.exchange"` |
| `ServiceEventQueueName(exchange, svc)` | `"events.topic.exchange", "orders"` | `"events.topic.exchange.queue.orders"` |
| `ServiceRequestExchangeName(svc)` | `"email-svc"` | `"email-svc.direct.exchange.request"` |
| `ServiceRequestQueueName(svc)` | `"email-svc"` | `"email-svc.direct.exchange.request.queue"` |
| `ServiceResponseExchangeName(svc)` | `"email-svc"` | `"email-svc.headers.exchange.response"` |
| `ServiceResponseQueueName(target, svc)` | `"email-svc", "web-app"` | `"email-svc.headers.exchange.response.queue.web-app"` |
| `NATSStreamName(name)` | `"events.topic.exchange"` | `"events"` |
| `NATSSubject(stream, key)` | `"events", "Order.Created"` | `"events.Order.Created"` |
| `TranslateWildcard(key)` | `"Order.#"` | `"Order.>"` |

## Types

### Core Types

```go
// Headers are transport message headers.
type Headers map[string]any

// Metadata holds CloudEvents metadata extracted from headers.
type Metadata struct {
    ID              string
    CorrelationID   string
    Timestamp       time.Time
    Source           string
    Type            string
    Subject         string    // optional
    DataContentType string    // optional
    SpecVersion     string
}

// DeliveryInfo holds transport-agnostic delivery metadata.
type DeliveryInfo struct {
    Destination string   // queue/subscription name
    Source      string   // exchange/subject name
    Key         string   // routing key
    Headers     Headers
}

// ConsumableEvent represents an event delivered to a handler.
type ConsumableEvent[T any] struct {
    Metadata
    DeliveryInfo DeliveryInfo
    Payload      T
}
```

### Handler Types

```go
// EventHandler processes events of type T.
type EventHandler[T any] func(ctx context.Context, event ConsumableEvent[T]) error

// RequestResponseEventHandler processes a request of type T and returns a response of type R.
type RequestResponseEventHandler[T any, R any] func(ctx context.Context, event ConsumableEvent[T]) (R, error)
```

### Topology Types

```go
type Transport         string  // "amqp" | "nats"
type EndpointDirection string  // "publish" | "consume"
type ExchangeKind      string  // "topic" | "direct" | "headers"
type Pattern           string  // "event-stream" | "custom-stream" | "service-request" | "service-response" | "queue-publish"

type Endpoint struct {
    Direction    EndpointDirection
    Pattern      Pattern
    ExchangeName string
    ExchangeKind ExchangeKind
    QueueName    string  // consumers only
    RoutingKey   string
    MessageType  string  // optional type hint
    Ephemeral    bool    // transient consumers
}

type Topology struct {
    Transport   Transport
    ServiceName string
    Endpoints   []Endpoint
}
```

### Notification Types

```go
type Notification struct {
    DeliveryInfo DeliveryInfo
    Duration     int64              // nanoseconds
    Source       NotificationSource // "CONSUMER"
}

type ErrorNotification struct {
    Error        error
    DeliveryInfo DeliveryInfo
    Source       NotificationSource
    Duration     int64
}
```

## Communication Patterns

| Pattern | Exchange Kind | Description |
|---------|--------------|-------------|
| `event-stream` | topic | Domain events on the default `events` exchange |
| `custom-stream` | topic | Events on a named exchange (e.g. `audit`) |
| `service-request` | direct | RPC request to a specific service |
| `service-response` | headers | RPC response from a service |
| `queue-publish` | direct | Direct queue publishing |

## CloudEvents

Messages use [CloudEvents 1.0](https://cloudevents.io/) binary content mode. Attributes are carried as message headers.

### Header Constants

```go
spec.CESpecVersion     // "ce-specversion"
spec.CEType            // "ce-type"
spec.CESource          // "ce-source"
spec.CEID              // "ce-id"
spec.CETime            // "ce-time"
spec.CEDataContentType // "ce-datacontenttype" (optional)
spec.CESubject         // "ce-subject" (optional)
spec.CEDataSchema      // "ce-dataschema" (optional)
spec.CECorrelationID   // "ce-correlationid" (extension)
```

### Functions

```go
// Extract metadata from message headers.
metadata := spec.MetadataFromHeaders(headers)

// Validate required CE attributes (returns warning strings).
warnings := spec.ValidateCEHeaders(headers)
```

## Validation

### Single Topology

```go
err := spec.Validate(topology)
```

Checks:
- Service name is not empty
- Exchange names follow naming conventions for the transport
- Consumers have queue names (unless ephemeral)
- Routing keys are present for topic/direct exchanges
- Routing keys don't contain NATS-reserved `>` character

### Cross-Service Validation

```go
err := spec.ValidateTopologies([]spec.Topology{topo1, topo2, topo3})
```

Validates each topology individually, then checks:
- Every consumer routing key has a matching publisher on the same exchange
- Topologies are grouped by transport (no cross-transport matching)
- Wildcard consumers (`#`, `*`, `>`) are skipped in matching

## Visualization

### Mermaid Diagrams

```go
diagram := spec.Mermaid(topologies)
fmt.Print(diagram)
```

Generates a Mermaid LR flowchart with:
- Hexagons for topic exchanges
- Rectangles for direct exchanges
- Circles for headers exchanges
- Color-coded services (pink) and exchanges (blue)

### Broker Discovery

```go
topologies, err := spec.DiscoverTopologies(spec.BrokerConfig{
    URL:      "http://localhost:15672",
    Username: "guest",
    Password: "guest",
    Vhost:    "/",
})
```

Connects to the RabbitMQ Management API to reconstruct per-service topologies from broker state.

## Conformance Test Helpers

The `spec/spectest` sub-package provides helpers for transport implementations:

```go
import "github.com/sparetimecoders/gomessaging/spec/spectest"

func TestTopologyConformance(t *testing.T) {
    scenarios := spectest.LoadScenarios(t, "../spec/testdata/topology.json")
    for _, sc := range scenarios {
        t.Run(sc.Name, func(t *testing.T) {
            expected, ok := sc.ExpectedEndpoints["amqp"]
            if !ok {
                t.Skip("no AMQP expectations")
            }
            topology := buildTopologyFromSetups(sc.ServiceName, sc.Setups)
            spectest.AssertTopology(t, expected, topology)
        })
    }
}
```

See [testdata/README.md](testdata/README.md) for the fixture file format.
