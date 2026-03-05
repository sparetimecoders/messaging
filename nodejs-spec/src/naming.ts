// MIT License
// Copyright (c) 2026 sparetimecoders

/** DefaultEventExchangeName is the default exchange name used for event streaming. */
export const DefaultEventExchangeName = "events";

/** Exchange kind constants matching AMQP exchange types. */
export const KindTopic = "topic";
export const KindDirect = "direct";
export const KindHeaders = "headers";

/**
 * TopicExchangeName returns the topic exchange name for the given name.
 * Format: `<name>.topic.exchange`
 */
export function topicExchangeName(name: string): string {
  return `${name}.${KindTopic}.exchange`;
}

/**
 * ServiceEventQueueName returns the durable event queue name for a service.
 * Format: `<exchangeName>.queue.<service>`
 */
export function serviceEventQueueName(
  exchangeName: string,
  service: string,
): string {
  return `${exchangeName}.queue.${service}`;
}

/**
 * ServiceRequestExchangeName returns the direct exchange name for requests to a service.
 * Format: `<service>.direct.exchange.request`
 */
export function serviceRequestExchangeName(service: string): string {
  return `${service}.${KindDirect}.exchange.request`;
}

/**
 * ServiceResponseExchangeName returns the headers exchange name for responses from a service.
 * Format: `<service>.headers.exchange.response`
 */
export function serviceResponseExchangeName(service: string): string {
  return `${service}.${KindHeaders}.exchange.response`;
}

/**
 * ServiceRequestQueueName returns the queue name for requests to a service.
 * Format: `<service>.direct.exchange.request.queue`
 */
export function serviceRequestQueueName(service: string): string {
  return `${serviceRequestExchangeName(service)}.queue`;
}

/**
 * ServiceResponseQueueName returns the queue name for responses from targetService to serviceName.
 * Format: `<targetService>.headers.exchange.response.queue.<serviceName>`
 */
export function serviceResponseQueueName(
  targetService: string,
  serviceName: string,
): string {
  return `${serviceResponseExchangeName(targetService)}.queue.${serviceName}`;
}

/**
 * NATSStreamName extracts the base stream name from a logical name.
 * If the name follows the AMQP convention `<name>.topic.exchange`, the prefix is extracted.
 * Otherwise the name is returned as-is.
 */
export function natsStreamName(name: string): string {
  const suffix = ".topic.exchange";
  if (name.endsWith(suffix)) {
    return name.slice(0, -suffix.length);
  }
  return name;
}

/**
 * NATSSubject builds a NATS subject from a stream name and routing key.
 * Format: `<stream>.<routingKey>`
 */
export function natsSubject(stream: string, routingKey: string): string {
  return `${stream}.${routingKey}`;
}

/**
 * TranslateWildcard converts AMQP-style wildcards to NATS-style wildcards.
 * AMQP "#" (multi-level) -> NATS ">" (multi-level)
 * AMQP "*" (single-level) stays "*" in NATS.
 */
export function translateWildcard(routingKey: string): string {
  return routingKey.replaceAll("#", ">");
}
