import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";
import { validate, validateTopologies } from "../src/validate.js";
import type { Topology } from "../src/types.js";

const fixturesPath = resolve(
  import.meta.dirname,
  "../../testdata/validate.json",
);
const fixtures = JSON.parse(readFileSync(fixturesPath, "utf-8"));

describe("validate", () => {
  describe("single topology", () => {
    for (const tc of fixtures.single) {
      it(tc.name, () => {
        const result = validate(tc.topology as Topology);
        if (tc.expectedError === null) {
          expect(result).toBeNull();
        } else {
          expect(result).not.toBeNull();
          expect(result).toContain(tc.expectedError);
        }
      });
    }
  });

  describe("cross-topology validation", () => {
    for (const tc of fixtures.cross) {
      it(tc.name, () => {
        const result = validateTopologies(tc.topologies as Topology[]);
        if (tc.expectedError === null) {
          expect(result).toBeNull();
        } else {
          expect(result).not.toBeNull();
          expect(result).toContain(tc.expectedError);
        }
      });
    }
  });
});
