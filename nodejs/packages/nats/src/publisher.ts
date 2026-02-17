// MIT License
// Copyright (c) 2026 sparetimecoders

import { randomUUID } from "node:crypto";
import type { Context } from "@opentelemetry/api";
import type { JetStreamClient, NatsConnection, MsgHdrs } from "nats";
import { headers as natsHeaders } from "nats";
import {
  CESpecVersion,
  CESpecVersionValue,
  CEType,
  CESource,
  CEDataContentType,
  CETime,
  CEID,
  natsSubject,
} from "@gomessaging/spec";
import { injectToHeaders } from "./tracing.js";
import type { TextMapPropagator } from "@opentelemetry/api";

const contentType = "application/json";

type PublishFn = (subject: string, data: Uint8Array, headers: MsgHdrs) => Promise<void>;

export interface PublisherOptions {
  serviceName: string;
  stream: string;
  propagator?: TextMapPropagator;
}

/**
 * Publisher sends messages to NATS via JetStream or Core request-reply.
 */
export class Publisher {
  private readonly serviceName: string;
  private readonly stream: string;
  private readonly propagator?: TextMapPropagator;
  private publishFn: PublishFn | null = null;

  constructor(options: PublisherOptions) {
    this.serviceName = options.serviceName;
    this.stream = options.stream;
    this.propagator = options.propagator;
  }

  /** Wire this publisher to use JetStream publish. */
  wireJetStream(js: JetStreamClient): void {
    this.publishFn = async (subject, data, hdrs) => {
      await js.publish(subject, data, { headers: hdrs });
    };
  }

  /** Wire this publisher to use Core NATS request-reply. */
  wireCoreRequest(nc: NatsConnection, timeout: number = 30_000): void {
    this.publishFn = async (subject, data, hdrs) => {
      await nc.request(subject, data, { timeout, headers: hdrs });
    };
  }

  /**
   * Publish a message with the given routing key.
   * CloudEvents headers are set using the setDefault pattern.
   */
  async publish(routingKey: string, msg: unknown, ctx?: Context): Promise<void> {
    if (!this.publishFn) {
      throw new Error("Publisher not wired — call wireJetStream or wireCoreRequest first");
    }

    const subject = natsSubject(this.stream, routingKey);
    const data = new TextEncoder().encode(JSON.stringify(msg));

    const hdrs = natsHeaders();

    // Set service header
    hdrs.set("service", this.serviceName);

    // setDefault pattern: only set if not already present
    const setDefault = (key: string, value: string) => {
      if (!hdrs.has(key)) {
        hdrs.set(key, value);
      }
    };

    setDefault(CESpecVersion, CESpecVersionValue);
    setDefault(CEType, routingKey);
    setDefault(CESource, this.serviceName);
    setDefault(CEDataContentType, contentType);
    setDefault(CETime, new Date().toISOString());

    const messageID = hdrs.has(CEID) ? hdrs.get(CEID) : randomUUID();
    hdrs.set(CEID, messageID);

    // Inject trace context if available
    if (ctx) {
      injectToHeaders(ctx, hdrs, this.propagator);
    }

    await this.publishFn(subject, data, hdrs);
  }
}
