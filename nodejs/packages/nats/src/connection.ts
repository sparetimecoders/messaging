// MIT License
// Copyright (c) 2026 sparetimecoders

import type { TextMapPropagator } from "@opentelemetry/api";
import type {
  Topology,
  Endpoint,
  EventHandler,
  RequestResponseEventHandler,
} from "@gomessaging/spec";
import {
  DefaultEventExchangeName,
  natsStreamName,
} from "@gomessaging/spec";
import { connect, type NatsConnection, type JetStreamClient } from "nats";
import { Publisher } from "./publisher.js";
import type {
  JSConsumerRegistration,
  CoreConsumerRegistration,
  ConsumerHandle,
} from "./consumer.js";
import {
  startJSConsumers,
  startCoreConsumers,
} from "./consumer.js";

export interface ConnectionOptions {
  /** NATS connection URL (e.g., "nats://localhost:4222") */
  url: string;
  /** Service name for subscription/queue naming */
  serviceName: string;
  /** Optional logger (defaults to console) */
  logger?: Pick<Console, "info" | "warn" | "error" | "debug">;
  /** Optional OTel propagator for trace context propagation */
  propagator?: TextMapPropagator;
  /** Timeout in milliseconds for NATS Core request-reply operations. Default: 30000 (30s). */
  requestTimeout?: number;
}

/**
 * Connection manages a NATS connection and provides methods to
 * set up event streams (via JetStream), custom streams, and
 * request-response patterns (via Core NATS).
 */
export class Connection {
  private readonly url: string;
  private readonly serviceName: string;
  private readonly logger: Pick<Console, "info" | "warn" | "error" | "debug">;
  private readonly propagator?: TextMapPropagator;
  private readonly endpoints: Endpoint[] = [];
  private readonly requestTimeout: number;

  private nc: NatsConnection | null = null;
  private js: JetStreamClient | null = null;

  private readonly jsRegistrations: JSConsumerRegistration<unknown>[] = [];
  private readonly coreRegistrations: CoreConsumerRegistration<unknown, unknown>[] = [];
  private readonly publishers: Publisher[] = [];
  private readonly consumerHandles: ConsumerHandle[] = [];

  // Track streams that need publishers wired
  private readonly jsPublisherStreams: { stream: string; publisher: Publisher }[] = [];
  private readonly corePublisherTargets: { targetService: string; publisher: Publisher }[] = [];

  constructor(options: ConnectionOptions) {
    this.url = options.url;
    this.serviceName = options.serviceName;
    this.logger = options.logger ?? console;
    this.propagator = options.propagator;
    this.requestTimeout = options.requestTimeout ?? 30_000;
  }

  /**
   * Register an event stream publisher on the default "events" stream.
   * Returns a Publisher that can be used to publish after start().
   */
  addEventPublisher(): Publisher {
    const stream = natsStreamName(DefaultEventExchangeName);
    const publisher = new Publisher({
      serviceName: this.serviceName,
      stream,
      propagator: this.propagator,
    });
    this.publishers.push(publisher);
    this.jsPublisherStreams.push({ stream, publisher });
    this.endpoints.push({
      direction: "publish",
      pattern: "event-stream",
      exchangeName: stream,
      exchangeKind: "topic",
    });
    return publisher;
  }

  /**
   * Register an event stream consumer (JetStream durable consumer).
   */
  addEventConsumer<T>(
    routingKey: string,
    handler: EventHandler<T>,
    options?: { ephemeral?: boolean },
  ): void {
    const stream = natsStreamName(DefaultEventExchangeName);
    this.jsRegistrations.push({
      kind: "jetstream",
      stream,
      routingKey,
      handler: handler as EventHandler<unknown>,
      durable: options?.ephemeral ? undefined : this.serviceName,
    });
    this.endpoints.push({
      direction: "consume",
      pattern: "event-stream",
      exchangeName: stream,
      exchangeKind: "topic",
      queueName: options?.ephemeral ? undefined : this.serviceName,
      routingKey,
      ephemeral: options?.ephemeral,
    });
  }

  /**
   * Register a custom stream publisher.
   * Returns a Publisher that can be used to publish after start().
   */
  addCustomStreamPublisher(exchange: string): Publisher {
    const stream = natsStreamName(exchange);
    const publisher = new Publisher({
      serviceName: this.serviceName,
      stream,
      propagator: this.propagator,
    });
    this.publishers.push(publisher);
    this.jsPublisherStreams.push({ stream, publisher });
    this.endpoints.push({
      direction: "publish",
      pattern: "custom-stream",
      exchangeName: stream,
      exchangeKind: "topic",
    });
    return publisher;
  }

