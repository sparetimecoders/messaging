import { describe, expect, it, vi, beforeEach } from "vitest";
import { Publisher } from "../src/publisher.js";
import {
  CESpecVersion,
  CESpecVersionValue,
  CEType,
  CESource,
  CEDataContentType,
  CETime,
  CEID,
} from "@gomessaging/spec";

// Mock amqplib ConfirmChannel
function createMockChannel() {
  return {
    publish: vi.fn().mockReturnValue(true),
  } as unknown as import("amqplib").ConfirmChannel;
}

describe("Publisher", () => {
  let publisher: Publisher;
  let channel: ReturnType<typeof createMockChannel>;

  beforeEach(() => {
    publisher = new Publisher();
    channel = createMockChannel();
    publisher.setup(
      channel as import("amqplib").ConfirmChannel,
      "events.topic.exchange",
      "test-service",
    );
  });

  it("throws if not initialized", async () => {
    const uninit = new Publisher();
    await expect(uninit.publish("key", {})).rejects.toThrow(
      "publisher not initialized",
    );
  });

  it("publishes JSON-encoded body", async () => {
    const msg = { orderId: "123", amount: 42 };
    await publisher.publish("order.created", msg);

    expect(channel.publish).toHaveBeenCalledOnce();
    const [exchange, routingKey, body, options] = (
      channel.publish as ReturnType<typeof vi.fn>
    ).mock.calls[0];
    expect(exchange).toBe("events.topic.exchange");
    expect(routingKey).toBe("order.created");
    expect(JSON.parse(body.toString())).toEqual(msg);
    expect(options.contentType).toBe("application/json");
    expect(options.deliveryMode).toBe(2);
  });

  it("sets CloudEvents headers via setDefault pattern", async () => {
    await publisher.publish("order.created", { data: true });

    const [, , , options] = (channel.publish as ReturnType<typeof vi.fn>).mock
      .calls[0];
    const headers = options.headers;

    expect(headers[CESpecVersion]).toBe(CESpecVersionValue);
    expect(headers[CEType]).toBe("order.created");
    expect(headers[CESource]).toBe("test-service");
    expect(headers[CEDataContentType]).toBe("application/json");
    expect(headers[CETime]).toBeDefined();
    expect(headers[CEID]).toBeDefined();
    // Verify ce-time is RFC3339-ish
    expect(new Date(headers[CETime] as string).toISOString()).toBe(
      headers[CETime],
    );
    // Verify ce-id is a UUID
    expect(headers[CEID]).toMatch(
      /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i,
    );
  });

  it("does not overwrite pre-existing CE headers", async () => {
    const existingHeaders = {
      [CEType]: "custom.type",
      [CESource]: "custom-source",
      [CEID]: "my-custom-id",
    };
    await publisher.publish("order.created", {}, existingHeaders);

    const [, , , options] = (channel.publish as ReturnType<typeof vi.fn>).mock
      .calls[0];
    const headers = options.headers;

    expect(headers[CEType]).toBe("custom.type");
    expect(headers[CESource]).toBe("custom-source");
    expect(headers[CEID]).toBe("my-custom-id");
    // But specversion is still set via default
    expect(headers[CESpecVersion]).toBe(CESpecVersionValue);
  });

  it("generates a new ce-id for each publish call", async () => {
    await publisher.publish("key1", {});
    await publisher.publish("key2", {});

    const id1 = (channel.publish as ReturnType<typeof vi.fn>).mock.calls[0][3]
      .headers[CEID];
    const id2 = (channel.publish as ReturnType<typeof vi.fn>).mock.calls[1][3]
      .headers[CEID];
    expect(id1).not.toBe(id2);
  });
});
