import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";
import {
  metadataFromHeaders,
  validateCEHeaders,
  hasCEHeaders,
  enrichLegacyMetadata,
  CESpecVersionValue,
} from "../src/cloudevents.js";
import type { DeliveryInfo, Headers, Metadata } from "../src/types.js";

const fixturesPath = resolve(
  import.meta.dirname,
  "../../testdata/cloudevents.json",
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

describe("hasCEHeaders", () => {
  it("returns true when ce- prefixed headers exist", () => {
    expect(hasCEHeaders({ "ce-specversion": "1.0", "x-custom": "val" })).toBe(true);
  });

  it("returns true when cloudEvents: prefixed headers exist", () => {
    expect(hasCEHeaders({ "cloudEvents:type": "order.created" })).toBe(true);
  });

  it("returns true when cloudEvents_ prefixed headers exist", () => {
    expect(hasCEHeaders({ "cloudEvents_source": "my-service" })).toBe(true);
  });

  it("returns false when no CE headers exist", () => {
    expect(hasCEHeaders({ "x-custom": "val", "content-type": "application/json" })).toBe(false);
  });

  it("returns false for empty headers", () => {
    expect(hasCEHeaders({})).toBe(false);
  });
});

describe("enrichLegacyMetadata", () => {
  const emptyMetadata: Metadata = {
    id: "",
    correlationId: "",
    timestamp: "",
    source: "",
    type: "",
    subject: "",
    dataContentType: "",
    specVersion: "",
  };

  const deliveryInfo: DeliveryInfo = {
    destination: "test-queue",
    source: "my-exchange",
    key: "order.created",
    headers: {},
  };

  it("fills in all empty fields with synthetic values", () => {
    const result = enrichLegacyMetadata(emptyMetadata, deliveryInfo);

    expect(result.id).toMatch(
      /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/,
    );
    expect(result.timestamp).toBeTruthy();
    expect(new Date(result.timestamp).getTime()).not.toBeNaN();
    expect(result.type).toBe("order.created");
    expect(result.source).toBe("my-exchange");
    expect(result.dataContentType).toBe("application/json");
    expect(result.specVersion).toBe(CESpecVersionValue);
  });

  it("does not set correlationId or subject", () => {
    const result = enrichLegacyMetadata(emptyMetadata, deliveryInfo);
    expect(result.correlationId).toBe("");
    expect(result.subject).toBe("");
  });

  it("preserves non-empty fields", () => {
    const partial: Metadata = {
      ...emptyMetadata,
      id: "existing-id",
      type: "existing-type",
    };
    const result = enrichLegacyMetadata(partial, deliveryInfo);

    expect(result.id).toBe("existing-id");
    expect(result.type).toBe("existing-type");
    expect(result.source).toBe("my-exchange");
    expect(result.dataContentType).toBe("application/json");
  });

  it("does not mutate the input metadata", () => {
    const original = { ...emptyMetadata };
    enrichLegacyMetadata(original, deliveryInfo);
    expect(original.id).toBe("");
    expect(original.type).toBe("");
  });
});
