import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";
import {
  metadataFromHeaders,
  validateCEHeaders,
} from "../src/cloudevents.js";
import type { Headers } from "../src/types.js";

const fixturesPath = resolve(
  import.meta.dirname,
  "../../../../specification/spec/testdata/cloudevents.json",
);
const fixtures = JSON.parse(readFileSync(fixturesPath, "utf-8"));

describe("validateCEHeaders", () => {
  for (const tc of fixtures.validateHeaders) {
    it(tc.name, () => {
      const warnings = validateCEHeaders(tc.headers as Headers);
      expect(warnings).toHaveLength(tc.expectedWarnings.length);
      for (const expected of tc.expectedWarnings) {
        expect(warnings).toContainEqual(expected);
      }
    });
  }
});

describe("metadataFromHeaders", () => {
  for (const tc of fixtures.metadataFromHeaders) {
    it(tc.name, () => {
      const m = metadataFromHeaders(tc.headers as Headers);
      expect(m.id).toBe(tc.expected.id);
      expect(m.source).toBe(tc.expected.source);
      expect(m.type).toBe(tc.expected.type);
      expect(m.subject).toBe(tc.expected.subject);
      expect(m.dataContentType).toBe(tc.expected.dataContentType);
      expect(m.specVersion).toBe(tc.expected.specVersion);
      expect(m.correlationId).toBe(tc.expected.correlationId);
      expect(m.timestamp).toBe(tc.expected.timestamp);
    });
  }
});
