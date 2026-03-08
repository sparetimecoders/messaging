import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "bun:test";
import {
  DefaultEventExchangeName,
  KindDirect,
  KindHeaders,
  KindTopic,
  CECorrelationID,
  CEDataContentType,
  CEDataSchema,
  CEID,
  CESource,
  CESpecVersion,
  CESpecVersionValue,
  CESubject,
  CETime,
  CEType,
} from "../src/index.js";

const fixturesPath = resolve(
  import.meta.dirname,
  "../../testdata/constants.json",
);
const fixtures = JSON.parse(readFileSync(fixturesPath, "utf-8"));

describe("constants", () => {
  it("defaultEventExchangeName", () => {
    expect(DefaultEventExchangeName).toBe(
      fixtures.defaultEventExchangeName,
    );
  });

  describe("exchangeKinds", () => {
    it("direct", () => {
      expect(KindDirect).toBe(fixtures.exchangeKinds.direct);
    });
    it("headers", () => {
      expect(KindHeaders).toBe(fixtures.exchangeKinds.headers);
    });
    it("topic", () => {
      expect(KindTopic).toBe(fixtures.exchangeKinds.topic);
    });
  });

  describe("patterns", () => {
    it("eventStream", () => {
      expect("event-stream" satisfies string).toBe(
        fixtures.patterns.eventStream,
      );
    });
    it("customStream", () => {
      expect("custom-stream" satisfies string).toBe(
        fixtures.patterns.customStream,
      );
    });
    it("serviceRequest", () => {
      expect("service-request" satisfies string).toBe(
        fixtures.patterns.serviceRequest,
      );
    });
    it("serviceResponse", () => {
      expect("service-response" satisfies string).toBe(
        fixtures.patterns.serviceResponse,
      );
    });
    it("queuePublish", () => {
      expect("queue-publish" satisfies string).toBe(
        fixtures.patterns.queuePublish,
      );
    });
  });

  describe("directions", () => {
    it("publish", () => {
      expect("publish" satisfies string).toBe(fixtures.directions.publish);
    });
    it("consume", () => {
      expect("consume" satisfies string).toBe(fixtures.directions.consume);
    });
  });

  describe("transports", () => {
    it("amqp", () => {
      expect("amqp" satisfies string).toBe(fixtures.transports.amqp);
    });
    it("nats", () => {
      expect("nats" satisfies string).toBe(fixtures.transports.nats);
    });
  });

  describe("cloudEvents", () => {
    it("specVersion", () => {
      expect(CESpecVersion).toBe(fixtures.cloudEvents.specVersion);
    });
    it("specVersionValue", () => {
      expect(CESpecVersionValue).toBe(fixtures.cloudEvents.specVersionValue);
    });
    it("type", () => {
      expect(CEType).toBe(fixtures.cloudEvents.type);
    });
    it("source", () => {
      expect(CESource).toBe(fixtures.cloudEvents.source);
    });
    it("id", () => {
      expect(CEID).toBe(fixtures.cloudEvents.id);
    });
    it("time", () => {
      expect(CETime).toBe(fixtures.cloudEvents.time);
    });
    it("dataContentType", () => {
      expect(CEDataContentType).toBe(fixtures.cloudEvents.dataContentType);
    });
    it("subject", () => {
      expect(CESubject).toBe(fixtures.cloudEvents.subject);
    });
    it("dataSchema", () => {
      expect(CEDataSchema).toBe(fixtures.cloudEvents.dataSchema);
    });
    it("correlationId", () => {
      expect(CECorrelationID).toBe(fixtures.cloudEvents.correlationId);
    });
  });
});
