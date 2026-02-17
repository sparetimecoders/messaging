// MIT License
// Copyright (c) 2026 sparetimecoders

import * as amqplib from "amqplib";
import type { TextMapPropagator } from "@opentelemetry/api";
import type {
  Topology,
  Endpoint,
  EventHandler,
  RequestResponseEventHandler,
  ConsumableEvent,
} from "@gomessaging/spec";
import {
  DefaultEventExchangeName,
  topicExchangeName,
  serviceEventQueueName,
  serviceRequestExchangeName,
  serviceResponseExchangeName,
  serviceRequestQueueName,
  serviceResponseQueueName,
} from "@gomessaging/spec";
import { Publisher } from "./publisher.js";
import { QueueConsumer } from "./consumer.js";

type Logger = Pick<Console, "info" | "warn" | "error" | "debug">;

/** Queue header constants matching Go defaults. */
const deleteQueueAfterMs = 5 * 24 * 60 * 60 * 1000; // 5 days
const ephemeralQueueTTLMs = 1000;

export interface ConnectionOptions {
  /** AMQP connection URL (e.g., "amqp://localhost") */
  url: string;
  /** Service name for queue naming */
  serviceName: string;
  /** Optional logger (defaults to console) */
  logger?: Logger;
  /** Optional OTel text map propagator */
  propagator?: TextMapPropagator;
  /** Optional callback invoked when the AMQP connection is unexpectedly closed.
   *  Use this to implement fail-fast behavior (e.g., process.exit(1)). */
  onClose?: (err: Error) => void;
}

interface PublisherRegistration {
  kind: "publisher";
  exchangeName: string;
  exchangeKind: string;
  publisher: Publisher;
}

interface ConsumerRegistration {
  kind: "consumer";
  exchangeName: string;
  exchangeKind: string;
  queueName: string;
  routingKey: string;
  handler: EventHandler<unknown>;
  queueHeaders: Record<string, unknown>;
  bindingHeaders?: Record<string, unknown>;
}

interface RequestConsumerRegistration {
  kind: "request-consumer";
  exchangeName: string;
  queueName: string;
  routingKey: string;
  handler: RequestResponseEventHandler<unknown, unknown>;
  responseExchangeName: string;
}

type Registration =
  | PublisherRegistration
  | ConsumerRegistration
  | RequestConsumerRegistration;

/**
 * Connection manages an AMQP connection and provides methods to
 * set up event streams, custom streams, and request-response patterns.
 */
export class Connection {
  private readonly url: string;
  private readonly serviceName: string;
  private readonly logger: Logger;
  private readonly propagator?: TextMapPropagator;
  private readonly endpoints: Endpoint[] = [];
  private readonly registrations: Registration[] = [];

  private readonly onClose?: (err: Error) => void;

  private amqpConn: amqplib.ChannelModel | null = null;
  private publishChannel: amqplib.ConfirmChannel | null = null;
  private consumerChannels: amqplib.Channel[] = [];
  private consumerTags: string[] = [];
  private closing = false;
  private lastError: Error | null = null;

  constructor(options: ConnectionOptions) {
    this.url = options.url;
    this.serviceName = options.serviceName;
    this.logger = options.logger ?? console;
    this.propagator = options.propagator;
    this.onClose = options.onClose;
  }

  /**
   * Register an event stream publisher on the default "events" exchange.
   */
  addEventPublisher(publisher?: Publisher): Publisher {
    const exchangeName = topicExchangeName(DefaultEventExchangeName);
    const pub = publisher ?? new Publisher();
    this.endpoints.push({
      direction: "publish",
      pattern: "event-stream",
      exchangeName,
      exchangeKind: "topic",
    });
    this.registrations.push({
      kind: "publisher",
      exchangeName,
      exchangeKind: "topic",
      publisher: pub,
    });
    return pub;
  }

  /**
   * Register an event stream consumer on the default "events" exchange.
   */
  addEventConsumer<T>(
    routingKey: string,
    handler: EventHandler<T>,
    options?: { ephemeral?: boolean },
  ): void {
    const exchangeName = topicExchangeName(DefaultEventExchangeName);
    const ephemeral = options?.ephemeral ?? false;
    const queueName = ephemeral
      ? `${serviceEventQueueName(exchangeName, this.serviceName)}.${randomSuffix()}`
      : serviceEventQueueName(exchangeName, this.serviceName);
    this.endpoints.push({
      direction: "consume",
      pattern: "event-stream",
      exchangeName,
      exchangeKind: "topic",
      queueName: ephemeral ? undefined : serviceEventQueueName(exchangeName, this.serviceName),
      routingKey,
      ephemeral: options?.ephemeral,
    });
    this.registrations.push({
      kind: "consumer",
      exchangeName,
      exchangeKind: "topic",
      queueName,
      routingKey,
      handler: handler as EventHandler<unknown>,
      queueHeaders: ephemeral
        ? { "x-queue-type": "quorum", "x-expires": ephemeralQueueTTLMs }
        : defaultQueueHeaders(),
    });
  }

