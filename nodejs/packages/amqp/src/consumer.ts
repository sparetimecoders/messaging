// MIT License
// Copyright (c) 2026 sparetimecoders

import type * as amqplib from "amqplib";
import type {
  ConsumableEvent,
  DeliveryInfo,
  EventHandler,
  Headers,
} from "@gomessaging/spec";
import {
  ErrParseJSON,
  metadataFromHeaders,
  validateCEHeaders,
  matchRoutingKey,
  routingKeyOverlaps,
} from "@gomessaging/spec";
import type { TextMapPropagator } from "@opentelemetry/api";
import { extractToContext } from "./tracing.js";

type Logger = Pick<Console, "info" | "warn" | "error" | "debug">;

/** RoutingKeyHandlers maps routing keys to typed event handlers. */
type RoutingKeyHandlers = Map<string, EventHandler<unknown>>;

/**
 * QueueConsumer manages a single AMQP queue with routing-key → handler dispatch.
 * Mirrors golang/amqp/consumer.go queueConsumer.
 */
export class QueueConsumer {
  readonly queue: string;
  readonly handlers: RoutingKeyHandlers = new Map();
  private logger: Logger;
  private propagator?: TextMapPropagator;
  private consumerTag = "";

  constructor(queue: string, logger: Logger, propagator?: TextMapPropagator) {
    this.queue = queue;
    this.logger = logger;
    this.propagator = propagator;
  }

  addHandler(routingKey: string, handler: EventHandler<unknown>): void {
    for (const existingKey of this.handlers.keys()) {
      if (routingKeyOverlaps(routingKey, existingKey)) {
        throw new Error(
          `routing key "${routingKey}" overlaps "${existingKey}" for queue "${this.queue}"`,
        );
      }
    }
    this.handlers.set(routingKey, handler);
  }

  /** Start consuming from the channel. Returns the consumer tag for cancellation. */
  async consume(channel: amqplib.Channel): Promise<string> {
    const { consumerTag } = await channel.consume(
      this.queue,
      (msg) => {
        if (!msg) {
          this.logger.warn(
            `[gomessaging/amqp] consumer received null message (channel closed) for queue "${this.queue}"`,
          );
          return;
        }
        this.handleMessage(channel, msg);
      },
      { noAck: false },
    );
    this.consumerTag = consumerTag;
    return consumerTag;
  }

  getConsumerTag(): string {
    return this.consumerTag;
  }

  private handleMessage(channel: amqplib.Channel, msg: amqplib.ConsumeMessage): void {
    const deliveryInfo = getDeliveryInfo(this.queue, msg);

    let handler: EventHandler<unknown> | undefined;
    for (const [pattern, h] of this.handlers) {
      if (matchRoutingKey(pattern, deliveryInfo.key)) {
        handler = h;
        break;
      }
    }
    if (!handler) {
      this.logger.warn(
        `[gomessaging/amqp] no handler for routing key "${deliveryInfo.key}" on queue "${this.queue}", rejecting`,
      );
      channel.nack(msg, false, false);
      return;
    }

    // Validate CE headers
    const warnings = validateCEHeaders(deliveryInfo.headers);
    if (warnings.length > 0) {
      this.logger.warn(
        `[gomessaging/amqp] invalid CloudEvents headers on "${deliveryInfo.key}": ${warnings.join(", ")}`,
      );
    }

    // Extract OTel context from headers
    extractToContext(deliveryInfo.headers, this.propagator);

    // Parse JSON body
    let payload: unknown;
    try {
      payload = JSON.parse(msg.content.toString("utf-8"));
    } catch {
      this.logger.warn(
        `[gomessaging/amqp] ${ErrParseJSON}: nacking without requeue for "${deliveryInfo.key}"`,
      );
      channel.nack(msg, false, false);
      return;
    }

    // Build ConsumableEvent
    const metadata = metadataFromHeaders(deliveryInfo.headers);
    const event: ConsumableEvent<unknown> = {
      ...metadata,
      deliveryInfo,
      payload,
    };

    handler(event)
      .then(() => {
        channel.ack(msg);
      })
      .catch((err: Error) => {
        this.logger.error(
          `[gomessaging/amqp] handler error for "${deliveryInfo.key}": ${err.message}`,
        );
        if (err.message.includes(ErrParseJSON)) {
          channel.nack(msg, false, false);
        } else {
          channel.nack(msg, false, true);
        }
      });
  }
}

function getDeliveryInfo(
  queueName: string,
  msg: amqplib.ConsumeMessage,
): DeliveryInfo {
  const headers: Headers = {};
  if (msg.properties.headers) {
    for (const [k, v] of Object.entries(msg.properties.headers)) {
      headers[k] = v;
    }
  }
  return {
    destination: queueName,
    source: msg.fields.exchange,
    key: msg.fields.routingKey,
    headers,
  };
}
