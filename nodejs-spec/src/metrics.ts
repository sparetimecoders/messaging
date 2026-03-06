// MIT License
// Copyright (c) 2026 sparetimecoders

/**
 * Pluggable metrics interface for messaging adapters.
 *
 * Users wire this to any metrics backend (prom-client, OpenTelemetry,
 * StatsD, etc.) by implementing MetricsRecorder and passing it to
 * ConnectionOptions.
 */

/** Records messaging metrics. Implement this interface to wire any metrics backend. */
export interface MetricsRecorder {
  /** An event was received from the broker. */
  eventReceived(queue: string, routingKey: string): void;

  /** An event was received but no handler matched its routing key. */
  eventWithoutHandler(queue: string, routingKey: string): void;

  /** An event could not be parsed (invalid JSON). */
  eventNotParsable(queue: string, routingKey: string): void;

  /** An event was acknowledged (successfully processed). */
  eventAck(queue: string, routingKey: string, durationMs: number): void;

  /** An event was negatively acknowledged (handler failed). */
  eventNack(queue: string, routingKey: string, durationMs: number): void;

  /** A message was published successfully. */
  publishSucceed(exchange: string, routingKey: string, durationMs: number): void;

  /** A message failed to publish. */
  publishFailed(exchange: string, routingKey: string, durationMs: number): void;
}

/**
 * Maps a routing key before it is passed to metrics.
 * Use this to normalize or redact dynamic segments (e.g. UUIDs)
 * to prevent unbounded label cardinality.
 */
export type RoutingKeyMapper = (key: string) => string;

/** Options for configuring metrics behavior. */
export interface MetricsOptions {
  routingKeyMapper?: RoutingKeyMapper;
}

/**
 * Apply a routing key mapper, defaulting to identity.
 * Empty mapped values are replaced with "unknown".
 */
export function mapRoutingKey(
  key: string,
  mapper?: RoutingKeyMapper,
): string {
  const mapped = mapper ? mapper(key) : key;
  return mapped === "" ? "unknown" : mapped;
}
