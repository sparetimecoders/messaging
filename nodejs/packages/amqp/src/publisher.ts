// MIT License
// Copyright (c) 2026 sparetimecoders

import { v4 as uuidv4 } from "uuid";
import { context as otelContext, type TextMapPropagator } from "@opentelemetry/api";
import type * as amqplib from "amqplib";
import {
  CESpecVersion,
  CESpecVersionValue,
  CEType,
  CESource,
  CEDataContentType,
  CETime,
  CEID,
} from "@gomessaging/spec";
import { injectToHeaders } from "./tracing.js";

/**
 * Publisher sends JSON-encoded messages with CloudEvents headers via AMQP.
 * Created empty and wired during Connection.start().
 */
export class Publisher {
  private channel: amqplib.ConfirmChannel | null = null;
  private exchange = "";
  private serviceName = "";
  private propagator?: TextMapPropagator;

  /** Wire the publisher to a channel and exchange. Called during start(). */
  setup(
    channel: amqplib.ConfirmChannel,
    exchange: string,
    serviceName: string,
    propagator?: TextMapPropagator,
  ): void {
    this.channel = channel;
    this.exchange = exchange;
    this.serviceName = serviceName;
    this.propagator = propagator;
  }

  /**
   * Publish sends msg as a JSON-encoded AMQP message with the given routing key.
   * CloudEvents headers are set via the setDefault pattern (only if not already present).
   */
  async publish(
    routingKey: string,
    msg: unknown,
    headers?: Record<string, unknown>,
  ): Promise<void> {
    if (!this.channel) {
      throw new Error("publisher not initialized — call connection.start() first");
    }

    const jsonBytes = Buffer.from(JSON.stringify(msg));
    const h: Record<string, unknown> = { ...headers };

    // setDefault pattern — only set if not already present
    const setDefault = (key: string, value: string) => {
      if (!(key in h)) {
        h[key] = value;
      }
    };
    setDefault(CESpecVersion, CESpecVersionValue);
    setDefault(CEType, routingKey);
    setDefault(CESource, this.serviceName);
    setDefault(CEDataContentType, "application/json");
    setDefault(CETime, new Date().toISOString());

    // ce-id: use existing if present, otherwise generate
    const messageID =
      typeof h[CEID] === "string" && h[CEID] !== ""
        ? (h[CEID] as string)
        : uuidv4();
    h[CEID] = messageID;

    // Inject OTel trace context into headers
    injectToHeaders(otelContext.active(), h, this.propagator);

    this.channel.publish(this.exchange, routingKey, jsonBytes, {
      headers: h,
      contentType: "application/json",
      deliveryMode: 2,
    });
  }
}
