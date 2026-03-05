// MIT License
// Copyright (c) 2026 sparetimecoders

// Types
export type {
  Transport,
  EndpointDirection,
  ExchangeKind,
  Pattern,
  Headers,
  Metadata,
  DeliveryInfo,
  ConsumableEvent,
  Endpoint,
  Topology,
  EventHandler,
  RequestResponseEventHandler,
  NotificationSource,
  Notification,
  ErrorNotification,
  NotificationHandler,
  ErrorNotificationHandler,
} from "./types.js";
export { ErrParseJSON } from "./types.js";

// Naming
export {
  DefaultEventExchangeName,
  KindTopic,
  KindDirect,
  KindHeaders,
  topicExchangeName,
  serviceEventQueueName,
  serviceRequestExchangeName,
  serviceResponseExchangeName,
  serviceRequestQueueName,
  serviceResponseQueueName,
  natsStreamName,
  natsSubject,
  translateWildcard,
} from "./naming.js";

// CloudEvents
export {
  CESpecVersion,
  CEType,
  CESource,
  CEID,
  CETime,
  CESpecVersionValue,
  CEDataContentType,
  CESubject,
  CEDataSchema,
  CECorrelationID,
  CEAttrSpecVersion,
  CEAttrType,
  CEAttrSource,
  CEAttrID,
  CEAttrTime,
  CEAttrDataContentType,
  CEAttrSubject,
  CEAttrCorrelationID,
  CERequiredAttributes,
  AMQPCEHeaderKey,
  normalizeCEHeaders,
  hasCEHeaders,
  enrichLegacyMetadata,
  metadataFromHeaders,
  validateCEHeaders,
} from "./cloudevents.js";

// Routing
export { matchRoutingKey, routingKeyOverlaps } from "./routing.js";

// Validation
export { validate, validateTopologies } from "./validate.js";

// Visualization
export { mermaid } from "./visualize.js";

// Topology (re-exports)
export {} from "./topology.js";

// Metrics
export type { MetricsRecorder, RoutingKeyMapper, MetricsOptions } from "./metrics.js";
export { mapRoutingKey } from "./metrics.js";
