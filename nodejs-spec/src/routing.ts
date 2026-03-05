// MIT License
// Copyright (c) 2026 sparetimecoders

/**
 * Converts an AMQP/NATS binding pattern to a regular expression string.
 *
 * - `.` is escaped to `\.`
 * - `*` matches a single word: `[^.]*`
 * - `#` matches zero or more words: `.*`
 */
function routingKeyToRegex(pattern: string): string {
  const escaped = pattern
    .replaceAll(".", "\\.")
    .replaceAll("*", "[^.]*")
    .replaceAll("#", ".*");
  return `^${escaped}$`;
}

/**
 * Returns true if routingKey matches the binding pattern.
 * Supports AMQP/NATS wildcard syntax: `*` matches one word, `#` matches zero or more.
 */
export function matchRoutingKey(pattern: string, routingKey: string): boolean {
  try {
    return new RegExp(routingKeyToRegex(pattern)).test(routingKey);
  } catch {
    return false;
  }
}

/**
 * Returns true if two binding patterns could match the same routing key.
 */
export function routingKeyOverlaps(p1: string, p2: string): boolean {
  if (p1 === p2) return true;
  if (matchRoutingKey(p1, p2)) return true;
  if (matchRoutingKey(p2, p1)) return true;
  return false;
}