  /**
   * Register a custom stream publisher.
   */
  addCustomStreamPublisher(exchange: string, publisher?: Publisher): Publisher {
    const exchangeName = topicExchangeName(exchange);
    const pub = publisher ?? new Publisher();
    this.endpoints.push({
      direction: "publish",
      pattern: "custom-stream",
      exchangeName,
      exchangeKind: "topic",
    });
    this.registrations.push({
      kind: "publisher",
      exchangeName,
      exchangeKind: "topic",
      publisher: pub,
    });
    return pub;
  }

  /**
   * Register a custom stream consumer.
   */
  addCustomStreamConsumer<T>(
    exchange: string,
    routingKey: string,
    handler: EventHandler<T>,
  ): void {
    const exchangeName = topicExchangeName(exchange);
    this.endpoints.push({
      direction: "consume",
      pattern: "custom-stream",
      exchangeName,
      exchangeKind: "topic",
      queueName: serviceEventQueueName(exchangeName, this.serviceName),
      routingKey,
    });
    this.registrations.push({
      kind: "consumer",
      exchangeName,
      exchangeKind: "topic",
      queueName: serviceEventQueueName(exchangeName, this.serviceName),
      routingKey,
      handler: handler as EventHandler<unknown>,
      queueHeaders: defaultQueueHeaders(),
    });
  }

  /**
   * Register a service request consumer (this service handles requests).
   */
  addServiceRequestConsumer<T, R>(
    routingKey: string,
    handler: (event: ConsumableEvent<T>) => Promise<R>,
  ): void {
    this.endpoints.push({
      direction: "consume",
      pattern: "service-request",
      exchangeName: serviceRequestExchangeName(this.serviceName),
      exchangeKind: "direct",
      queueName: serviceRequestQueueName(this.serviceName),
      routingKey,
    });
    this.registrations.push({
      kind: "request-consumer",
      exchangeName: serviceRequestExchangeName(this.serviceName),
      queueName: serviceRequestQueueName(this.serviceName),
      routingKey,
      handler: handler as RequestResponseEventHandler<unknown, unknown>,
      responseExchangeName: serviceResponseExchangeName(this.serviceName),
    });
    // Response exchange is declared at broker level only, not tracked as topology endpoint.
  }

  /**
   * Register a service request publisher (this service sends requests).
   */
  addServiceRequestPublisher(
    targetService: string,
    publisher?: Publisher,
  ): Publisher {
    const exchangeName = serviceRequestExchangeName(targetService);
    const pub = publisher ?? new Publisher();
    this.endpoints.push({
      direction: "publish",
      pattern: "service-request",
      exchangeName,
      exchangeKind: "direct",
    });
    this.registrations.push({
      kind: "publisher",
      exchangeName,
      exchangeKind: "direct",
      publisher: pub,
    });
    return pub;
  }

  /**
   * Register a service response consumer.
   */
  addServiceResponseConsumer<T>(
    targetService: string,
    routingKey: string,
    handler: EventHandler<T>,
  ): void {
    this.endpoints.push({
      direction: "consume",
      pattern: "service-response",
      exchangeName: serviceResponseExchangeName(targetService),
      exchangeKind: "headers",
      queueName: serviceResponseQueueName(targetService, this.serviceName),
      routingKey,
    });
    this.registrations.push({
      kind: "consumer",
      exchangeName: serviceResponseExchangeName(targetService),
      exchangeKind: "headers",
      queueName: serviceResponseQueueName(targetService, this.serviceName),
      routingKey,
      handler: handler as EventHandler<unknown>,
      queueHeaders: defaultQueueHeaders(),
      bindingHeaders: { service: this.serviceName },
    });
  }

  /**
   * Returns the declared topology for this service.
   */
  topology(): Topology {
    return {
      transport: "amqp",
      serviceName: this.serviceName,
      endpoints: [...this.endpoints],
    };
  }

