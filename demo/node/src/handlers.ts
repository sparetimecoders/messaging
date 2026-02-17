import { Connection as AmqpConnection } from "@gomessaging/amqp";
import { Publisher as AmqpPublisher } from "@gomessaging/amqp";
import { Connection as NatsConnection } from "@gomessaging/nats";
import { Publisher as NatsPublisher } from "@gomessaging/nats";
import type { ConsumableEvent } from "@gomessaging/spec";
import type {
  OrderCreated,
  PingMessage,
  EmailRequest,
  EmailResponse,
} from "./messages.js";
import type { SSEEvent } from "./web.js";
import { randomUUID } from "node:crypto";

export interface AmqpSetup {
  conn: AmqpConnection;
  publisher: AmqpPublisher;
}

export interface NatsSetup {
  conn: NatsConnection;
  publisher: NatsPublisher;
}

export function setupAmqp(
  amqpUrl: string,
  broadcast: (event: SSEEvent) => void,
): AmqpSetup {
  const conn = new AmqpConnection({
    url: amqpUrl,
    serviceName: "node-demo",
    onClose: (err) => {
      console.error("AMQP connection lost, shutting down:", err.message);
      process.exit(1);
    },
  });

  const publisher = conn.addEventPublisher();

  conn.addEventConsumer<OrderCreated>("Order.Created", async (event) => {
    console.log(
      `[amqp] received Order.Created from ${event.payload.source}: ${event.payload.orderId}`,
    );
    broadcast({
      type: "received",
      transport: "amqp",
      source: event.payload.source,
      routingKey: event.deliveryInfo.key,
      payload: event.payload,
      traceId: event.id,
    });
  });

  conn.addEventConsumer<PingMessage>("Ping", async (event) => {
    console.log(
      `[amqp] received Ping from ${event.payload.source}: ${event.payload.message}`,
    );
    broadcast({
      type: "received",
      transport: "amqp",
      source: event.payload.source,
      routingKey: event.deliveryInfo.key,
      payload: event.payload,
      traceId: event.id,
    });
  });

  conn.addServiceRequestConsumer<EmailRequest, EmailResponse>(
    "email.send",
    async (event: ConsumableEvent<EmailRequest>) => {
      console.log(
        `[amqp] handling email request to ${event.payload.to}`,
      );
      broadcast({
        type: "received",
        transport: "amqp",
        source: event.source,
        routingKey: event.deliveryInfo.key,
        payload: event.payload,
        traceId: event.id,
      });
      return {
        messageId: `msg-${randomUUID().substring(0, 8)}`,
        status: "sent",
      };
    },
  );

  return { conn, publisher };
}

export function setupNats(
  natsUrl: string,
  broadcast: (event: SSEEvent) => void,
): NatsSetup {
  const conn = new NatsConnection({ url: natsUrl, serviceName: "node-demo" });

  const publisher = conn.addEventPublisher();

  conn.addEventConsumer<OrderCreated>("Order.Created", async (event) => {
    console.log(
      `[nats] received Order.Created from ${event.payload.source}: ${event.payload.orderId}`,
    );
    broadcast({
      type: "received",
      transport: "nats",
      source: event.payload.source,
      routingKey: event.deliveryInfo.key,
      payload: event.payload,
      traceId: event.id,
    });
  });

  conn.addEventConsumer<PingMessage>("Ping", async (event) => {
    console.log(
      `[nats] received Ping from ${event.payload.source}: ${event.payload.message}`,
    );
    broadcast({
      type: "received",
      transport: "nats",
      source: event.payload.source,
      routingKey: event.deliveryInfo.key,
      payload: event.payload,
      traceId: event.id,
    });
  });

  conn.addServiceRequestConsumer<EmailRequest, EmailResponse>(
    "email.send",
    async (event: ConsumableEvent<EmailRequest>) => {
      console.log(
        `[nats] handling email request to ${event.payload.to}`,
      );
      broadcast({
        type: "received",
        transport: "nats",
        source: event.source,
        routingKey: event.deliveryInfo.key,
        payload: event.payload,
        traceId: event.id,
      });
      return {
        messageId: `msg-${randomUUID().substring(0, 8)}`,
        status: "sent",
      };
    },
  );

  return { conn, publisher };
}
