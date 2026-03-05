import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";
import {
  topicExchangeName,
  serviceEventQueueName,
  serviceRequestExchangeName,
  serviceResponseExchangeName,
  serviceRequestQueueName,
  serviceResponseQueueName,
  natsStreamName,
  natsSubject,
  translateWildcard,
} from "../src/naming.js";

const fixturesPath = resolve(
  import.meta.dirname,
  "../../../../specification/spec/testdata/naming.json",
);
const fixtures = JSON.parse(readFileSync(fixturesPath, "utf-8"));

describe("naming", () => {
  describe("topicExchangeName", () => {
    for (const tc of fixtures.topicExchangeName) {
      it(`${tc.input.name} -> ${tc.expected}`, () => {
        expect(topicExchangeName(tc.input.name)).toBe(tc.expected);
      });
    }
  });

  describe("serviceEventQueueName", () => {
    for (const tc of fixtures.serviceEventQueueName) {
      it(`${tc.input.exchangeName}, ${tc.input.service} -> ${tc.expected}`, () => {
        expect(
          serviceEventQueueName(tc.input.exchangeName, tc.input.service),
        ).toBe(tc.expected);
      });
    }
  });

  describe("serviceRequestExchangeName", () => {
    for (const tc of fixtures.serviceRequestExchangeName) {
      it(`${tc.input.service} -> ${tc.expected}`, () => {
        expect(serviceRequestExchangeName(tc.input.service)).toBe(
          tc.expected,
        );
      });
    }
  });

  describe("serviceResponseExchangeName", () => {
    for (const tc of fixtures.serviceResponseExchangeName) {
      it(`${tc.input.service} -> ${tc.expected}`, () => {
        expect(serviceResponseExchangeName(tc.input.service)).toBe(
          tc.expected,
        );
      });
    }
  });

  describe("serviceRequestQueueName", () => {
    for (const tc of fixtures.serviceRequestQueueName) {
      it(`${tc.input.service} -> ${tc.expected}`, () => {
        expect(serviceRequestQueueName(tc.input.service)).toBe(tc.expected);
      });
    }
  });

  describe("serviceResponseQueueName", () => {
    for (const tc of fixtures.serviceResponseQueueName) {
      it(`${tc.input.targetService}, ${tc.input.serviceName} -> ${tc.expected}`, () => {
        expect(
          serviceResponseQueueName(
            tc.input.targetService,
            tc.input.serviceName,
          ),
        ).toBe(tc.expected);
      });
    }
  });

  describe("natsStreamName", () => {
    for (const tc of fixtures.natsStreamName) {
      it(`${tc.input.name} -> ${tc.expected}`, () => {
        expect(natsStreamName(tc.input.name)).toBe(tc.expected);
      });
    }
  });

  describe("natsSubject", () => {
    for (const tc of fixtures.natsSubject) {
      it(`${tc.input.stream}, ${tc.input.routingKey} -> ${tc.expected}`, () => {
        expect(natsSubject(tc.input.stream, tc.input.routingKey)).toBe(
          tc.expected,
        );
      });
    }
  });

  describe("translateWildcard", () => {
    for (const tc of fixtures.translateWildcard) {
      it(`${tc.input.routingKey} -> ${tc.expected}`, () => {
        expect(translateWildcard(tc.input.routingKey)).toBe(tc.expected);
      });
    }
  });
});