  /**
   * Start the AMQP connection, declare topology, and start consumers.
   * Mirrors Go Connection.Start().
   */
  async start(): Promise<void> {
    this.logger.info(
      `[gomessaging/amqp] Starting connection to ${this.url} for service "${this.serviceName}"`,
    );

    // 1. Connect
    this.amqpConn = await amqplib.connect(this.url);
    const conn = this.amqpConn;

    // Register connection-level close/error handlers
    conn.on("error", (err: Error) => {
      this.logger.error(`[gomessaging/amqp] connection error: ${err.message}`);
      this.lastError = err;
    });
    conn.on("close", () => {
      if (this.closing) return;
      const err = this.lastError ?? new Error("AMQP connection closed");
      this.logger.error(
        `[gomessaging/amqp] connection closed unexpectedly: ${err.message}`,
      );
      if (this.onClose) {
        this.onClose(err);
      }
    });

    // 2. Create publish confirm channel
    this.publishChannel = await conn.createConfirmChannel();

    // 3. Create setup channel for declaring topology
    const setupChannel = await conn.createChannel();

    // Group consumer registrations by queue name for multi-routing-key support
    const queueConsumers = new Map<string, QueueConsumer>();

    for (const reg of this.registrations) {
      if (reg.kind === "publisher") {
        // Declare exchange and wire publisher
        await setupChannel.assertExchange(reg.exchangeName, reg.exchangeKind, {
          durable: true,
        });
        reg.publisher.setup(
          this.publishChannel!,
          reg.exchangeName,
          this.serviceName,
          this.propagator,
        );
        this.logger.info(
          `[gomessaging/amqp] configured publisher exchange="${reg.exchangeName}"`,
        );
      } else if (reg.kind === "consumer") {
        // Declare exchange
        await setupChannel.assertExchange(reg.exchangeName, reg.exchangeKind, {
          durable: true,
        });
        // Declare queue
        await setupChannel.assertQueue(reg.queueName, {
          durable: true,
          arguments: reg.queueHeaders,
        });
        // Bind queue
        await setupChannel.bindQueue(
          reg.queueName,
          reg.exchangeName,
          reg.routingKey,
          reg.bindingHeaders,
        );
        this.logger.info(
          `[gomessaging/amqp] bound queue="${reg.queueName}" exchange="${reg.exchangeName}" routingKey="${reg.routingKey}"`,
        );

        // Group handlers by queue
        let qc = queueConsumers.get(reg.queueName);
        if (!qc) {
          qc = new QueueConsumer(reg.queueName, this.logger, this.propagator);
          queueConsumers.set(reg.queueName, qc);
        }
        qc.addHandler(reg.routingKey, reg.handler);
      } else if (reg.kind === "request-consumer") {
        // Declare request exchange (direct)
        await setupChannel.assertExchange(reg.exchangeName, "direct", {
          durable: true,
        });
        // Declare response exchange (headers)
        await setupChannel.assertExchange(reg.responseExchangeName, "headers", {
          durable: true,
        });
        // Declare request queue
        await setupChannel.assertQueue(reg.queueName, {
          durable: true,
          arguments: defaultQueueHeaders(),
        });
        // Bind queue
        await setupChannel.bindQueue(
          reg.queueName,
          reg.exchangeName,
          reg.routingKey,
        );
        this.logger.info(
          `[gomessaging/amqp] bound request queue="${reg.queueName}" exchange="${reg.exchangeName}" routingKey="${reg.routingKey}"`,
        );

        // Register as a regular consumer handler
        let qc = queueConsumers.get(reg.queueName);
        if (!qc) {
          qc = new QueueConsumer(reg.queueName, this.logger, this.propagator);
          queueConsumers.set(reg.queueName, qc);
        }
        qc.addHandler(
          reg.routingKey,
          reg.handler as unknown as EventHandler<unknown>,
        );
      }
    }

    // Close setup channel
    await setupChannel.close();

    // Start consumers — each queue gets its own channel
    for (const qc of queueConsumers.values()) {
      const ch = await conn.createChannel();
      await ch.prefetch(20);
      this.consumerChannels.push(ch);
      const tag = await qc.consume(ch);
      this.consumerTags.push(tag);
      this.logger.info(
        `[gomessaging/amqp] started consumer queue="${qc.queue}" tag="${tag}"`,
      );
    }

    this.logger.info(
      `[gomessaging/amqp] connection started, consumers=${queueConsumers.size}`,
    );
  }

  /**
   * Gracefully close the connection.
   */
  async close(): Promise<void> {
    this.closing = true;
    this.logger.info(
      `[gomessaging/amqp] Closing connection for service "${this.serviceName}"`,
    );

    // Cancel consumers
    for (let i = 0; i < this.consumerChannels.length; i++) {
      try {
        if (this.consumerTags[i]) {
          await this.consumerChannels[i].cancel(this.consumerTags[i]);
        }
      } catch {
        // channel may already be closed
      }
    }

    // Close consumer channels
    for (const ch of this.consumerChannels) {
      try {
        await ch.close();
      } catch {
        // ignore
      }
    }
    this.consumerChannels = [];
    this.consumerTags = [];

    // Close publish channel
    if (this.publishChannel) {
      try {
        await this.publishChannel.close();
      } catch {
        // ignore
      }
      this.publishChannel = null;
    }

    // Close connection
    if (this.amqpConn) {
      await this.amqpConn.close();
      this.amqpConn = null;
    }
  }
}

function defaultQueueHeaders(): Record<string, unknown> {
  return {
    "x-queue-type": "quorum",
    "x-single-active-consumer": true,
    "x-expires": deleteQueueAfterMs,
  };
}

function randomSuffix(): string {
  return Math.random().toString(36).slice(2, 10);
}
