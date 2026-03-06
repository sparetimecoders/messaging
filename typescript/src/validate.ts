// MIT License
// Copyright (c) 2026 sparetimecoders

import type { Endpoint, Topology, Transport } from "./types.js";

/**
 * Validate checks a single service's topology for internal consistency.
 * Returns null if valid, or an error message string if invalid.
 */
export function validate(t: Topology): string | null {
  if (!t.serviceName) {
    return "service name must not be empty";
  }

  const errs: string[] = [];

  for (let i = 0; i < (t.endpoints ?? []).length; i++) {
    const ep = t.endpoints[i];

    if (!ep.exchangeName) {
      errs.push(`endpoint[${i}]: exchange name must not be empty`);
    }

    if (ep.direction === "consume" && !ep.queueName && !ep.ephemeral) {
      errs.push(
        `endpoint[${i}]: consume endpoint must have a queue name`,
      );
    }

    if (!ep.routingKey && ep.exchangeKind !== "headers") {
      errs.push(
        `endpoint[${i}]: routing key must not be empty for ${ep.exchangeKind} exchange`,
      );
    }

    if (ep.routingKey && ep.routingKey.includes(">")) {
      errs.push(
        `endpoint[${i}]: routing key must not contain '>' (use '#' for multi-level wildcard)`,
      );
    }

    const transportErr = validateExchangeNameForTransport(
      t.transport,
      ep,
      i,
    );
    if (transportErr) {
      errs.push(transportErr);
    }
  }

  return errs.length > 0 ? errs.join("\n") : null;
}

function validateExchangeNameForTransport(
  transport: Transport | undefined,
  ep: Endpoint,
  index: number,
): string | null {
  switch (transport) {
    case "amqp":
      return validateAMQPExchangeName(ep, index);
    case "nats":
      return validateNATSExchangeName(ep, index);
    default:
      return null;
  }
}

function validateAMQPExchangeName(
  ep: Endpoint,
  index: number,
): string | null {
  const name = ep.exchangeName;
  switch (ep.exchangeKind) {
    case "topic":
      if (!name.endsWith(".topic.exchange")) {
        return `endpoint[${index}]: topic exchange "${name}" must end with .topic.exchange`;
      }
      break;
    case "direct":
      if (!name.endsWith(".direct.exchange.request")) {
        return `endpoint[${index}]: direct exchange "${name}" must end with .direct.exchange.request`;
      }
      break;
    case "headers":
      if (!name.endsWith(".headers.exchange.response")) {
        return `endpoint[${index}]: headers exchange "${name}" must end with .headers.exchange.response`;
      }
      break;
  }
  return null;
}

function validateNATSExchangeName(
  ep: Endpoint,
  index: number,
): string | null {
  const name = ep.exchangeName;
  const amqpSuffixes = [
    ".topic.exchange",
    ".direct.exchange.request",
    ".headers.exchange.response",
  ];
  for (const suffix of amqpSuffixes) {
    if (name.endsWith(suffix)) {
      return `endpoint[${index}]: NATS exchange "${name}" must not use AMQP suffix "${suffix}"`;
    }
  }
  if (/[\s]/.test(name)) {
    return `endpoint[${index}]: NATS exchange "${name}" must not contain whitespace`;
  }
  return null;
}

/**
 * ValidateTopologies checks cross-service consistency across multiple topologies.
 * Returns null if valid, or an error message string if invalid.
 */
export function validateTopologies(topologies: Topology[]): string | null {
  const errs: string[] = [];

  for (const t of topologies) {
    const err = validate(t);
    if (err) {
      errs.push(`service "${t.serviceName}": ${err}`);
    }
  }

  // Group topologies by transport for cross-validation.
  const groups = new Map<string, Topology[]>();
  for (const t of topologies) {
    const key = t.transport ?? "";
    const group = groups.get(key) ?? [];
    group.push(t);
    groups.set(key, group);
  }

  for (const group of groups.values()) {
    const err = crossValidateGroup(group);
    if (err) {
      errs.push(err);
    }
  }

  return errs.length > 0 ? errs.join("\n") : null;
}

function crossValidateGroup(topologies: Topology[]): string | null {
  const errs: string[] = [];

  // Build a set of all published routing keys per exchange.
  const published = new Map<string, string>(); // "exchange|routingKey" -> service name
  for (const t of topologies) {
    for (const ep of t.endpoints ?? []) {
      if (ep.direction === "publish" && ep.routingKey) {
        published.set(
          `${ep.exchangeName}|${ep.routingKey}`,
          t.serviceName,
        );
      }
    }
  }

  // Check that each consumer has a matching publisher.
  for (const t of topologies) {
    for (const ep of t.endpoints ?? []) {
      if (ep.direction !== "consume") continue;
      if (!ep.routingKey) continue;
      // Skip wildcard consumers.
      if (/[#*>]/.test(ep.routingKey)) continue;

      const key = `${ep.exchangeName}|${ep.routingKey}`;
      if (!published.has(key)) {
        errs.push(
          `service "${t.serviceName}" consumes "${ep.routingKey}" on exchange "${ep.exchangeName}" but no service publishes it`,
        );
      }
    }
  }

  return errs.length > 0 ? errs.join("\n") : null;
}