  /**
   * Register a custom stream consumer.
   */
  addCustomStreamConsumer<T>(
    exchange: string,
    routingKey: string,
    handler: EventHandler<T>,
  ): void {
    const stream = natsStreamName(exchange);
    this.jsRegistrations.push({
      kind: "jetstream",
      stream,
      routingKey,
      handler: handler as EventHandler<unknown>,
      durable: this.serviceName,
    });
    this.endpoints.push({
      direction: "consume",
      pattern: "custom-stream",
      exchangeName: stream,
      exchangeKind: "topic",
      queueName: this.serviceName,
      routingKey,
    });
  }

  /**
   * Register a service request consumer (Core NATS request-reply).
   */
  addServiceRequestConsumer<T, R>(
    routingKey: string,
    handler: RequestResponseEventHandler<T, R>,
  ): void {
    const subject = `${this.serviceName}.request.${routingKey}`;
    this.coreRegistrations.push({
      kind: "core",
      subject,
      routingKey,
      handler: handler as RequestResponseEventHandler<unknown, unknown>,
      requestReply: true,
    });
    this.endpoints.push({
      direction: "consume",
      pattern: "service-request",
      exchangeName: this.serviceName,
      exchangeKind: "direct",
      queueName: this.serviceName,
      routingKey,
    });
  }

  /**
   * Register a service request publisher.
   * Returns a Publisher that can be used to publish after start().
   */
  addServiceRequestPublisher(targetService: string): Publisher {
    const publisher = new Publisher({
      serviceName: this.serviceName,
      stream: targetService,
      propagator: this.propagator,
    });
    this.publishers.push(publisher);
    this.corePublisherTargets.push({ targetService, publisher });
    this.endpoints.push({
      direction: "publish",
      pattern: "service-request",
      exchangeName: targetService,
      exchangeKind: "direct",
    });
    return publisher;
  }

  /**
   * Register a service response consumer.
   * In NATS, request-reply responses are handled automatically by the
   * request() call. This method exists for topology registration.
   */
  addServiceResponseConsumer<T>(
    targetService: string,
    routingKey: string,
    _handler: EventHandler<T>,
  ): void {
    this.endpoints.push({
      direction: "consume",
      pattern: "service-response",
      exchangeName: targetService,
      exchangeKind: "headers",
      queueName: this.serviceName,
      routingKey,
    });
  }

  /**
   * Returns the declared topology for this service.
   */
  topology(): Topology {
    return {
      transport: "nats",
      serviceName: this.serviceName,
      endpoints: [...this.endpoints],
    };
  }

  /**
   * Start the NATS connection and set up streams/subscriptions.
   */
  async start(): Promise<void> {
    this.logger.info(
      `[gomessaging/nats] Starting connection to ${this.url} for service "${this.serviceName}"`,
    );

    // Connect to NATS
    this.nc = await connect({ servers: [this.url] });
    this.js = this.nc.jetstream();
    const jsm = await this.nc.jetstreamManager();

    this.logger.info(`[gomessaging/nats] Connected to ${this.url}`);

    // Ensure streams for publishers and wire them
    for (const { stream, publisher } of this.jsPublisherStreams) {
      try {
        await jsm.streams.add({
          name: stream,
          subjects: [`${stream}.>`],
        });
      } catch {
        // Stream may already exist
        try {
          await jsm.streams.update(stream, { subjects: [`${stream}.>`] });
        } catch {
          // Already exists with correct config
        }
      }
      publisher.wireJetStream(this.js);
    }

    // Wire Core request publishers
    for (const { publisher } of this.corePublisherTargets) {
      publisher.wireCoreRequest(this.nc, this.requestTimeout);
    }

    // Start JetStream consumers
    if (this.jsRegistrations.length > 0) {
      const handles = await startJSConsumers(
        this.js,
        jsm,
        this.serviceName,
        this.jsRegistrations,
        this.logger,
        this.propagator,
      );
      this.consumerHandles.push(...handles);
    }

    // Start Core consumers
    if (this.coreRegistrations.length > 0) {
      const handles = startCoreConsumers(
        this.nc,
        this.serviceName,
        this.coreRegistrations,
        this.logger,
        this.propagator,
      );
      this.consumerHandles.push(...handles);
    }

    this.logger.info(
      `[gomessaging/nats] Connection started for service "${this.serviceName}"`,
    );
  }

  /**
   * Gracefully close the connection.
   */
  async close(): Promise<void> {
    this.logger.info(
      `[gomessaging/nats] Closing connection for service "${this.serviceName}"`,
    );

    // Stop all consumer handles
    for (const handle of this.consumerHandles) {
      handle.stop();
    }

    // Drain and close
    if (this.nc) {
      await this.nc.drain();
      this.nc = null;
      this.js = null;
    }
  }
}
