import { describe, expect, it } from "vitest";
import { mapRoutingKey } from "../src/metrics.js";

describe("mapRoutingKey", () => {
  it("returns identity when no mapper provided", () => {
    expect(mapRoutingKey("order.created")).toBe("order.created");
  });

  it("applies mapper function", () => {
    const mapper = (key: string) => key.toUpperCase();
    expect(mapRoutingKey("order.created", mapper)).toBe("ORDER.CREATED");
  });

  it("replaces empty mapped value with 'unknown'", () => {
    const mapper = () => "";
    expect(mapRoutingKey("order.created", mapper)).toBe("unknown");
  });

  it("passes through non-empty mapped values", () => {
    const mapper = (key: string) => key.replace(/\.[0-9a-f-]+$/, ".ID");
    expect(mapRoutingKey("order.abc-123", mapper)).toBe("order.ID");
  });
});
