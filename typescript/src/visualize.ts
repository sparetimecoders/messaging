// MIT License
// Copyright (c) 2026 sparetimecoders

import type { ExchangeKind, Topology } from "./types.js";

interface ExchangeEntry {
  kind: ExchangeKind;
  transport: string;
}

interface Edge {
  from: string;
  to: string;
  label: string;
  style: string; // "-->" or "-.->"
  transport: string;
}

function sanitizeID(name: string): string {
  return name.replace(/[^a-zA-Z0-9_]/g, "_");
}

function exchangeNode(
  id: string,
  name: string,
  kind: ExchangeKind,
  indent: string,
): string {
  switch (kind) {
    case "topic":
      return `${indent}${id}{{"${name}"}}\n`;
    case "headers":
      return `${indent}${id}(("${name}"))\n`;
    case "direct":
    default:
      return `${indent}${id}["${name}"]\n`;
  }
}

/**
 * Generates a Mermaid flowchart diagram from one or more topologies.
 * The diagram shows services as rectangle nodes, exchanges as shaped nodes
 * (based on exchange kind), and edges for publish/consume relationships.
 * When topologies use multiple transports, exchanges are grouped into
 * subgraphs by transport (e.g. AMQP, NATS).
 */
export function mermaid(topologies: Topology[]): string {
  const services = new Set<string>();
  const exchanges = new Map<string, ExchangeEntry>();
  const edges: Edge[] = [];
  const transports = new Set<string>();

  for (const t of topologies) {
    if (!t.serviceName) continue;
    services.add(t.serviceName);
    if (t.transport) {
      transports.add(t.transport);
    }

    for (const ep of t.endpoints) {
      if (!ep.exchangeName) continue;
      exchanges.set(ep.exchangeName, {
        kind: ep.exchangeKind,
        transport: t.transport ?? "",
      });

      const arrow = ep.pattern === "service-response" ? "-.->" : "-->";

      if (ep.direction === "publish") {
        edges.push({
          from: t.serviceName,
          to: ep.exchangeName,
          label: ep.routingKey ?? "",
          style: arrow,
          transport: t.transport ?? "",
        });
      } else if (ep.direction === "consume") {
        edges.push({
          from: ep.exchangeName,
          to: t.serviceName,
          label: ep.routingKey ?? "",
          style: arrow,
          transport: t.transport ?? "",
        });
      }
    }
  }

  const useSubgraphs = transports.size > 1;

  const exchID = (transport: string, name: string): string => {
    if (useSubgraphs) {
      return sanitizeID(`${transport}_${name}`);
    }
    return sanitizeID(name);
  };

  let out = "graph LR\n";

  // Service nodes (sorted)
  const sortedServices = [...services].sort();
  for (const svc of sortedServices) {
    out += `    ${sanitizeID(svc)}["${svc}"]\n`;
  }

  const sortedExchangeNames = [...exchanges.keys()].sort();

  if (useSubgraphs) {
    // Group exchanges by transport into subgraphs
    const sortedTransports = [...transports].sort();
    for (const tr of sortedTransports) {
      out += `    subgraph ${tr.toUpperCase()}\n`;
      for (const name of sortedExchangeNames) {
        const entry = exchanges.get(name)!;
        if (entry.transport !== tr) continue;
        out += exchangeNode(exchID(tr, name), name, entry.kind, "        ");
      }
      out += "    end\n";
    }
  } else {
    // Single transport or no transport — flat list (backward compatible)
    for (const name of sortedExchangeNames) {
      const entry = exchanges.get(name)!;
      out += exchangeNode(
        exchID(entry.transport, name),
        name,
        entry.kind,
        "    ",
      );
    }
  }

  // Edges (sorted by from/to/label for deterministic output)
  edges.sort((a, b) => {
    if (a.from !== b.from) return a.from < b.from ? -1 : 1;
    if (a.to !== b.to) return a.to < b.to ? -1 : 1;
    return a.label < b.label ? -1 : a.label > b.label ? 1 : 0;
  });

  for (const e of edges) {
    const fromID = services.has(e.from)
      ? sanitizeID(e.from)
      : exchID(e.transport, e.from);
    const toID = services.has(e.to)
      ? sanitizeID(e.to)
      : exchID(e.transport, e.to);
    if (e.label) {
      out += `    ${fromID} ${e.style}|"${e.label}"| ${toID}\n`;
    } else {
      out += `    ${fromID} ${e.style} ${toID}\n`;
    }
  }

  // Style declarations
  if (sortedServices.length > 0) {
    const ids = sortedServices.map(sanitizeID).join(",");
    out += `    style ${ids} fill:#f9f,stroke:#333\n`;
  }
  if (sortedExchangeNames.length > 0) {
    const ids = sortedExchangeNames
      .map((name) => exchID(exchanges.get(name)!.transport, name))
      .join(",");
    out += `    style ${ids} fill:#bbf,stroke:#333\n`;
  }

  return out;
}
