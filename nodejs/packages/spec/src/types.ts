// MIT License
// Copyright (c) 2026 sparetimecoders

/** Transport identifies the messaging transport implementation. */
export type Transport = "amqp" | "nats";

/** EndpointDirection indicates whether an endpoint publishes or consumes messages. */
export type EndpointDirection = "publish" | "consume";

/** ExchangeKind represents the type of an exchange. */
export type ExchangeKind = "topic" | "direct" | "headers";

/** Pattern identifies the communication pattern an endpoint participates in. */
export type Pattern =
  | "event-stream"
  | "custom-stream"
  | "service-request"
  | "service-response"
  | "queue-publish";

/** Headers represent all meta-data for the message. */
export type Headers = Record<string, unknown>;

/** Metadata holds the metadata of an event. */
export interface Metadata {
  id: string;
  correlationId: string;
  timestamp: string;
  source: string;
  type: string;
  subject: string;
  dataContentType: string;
  specVersion: string;
}

/** DeliveryInfo holds transport-agnostic delivery metadata. */
export interface DeliveryInfo {
  destination: string;
  source: string;
  key: string;
  headers: Headers;
}

/** ConsumableEvent represents an event that can be consumed. */
export interface ConsumableEvent<T> extends Metadata {
  deliveryInfo: DeliveryInfo;
  payload: T;
}

/** Endpoint describes a single exchange/queue/binding that a service declares. */
export interface Endpoint {
  direction: EndpointDirection;
  pattern: Pattern;
  exchangeName: string;
  exchangeKind: ExchangeKind;
  queueName?: string;
  routingKey?: string;
  messageType?: string;
  ephemeral?: boolean;
}

/** Topology describes the full messaging topology declared by a single service. */
export interface Topology {
  transport?: Transport;
  serviceName: string;
  endpoints: Endpoint[];
}

/** EventHandler is a function that handles events of a specific type. */
export type EventHandler<T> = (
  event: ConsumableEvent<T>,
) => Promise<void>;

/** RequestResponseEventHandler handles events and returns a response. */
export type RequestResponseEventHandler<T, R> = (
  event: ConsumableEvent<T>,
) => Promise<R>;

/** ErrParseJSON sentinel error message. */
export const ErrParseJSON = "failed to parse";
