// MIT License
// Copyright (c) 2026 sparetimecoders

import type { Headers, Metadata } from "./types.js";

/** CloudEvents attribute header keys for binary content mode. */
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

/** CERequiredAttributes lists header keys required by CE 1.0. */
export const CERequiredAttributes = [
  CESpecVersion,
  CEType,
  CESource,
  CEID,
  CETime,
];

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
