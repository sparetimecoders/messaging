// MIT License
// Copyright (c) 2026 sparetimecoders

import type { DeliveryInfo, Headers, Metadata } from "./types.js";

/** CloudEvents attribute header keys for binary content mode (canonical "ce-" prefix). */
export const CESpecVersion = "ce-specversion";
export const CEType = "ce-type";
export const CESource = "ce-source";
export const CEID = "ce-id";
export const CETime = "ce-time";
export const CESpecVersionValue = "1.0";

/** Optional CE attributes */
export const CEDataContentType = "ce-datacontenttype";
export const CESubject = "ce-subject";
export const CEDataSchema = "ce-dataschema";

/** Extension attribute for correlation */
export const CECorrelationID = "ce-correlationid";

/** Bare CE attribute names (without prefix). */
export const CEAttrSpecVersion = "specversion";
export const CEAttrType = "type";
export const CEAttrSource = "source";
export const CEAttrID = "id";
export const CEAttrTime = "time";
export const CEAttrDataContentType = "datacontenttype";
export const CEAttrSubject = "subject";
export const CEAttrCorrelationID = "correlationid";

/** CERequiredAttributes lists header keys required by CE 1.0. */
export const CERequiredAttributes = [
  CESpecVersion,
  CEType,
  CESource,
  CEID,
  CETime,
];

/**
 * AMQPCEHeaderKey returns the AMQP application-properties key for a bare
 * CloudEvents attribute name, using the "cloudEvents:" prefix per the
 * CloudEvents AMQP Protocol Binding specification.
 * Example: AMQPCEHeaderKey("specversion") -> "cloudEvents:specversion"
 */
export function AMQPCEHeaderKey(attr: string): string {
  return "cloudEvents:" + attr;
}

/**
 * NormalizeCEHeaders rewrites incoming transport headers so that all
 * CloudEvents attributes use the canonical "ce-" prefix. This allows
 * consumers to accept messages with any known prefix variant:
 *   - "cloudEvents:specversion" -> "ce-specversion"
 *   - "cloudEvents_specversion" -> "ce-specversion"  (JMS compat)
 *   - "ce-specversion" -> unchanged
 *
 * Non-CE headers are preserved unchanged. A new object is returned.
 */
export function normalizeCEHeaders(h: Headers): Headers {
  const out: Headers = {};
  for (const [k, v] of Object.entries(h)) {
    if (k.startsWith("cloudEvents:")) {
      out["ce-" + k.slice("cloudEvents:".length)] = v;
    } else if (k.startsWith("cloudEvents_")) {
      out["ce-" + k.slice("cloudEvents_".length)] = v;
    } else {
      out[k] = v;
    }
  }
  return out;
}

/**
 * HasCEHeaders reports whether h contains at least one CE required attribute,
 * checking for "ce-", "cloudEvents:", and "cloudEvents_" prefixes.
 * Use this to distinguish legacy (pre-CloudEvents) messages from malformed CE messages.
 */
export function hasCEHeaders(h: Headers): boolean {
  for (const key of Object.keys(h)) {
    if (
      key.startsWith("ce-") ||
      key.startsWith("cloudEvents:") ||
      key.startsWith("cloudEvents_")
    ) {
      return true;
    }
  }
  return false;
}

/**
 * EnrichLegacyMetadata populates empty Metadata fields with synthetic
 * CloudEvents attributes derived from transport delivery information.
 *
 * Fields set when empty: id (randomUUID), timestamp (now), type (from key),
 * source (from source), dataContentType ("application/json"), specVersion ("1.0").
 *
 * Fields NOT set: correlationId, subject (cannot be inferred).
 * Any non-empty fields in m are preserved (not overwritten).
 * Returns a new Metadata object; the input is not mutated.
 */
export function enrichLegacyMetadata(
  m: Metadata,
  info: DeliveryInfo,
): Metadata {
  return {
    ...m,
    id: m.id || crypto.randomUUID(),
    timestamp: m.timestamp || new Date().toISOString(),
    type: m.type || info.key,
    source: m.source || info.source,
    dataContentType: m.dataContentType || "application/json",
    specVersion: m.specVersion || CESpecVersionValue,
  };
}

/** RFC 3339 regex for timestamp validation. */
const RFC3339_RE =
  /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})$/;

function headerString(h: Headers, key: string): string {
  const v = h[key];
  if (typeof v === "string") {
    return v;
  }
  return "";
}

/**
 * MetadataFromHeaders extracts CloudEvents metadata from message headers
 * into a Metadata struct. Invalid (non-RFC3339) timestamps return empty string.
 */
export function metadataFromHeaders(h: Headers): Metadata {
  const timeStr = headerString(h, CETime);
  return {
    id: headerString(h, CEID),
    source: headerString(h, CESource),
    type: headerString(h, CEType),
    subject: headerString(h, CESubject),
    dataContentType: headerString(h, CEDataContentType),
    specVersion: headerString(h, CESpecVersion),
    correlationId: headerString(h, CECorrelationID),
    timestamp: RFC3339_RE.test(timeStr) ? timeStr : "",
  };
}

/**
 * ValidateCEHeaders checks that all required CloudEvents 1.0 attributes
 * are present and are strings. Returns a list of warnings for any
 * missing or non-string attributes.
 */
export function validateCEHeaders(h: Headers): string[] {
  const warnings: string[] = [];
  for (const key of CERequiredAttributes) {
    if (!(key in h)) {
      warnings.push(`missing required attribute "${key}"`);
      continue;
    }
    if (typeof h[key] !== "string") {
      warnings.push(`attribute "${key}" is not a string`);
    }
  }
  return warnings;
}
