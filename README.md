# messaging

<p align="center">
  <strong>Opinionated multi-transport messaging with shared specification, conformance testing, and topology visualization.</strong>
</p>

<p align="center">
  <a href="https://github.com/sparetimecoders/messaging/actions"><img alt="CI" src="https://github.com/sparetimecoders/messaging/actions/workflows/ci.yml/badge.svg"></a>
  <a href="https://github.com/sparetimecoders/messaging/releases"><img alt="GitHub release" src="https://img.shields.io/github/v/release/sparetimecoders/messaging"></a>
  <a href="https://pkg.go.dev/github.com/sparetimecoders/messaging"><img alt="Go Reference" src="https://pkg.go.dev/badge/github.com/sparetimecoders/messaging.svg"></a>
  <a href="https://www.npmjs.com/package/@sparetimecoders/messaging"><img alt="npm" src="https://img.shields.io/npm/v/@sparetimecoders/messaging"></a>
  <a href="LICENSE"><img alt="License: MIT" src="https://img.shields.io/badge/license-MIT-blue.svg"></a>
</p>

---

This repo contains the **shared specification and tooling** for messaging &mdash; a multi-transport messaging library for event-driven microservices. It provides:

- **Deterministic naming** &mdash; exchange, queue, and stream names derive from service name + pattern
- **Topology validation** &mdash; catch wiring errors before deployment
- **Topology visualization** &mdash; auto-generated Mermaid diagrams of your message flows
- **CloudEvents 1.0** &mdash; all messages carry standardized metadata
- **Conformance testing** &mdash; the TCK proves transport implementations are correct

This repo is for **transport implementors**. For end-user guides, see the transport repos below.

## Ecosystem

| Package | Language | Description |
|---------|----------|-------------|
| [`messaging`](https://github.com/sparetimecoders/messaging) | Go | Shared library, TCK runner, validation, visualization |
| [`@sparetimecoders/messaging`](https://github.com/sparetimecoders/messaging/tree/main/typescript) | TypeScript | Shared library (mirrors Go) |
| [`go-messaging-amqp`](https://github.com/sparetimecoders/go-messaging-amqp) | Go | AMQP/RabbitMQ transport |
| [`go-messaging-nats`](https://github.com/sparetimecoders/go-messaging-nats) | Go | NATS/JetStream transport |
| [`@sparetimecoders/messaging-amqp`](https://github.com/sparetimecoders/nodejs-messaging-amqp) | TypeScript | AMQP/RabbitMQ transport |
| [`@sparetimecoders/messaging-nats`](https://github.com/sparetimecoders/nodejs-messaging-nats) | TypeScript | NATS/JetStream transport |

## Documentation

See the [docs/](docs/) directory for detailed guides:

| Guide | Description |
|-------|-------------|
| [Communication Patterns](docs/patterns.md) | The five messaging patterns and broker mappings |
| [Naming Conventions](docs/naming.md) | How exchanges, queues, and streams are named |
| [CloudEvents](docs/cloudevents.md) | CloudEvents 1.0 metadata on every message |
| [Topology Tools](docs/topology.md) | Validation, visualization, and broker discovery |
| [Specification Reference](docs/specification.md) | Formal rules, types, and contracts |
| [Implementing a Transport](docs/implementing.md) | Step-by-step guide to building a conformant transport |
| [TCK Guide](docs/tck.md) | Running and writing adapters for the conformance test suite |
| [Test Fixtures](docs/fixtures.md) | Shared JSON test fixtures that define expected behavior |

## Project Structure

```
.
├── *.go                  Go shared library (naming, topology, validation, CloudEvents, visualization)
├── spectest/             Conformance test helpers and assertion functions
├── tck/                  Technology Compatibility Kit (runner, protocol, broker access)
│   ├── adapterutil/      Reusable JSON-RPC handler for adapter implementations
│   └── cmd/tck-runner/   TCK CLI runner
├── specverify/           CLI for topology validation and visualization
├── testdata/             Shared JSON test fixtures (canonical specification)
├── typescript/           TypeScript shared messaging library (mirrors Go)
└── docs/                 Implementor documentation
```

## Development

```sh
# Run Go tests
go test ./...

# Run TCK (requires running brokers)
cd tck && docker compose up -d
go run ./cmd/tck-runner/ --adapter path/to/adapter

# Regenerate fixtures
go test -run TestGenerateFixtures ./...

# TypeScript tests
cd typescript && npm install && npm test
```

## License

MIT
