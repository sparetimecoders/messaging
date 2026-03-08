import { describe, expect, it } from "bun:test";
import { matchRoutingKey, routingKeyOverlaps } from "../src/routing.js";

describe("matchRoutingKey", () => {
  const cases: Array<{ name: string; pattern: string; routingKey: string; expected: boolean }> = [
    { name: "exact match", pattern: "Order.Created", routingKey: "Order.Created", expected: true },
    { name: "no match", pattern: "Order.Created", routingKey: "Order.Deleted", expected: false },
    { name: "star wildcard matches single", pattern: "Order.*", routingKey: "Order.Created", expected: true },
    { name: "star wildcard does not match multi", pattern: "Order.*", routingKey: "Order.Created.V2", expected: false },
    { name: "hash wildcard matches single", pattern: "Order.#", routingKey: "Order.Created", expected: true },
    { name: "hash wildcard matches multi", pattern: "Order.#", routingKey: "Order.Created.V2", expected: true },
    { name: "hash wildcard matches everything", pattern: "#", routingKey: "Order.Created", expected: true },
    { name: "star in middle", pattern: "*.Created", routingKey: "Order.Created", expected: true },
    { name: "star in middle no match", pattern: "*.Created", routingKey: "Order.Deleted", expected: false },
    { name: "invalid regex pattern", pattern: "[", routingKey: "anything", expected: false },
  ];

  for (const tc of cases) {
    it(tc.name, () => {
      expect(matchRoutingKey(tc.pattern, tc.routingKey)).toBe(tc.expected);
    });
  }
});

describe("routingKeyOverlaps", () => {
  const cases: Array<{ name: string; p1: string; p2: string; expected: boolean }> = [
    { name: "identical patterns", p1: "Order.Created", p2: "Order.Created", expected: true },
    { name: "no overlap", p1: "Order.Created", p2: "Order.Deleted", expected: false },
    { name: "wildcard overlaps exact", p1: "Order.#", p2: "Order.Created", expected: true },
    { name: "exact overlaps wildcard", p1: "Order.Created", p2: "Order.#", expected: true },
    { name: "star overlaps exact", p1: "Order.*", p2: "Order.Created", expected: true },
    { name: "non-overlapping wildcards", p1: "Order.*", p2: "User.*", expected: false },
    { name: "hash overlaps star", p1: "Order.#", p2: "Order.*", expected: true },
    { name: "invalid regex", p1: "[", p2: "Order.Created", expected: false },
  ];

  for (const tc of cases) {
    it(tc.name, () => {
      expect(routingKeyOverlaps(tc.p1, tc.p2)).toBe(tc.expected);
    });
  }
});
