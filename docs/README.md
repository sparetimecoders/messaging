# messaging Documentation

Documentation for transport implementors. For end-user guides, see the individual transport repos.

| Guide | Description |
|-------|-------------|
| [Communication Patterns](patterns.md) | The five messaging patterns and broker mappings |
| [Naming Conventions](naming.md) | How exchanges, queues, and streams are named |
| [CloudEvents](cloudevents.md) | CloudEvents 1.0 metadata on every message |
| [Topology Tools](topology.md) | Validation, visualization, and broker discovery |
| [Specification Reference](specification.md) | Formal rules, types, and contracts |
| [Implementing a Transport](implementing.md) | Step-by-step guide to building a conformant transport |
| [TCK Guide](tck.md) | Running and writing adapters for the conformance test suite |
| [Test Fixtures](fixtures.md) | Shared JSON test fixtures that define expected behavior |

## Ecosystem

```
messaging (this repo)            ← specification, TCK, tooling
  ├── go-messaging-amqp          ← Go AMQP/RabbitMQ transport
  ├── go-messaging-nats          ← Go NATS/JetStream transport
  ├── nodejs-messaging-amqp      ← TypeScript AMQP transport
  └── nodejs-messaging-nats      ← TypeScript NATS transport
```

All transports share the same specification, pass the same conformance tests, and produce identical topologies for the same service definitions.
