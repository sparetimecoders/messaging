// MIT License
//
// Copyright (c) 2026 sparetimecoders
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package tck

import (
	"github.com/sparetimecoders/messaging"
	"github.com/sparetimecoders/messaging/spectest"
)

// ComputeExpectedEndpoints computes expected topology endpoints from intents
// using spec naming functions applied to runtime (randomized) names.
// The result is keyed by template service name.
func ComputeExpectedEndpoints(transportKey string, services map[string]spectest.ServiceConfig, mapper *NameMapper) map[string][]spectest.ExpectedEndpoint {
	result := make(map[string][]spectest.ExpectedEndpoint)
	for templateName, svcCfg := range services {
		runtimeName := mapper.Runtime(templateName)
		var endpoints []spectest.ExpectedEndpoint
		for _, intent := range svcCfg.Setups {
			eps := computeEndpoints(transportKey, runtimeName, intent, mapper)
			endpoints = append(endpoints, eps...)
		}
		result[templateName] = endpoints
	}
	return result
}

// ComputeExpectedBrokerState computes expected broker-level state from service
// intents using spec naming functions applied to runtime names.
func ComputeExpectedBrokerState(transportKey string, services map[string]spectest.ServiceConfig, mapper *NameMapper) spectest.BrokerState {
	switch transportKey {
	case "amqp":
		return spectest.BrokerState{AMQP: computeAMQPBrokerState(services, mapper)}
	case "nats":
		return spectest.BrokerState{NATS: computeNATSBrokerState(services, mapper)}
	}
	return spectest.BrokerState{}
}

// ComputeProbeTarget derives a probe's raw broker target from the scenario context
// and randomized names. This replaces the static rawTarget field in tck.json.
func ComputeProbeTarget(transportKey string, probe ProbeMessage, services map[string]spectest.ServiceConfig, mapper *NameMapper) spectest.ProbeTarget {
	baseExchange := resolveProbeExchange(probe, services)

	switch transportKey {
	case "amqp":
		return spectest.ProbeTarget{
			Exchange:   messaging.TopicExchangeName(baseExchange),
			RoutingKey: probe.RoutingKey,
		}
	case "nats":
		return spectest.ProbeTarget{
			Stream:  baseExchange,
			Subject: messaging.NATSSubject(baseExchange, probe.RoutingKey),
		}
	}
	return spectest.ProbeTarget{}
}

// resolveProbeExchange finds the base exchange/stream name for a probe from
// the relevant service's intents.
func resolveProbeExchange(probe ProbeMessage, services map[string]spectest.ServiceConfig) string {
	var svcName string
	var direction string
	if probe.Direction == "outbound" {
		svcName = probe.PublishVia
		direction = "publish"
	} else {
		svcName = probe.ExpectReceivedBy
		direction = "consume"
	}

	svc, ok := services[svcName]
	if !ok {
		return ""
	}
	for _, intent := range svc.Setups {
		if intent.Direction == direction {
			return intentBaseExchange(intent)
		}
	}
	return ""
}

func intentBaseExchange(intent spectest.SetupIntent) string {
	switch intent.Pattern {
	case "event-stream":
		return messaging.DefaultEventExchangeName
	case "custom-stream":
		return intent.Exchange
	case "service-request":
		return intent.TargetService
	default:
		return ""
	}
}

// --- Endpoint computation ---

func computeEndpoints(transportKey, runtimeService string, intent spectest.SetupIntent, mapper *NameMapper) []spectest.ExpectedEndpoint {
	switch transportKey {
	case "amqp":
		return computeAMQPEndpoints(runtimeService, intent, mapper)
	case "nats":
		return computeNATSEndpoints(runtimeService, intent, mapper)
	}
	return nil
}

func computeAMQPEndpoints(runtimeService string, intent spectest.SetupIntent, mapper *NameMapper) []spectest.ExpectedEndpoint {
	switch {
	case intent.Pattern == "event-stream" && intent.Direction == "publish":
		return []spectest.ExpectedEndpoint{{
			Direction:    "publish",
			Pattern:      "event-stream",
			ExchangeName: messaging.TopicExchangeName(messaging.DefaultEventExchangeName),
			ExchangeKind: messaging.KindTopic,
		}}

	case intent.Pattern == "event-stream" && intent.Direction == "consume" && !intent.Ephemeral:
		exName := messaging.TopicExchangeName(messaging.DefaultEventExchangeName)
		qName := messaging.ServiceEventQueueName(exName, runtimeService)
		if intent.QueueSuffix != "" {
			qName += "-" + intent.QueueSuffix
		}
		return []spectest.ExpectedEndpoint{{
			Direction:    "consume",
			Pattern:      "event-stream",
			ExchangeName: exName,
			ExchangeKind: messaging.KindTopic,
			QueueName:    qName,
			RoutingKey:   intent.RoutingKey,
		}}

	case intent.Pattern == "event-stream" && intent.Direction == "consume" && intent.Ephemeral:
		exName := messaging.TopicExchangeName(messaging.DefaultEventExchangeName)
		return []spectest.ExpectedEndpoint{{
			Direction:       "consume",
			Pattern:         "event-stream",
			ExchangeName:    exName,
			ExchangeKind:    messaging.KindTopic,
			QueueNamePrefix: messaging.ServiceEventQueueName(exName, runtimeService) + "-",
			RoutingKey:      intent.RoutingKey,
			Ephemeral:       true,
		}}

	case intent.Pattern == "custom-stream" && intent.Direction == "publish":
		return []spectest.ExpectedEndpoint{{
			Direction:    "publish",
			Pattern:      "custom-stream",
			ExchangeName: messaging.TopicExchangeName(intent.Exchange),
			ExchangeKind: messaging.KindTopic,
		}}

	case intent.Pattern == "custom-stream" && intent.Direction == "consume" && !intent.Ephemeral:
		exName := messaging.TopicExchangeName(intent.Exchange)
		qName := messaging.ServiceEventQueueName(exName, runtimeService)
		if intent.QueueSuffix != "" {
			qName += "-" + intent.QueueSuffix
		}
		return []spectest.ExpectedEndpoint{{
			Direction:    "consume",
			Pattern:      "custom-stream",
			ExchangeName: exName,
			ExchangeKind: messaging.KindTopic,
			QueueName:    qName,
			RoutingKey:   intent.RoutingKey,
		}}

	case intent.Pattern == "custom-stream" && intent.Direction == "consume" && intent.Ephemeral:
		exName := messaging.TopicExchangeName(intent.Exchange)
		return []spectest.ExpectedEndpoint{{
			Direction:       "consume",
			Pattern:         "custom-stream",
			ExchangeName:    exName,
			ExchangeKind:    messaging.KindTopic,
			QueueNamePrefix: messaging.ServiceEventQueueName(exName, runtimeService) + "-",
			RoutingKey:      intent.RoutingKey,
			Ephemeral:       true,
		}}

	case intent.Pattern == "service-request" && intent.Direction == "consume":
		return []spectest.ExpectedEndpoint{{
			Direction:    "consume",
			Pattern:      "service-request",
			ExchangeName: messaging.ServiceRequestExchangeName(runtimeService),
			ExchangeKind: messaging.KindDirect,
			QueueName:    messaging.ServiceRequestQueueName(runtimeService),
			RoutingKey:   intent.RoutingKey,
		}}

	case intent.Pattern == "service-request" && intent.Direction == "publish":
		runtimeTarget := mapper.Runtime(intent.TargetService)
		return []spectest.ExpectedEndpoint{{
			Direction:    "publish",
			Pattern:      "service-request",
			ExchangeName: messaging.ServiceRequestExchangeName(runtimeTarget),
			ExchangeKind: messaging.KindDirect,
		}}

	case intent.Pattern == "service-response" && intent.Direction == "consume":
		runtimeTarget := mapper.Runtime(intent.TargetService)
		return []spectest.ExpectedEndpoint{{
			Direction:    "consume",
			Pattern:      "service-response",
			ExchangeName: messaging.ServiceResponseExchangeName(runtimeTarget),
			ExchangeKind: messaging.KindHeaders,
			QueueName:    messaging.ServiceResponseQueueName(runtimeTarget, runtimeService),
			RoutingKey:   intent.RoutingKey,
		}}

	case intent.Pattern == "service-response" && intent.Direction == "publish":
		// PublishServiceResponse doesn't register an endpoint — the response
		// exchange is declared by the consumer side. No topology entry expected.
		return nil

	case intent.Pattern == "queue-publish" && intent.Direction == "publish":
		return []spectest.ExpectedEndpoint{{
			Direction:    "publish",
			Pattern:      "queue-publish",
			ExchangeName: "(default)",
			QueueName:    intent.DestinationQueue,
		}}

	}
	return nil
}

func computeNATSEndpoints(runtimeService string, intent spectest.SetupIntent, mapper *NameMapper) []spectest.ExpectedEndpoint {
	switch {
	case intent.Pattern == "event-stream" && intent.Direction == "publish":
		return []spectest.ExpectedEndpoint{{
			Direction:    "publish",
			Pattern:      "event-stream",
			ExchangeName: messaging.DefaultEventExchangeName,
			ExchangeKind: messaging.KindTopic,
		}}

	case intent.Pattern == "event-stream" && intent.Direction == "consume" && !intent.Ephemeral:
		qName := runtimeService
		if intent.QueueSuffix != "" {
			qName += "-" + intent.QueueSuffix
		}
		return []spectest.ExpectedEndpoint{{
			Direction:    "consume",
			Pattern:      "event-stream",
			ExchangeName: messaging.DefaultEventExchangeName,
			ExchangeKind: messaging.KindTopic,
			QueueName:    qName,
			RoutingKey:   intent.RoutingKey,
		}}

	case intent.Pattern == "event-stream" && intent.Direction == "consume" && intent.Ephemeral:
		return []spectest.ExpectedEndpoint{{
			Direction:    "consume",
			Pattern:      "event-stream",
			ExchangeName: messaging.DefaultEventExchangeName,
			ExchangeKind: messaging.KindTopic,
			RoutingKey:   intent.RoutingKey,
			Ephemeral:    true,
		}}

	case intent.Pattern == "custom-stream" && intent.Direction == "publish":
		return []spectest.ExpectedEndpoint{{
			Direction:    "publish",
			Pattern:      "custom-stream",
			ExchangeName: intent.Exchange,
			ExchangeKind: messaging.KindTopic,
		}}

	case intent.Pattern == "custom-stream" && intent.Direction == "consume" && !intent.Ephemeral:
		qName := runtimeService
		if intent.QueueSuffix != "" {
			qName += "-" + intent.QueueSuffix
		}
		return []spectest.ExpectedEndpoint{{
			Direction:    "consume",
			Pattern:      "custom-stream",
			ExchangeName: intent.Exchange,
			ExchangeKind: messaging.KindTopic,
			QueueName:    qName,
			RoutingKey:   intent.RoutingKey,
		}}

	case intent.Pattern == "custom-stream" && intent.Direction == "consume" && intent.Ephemeral:
		return []spectest.ExpectedEndpoint{{
			Direction:    "consume",
			Pattern:      "custom-stream",
			ExchangeName: intent.Exchange,
			ExchangeKind: messaging.KindTopic,
			RoutingKey:   intent.RoutingKey,
			Ephemeral:    true,
		}}

	case intent.Pattern == "service-request" && intent.Direction == "consume":
		return []spectest.ExpectedEndpoint{{
			Direction:    "consume",
			Pattern:      "service-request",
			ExchangeName: runtimeService,
			ExchangeKind: messaging.KindDirect,
			QueueName:    runtimeService,
			RoutingKey:   intent.RoutingKey,
		}}

	case intent.Pattern == "service-request" && intent.Direction == "publish":
		runtimeTarget := mapper.Runtime(intent.TargetService)
		return []spectest.ExpectedEndpoint{{
			Direction:    "publish",
			Pattern:      "service-request",
			ExchangeName: runtimeTarget,
			ExchangeKind: messaging.KindDirect,
		}}

	case intent.Pattern == "service-response" && intent.Direction == "consume":
		runtimeTarget := mapper.Runtime(intent.TargetService)
		return []spectest.ExpectedEndpoint{{
			Direction:    "consume",
			Pattern:      "service-response",
			ExchangeName: runtimeTarget,
			ExchangeKind: messaging.KindHeaders,
			QueueName:    runtimeService,
			RoutingKey:   intent.RoutingKey,
		}}

	}
	return nil
}

// --- Broker state computation ---

const (
	durableQueueExpiry   = 432000000 // 5 days in ms
	ephemeralQueueExpiry = 1000      // 1 second in ms
)

func computeAMQPBrokerState(services map[string]spectest.ServiceConfig, mapper *NameMapper) spectest.AMQPBrokerState {
	exchangeMap := make(map[string]spectest.AMQPExchange)
	var queues []spectest.AMQPQueue
	var bindings []spectest.AMQPBinding

	for templateName, svcCfg := range services {
		runtimeName := mapper.Runtime(templateName)
		for _, intent := range svcCfg.Setups {
			switch {
			case intent.Pattern == "event-stream" && intent.Direction == "publish":
				exName := messaging.TopicExchangeName(messaging.DefaultEventExchangeName)
				exchangeMap[exName] = spectest.AMQPExchange{
					Name: exName, Type: messaging.KindTopic, Durable: true,
				}

			case intent.Pattern == "event-stream" && intent.Direction == "consume" && !intent.Ephemeral:
				exName := messaging.TopicExchangeName(messaging.DefaultEventExchangeName)
				exchangeMap[exName] = spectest.AMQPExchange{
					Name: exName, Type: messaging.KindTopic, Durable: true,
				}
				qName := messaging.ServiceEventQueueName(exName, runtimeName)
				if intent.QueueSuffix != "" {
					qName += "-" + intent.QueueSuffix
				}
				queues = append(queues, spectest.AMQPQueue{
					Name: qName, Durable: true,
					Arguments: spectest.QueueArguments{XQueueType: "quorum", XExpires: durableQueueExpiry},
				})
				bindings = append(bindings, spectest.AMQPBinding{
					Source: exName, Destination: qName, RoutingKey: intent.RoutingKey,
				})

			case intent.Pattern == "event-stream" && intent.Direction == "consume" && intent.Ephemeral:
				exName := messaging.TopicExchangeName(messaging.DefaultEventExchangeName)
				exchangeMap[exName] = spectest.AMQPExchange{
					Name: exName, Type: messaging.KindTopic, Durable: true,
				}
				qPrefix := messaging.ServiceEventQueueName(exName, runtimeName) + "-"
				queues = append(queues, spectest.AMQPQueue{
					NamePrefix: qPrefix, Durable: true,
					Arguments: spectest.QueueArguments{XQueueType: "quorum", XExpires: ephemeralQueueExpiry},
				})
				bindings = append(bindings, spectest.AMQPBinding{
					Source: exName, DestinationPrefix: qPrefix, RoutingKey: intent.RoutingKey,
				})

			case intent.Pattern == "custom-stream" && intent.Direction == "publish":
				exName := messaging.TopicExchangeName(intent.Exchange)
				exchangeMap[exName] = spectest.AMQPExchange{
					Name: exName, Type: messaging.KindTopic, Durable: true,
				}

			case intent.Pattern == "custom-stream" && intent.Direction == "consume" && !intent.Ephemeral:
				exName := messaging.TopicExchangeName(intent.Exchange)
				exchangeMap[exName] = spectest.AMQPExchange{
					Name: exName, Type: messaging.KindTopic, Durable: true,
				}
				qName := messaging.ServiceEventQueueName(exName, runtimeName)
				if intent.QueueSuffix != "" {
					qName += "-" + intent.QueueSuffix
				}
				queues = append(queues, spectest.AMQPQueue{
					Name: qName, Durable: true,
					Arguments: spectest.QueueArguments{XQueueType: "quorum", XExpires: durableQueueExpiry},
				})
				bindings = append(bindings, spectest.AMQPBinding{
					Source: exName, Destination: qName, RoutingKey: intent.RoutingKey,
				})

			case intent.Pattern == "custom-stream" && intent.Direction == "consume" && intent.Ephemeral:
				exName := messaging.TopicExchangeName(intent.Exchange)
				exchangeMap[exName] = spectest.AMQPExchange{
					Name: exName, Type: messaging.KindTopic, Durable: true,
				}
				qPrefix := messaging.ServiceEventQueueName(exName, runtimeName) + "-"
				queues = append(queues, spectest.AMQPQueue{
					NamePrefix: qPrefix, Durable: true,
					Arguments: spectest.QueueArguments{XQueueType: "quorum", XExpires: ephemeralQueueExpiry},
				})
				bindings = append(bindings, spectest.AMQPBinding{
					Source: exName, DestinationPrefix: qPrefix, RoutingKey: intent.RoutingKey,
				})

			case intent.Pattern == "service-request" && intent.Direction == "consume":
				exName := messaging.ServiceRequestExchangeName(runtimeName)
				exchangeMap[exName] = spectest.AMQPExchange{
					Name: exName, Type: messaging.KindDirect, Durable: true,
				}
				qName := messaging.ServiceRequestQueueName(runtimeName)
				queues = append(queues, spectest.AMQPQueue{
					Name: qName, Durable: true,
					Arguments: spectest.QueueArguments{XQueueType: "quorum", XExpires: durableQueueExpiry},
				})
				bindings = append(bindings, spectest.AMQPBinding{
					Source: exName, Destination: qName, RoutingKey: intent.RoutingKey,
				})

			case intent.Pattern == "service-request" && intent.Direction == "publish":
				runtimeTarget := mapper.Runtime(intent.TargetService)
				exName := messaging.ServiceRequestExchangeName(runtimeTarget)
				exchangeMap[exName] = spectest.AMQPExchange{
					Name: exName, Type: messaging.KindDirect, Durable: true,
				}

			case intent.Pattern == "service-response" && intent.Direction == "consume":
				runtimeTarget := mapper.Runtime(intent.TargetService)
				exName := messaging.ServiceResponseExchangeName(runtimeTarget)
				exchangeMap[exName] = spectest.AMQPExchange{
					Name: exName, Type: messaging.KindHeaders, Durable: true,
				}
				qName := messaging.ServiceResponseQueueName(runtimeTarget, runtimeName)
				queues = append(queues, spectest.AMQPQueue{
					Name: qName, Durable: true,
					Arguments: spectest.QueueArguments{XQueueType: "quorum", XExpires: durableQueueExpiry},
				})
				bindings = append(bindings, spectest.AMQPBinding{
					Source: exName, Destination: qName, RoutingKey: intent.RoutingKey,
				})

			case intent.Pattern == "service-response" && intent.Direction == "publish":
				// PublishServiceResponse doesn't declare resources — the response
				// exchange is declared by the consumer side.

			case intent.Pattern == "queue-publish" && intent.Direction == "publish":
				// QueuePublisher doesn't declare the queue — it expects it to
				// already exist. No broker state entry.

			}
		}
	}

	exchanges := make([]spectest.AMQPExchange, 0, len(exchangeMap))
	for _, ex := range exchangeMap {
		exchanges = append(exchanges, ex)
	}

	return spectest.AMQPBrokerState{
		Exchanges: exchanges,
		Queues:    queues,
		Bindings:  bindings,
	}
}

func computeNATSBrokerState(services map[string]spectest.ServiceConfig, mapper *NameMapper) spectest.NATSBrokerState {
	streamMap := make(map[string]spectest.NATSStream)
	var consumers []spectest.NATSConsumer

	for templateName, svcCfg := range services {
		runtimeName := mapper.Runtime(templateName)
		for _, intent := range svcCfg.Setups {
			switch {
			case intent.Pattern == "event-stream" && intent.Direction == "publish":
				streamName := messaging.DefaultEventExchangeName
				streamMap[streamName] = spectest.NATSStream{
					Name: streamName, Subjects: []string{streamName + ".>"}, Storage: "file",
				}

			case intent.Pattern == "event-stream" && intent.Direction == "consume" && !intent.Ephemeral:
				streamName := messaging.DefaultEventExchangeName
				streamMap[streamName] = spectest.NATSStream{
					Name: streamName, Subjects: []string{streamName + ".>"}, Storage: "file",
				}
				durableName := runtimeName
				if intent.QueueSuffix != "" {
					durableName += "-" + intent.QueueSuffix
				}
				consumers = append(consumers, spectest.NATSConsumer{
					Stream:        streamName,
					Durable:       durableName,
					FilterSubject: messaging.NATSSubject(streamName, messaging.TranslateWildcard(intent.RoutingKey)),
					AckPolicy:     "explicit",
				})

			case intent.Pattern == "event-stream" && intent.Direction == "consume" && intent.Ephemeral:
				streamName := messaging.DefaultEventExchangeName
				streamMap[streamName] = spectest.NATSStream{
					Name: streamName, Subjects: []string{streamName + ".>"}, Storage: "file",
				}
				consumers = append(consumers, spectest.NATSConsumer{
					Stream:        streamName,
					FilterSubject: messaging.NATSSubject(streamName, messaging.TranslateWildcard(intent.RoutingKey)),
					AckPolicy:     "explicit",
				})

			case intent.Pattern == "custom-stream" && intent.Direction == "publish":
				streamName := intent.Exchange
				streamMap[streamName] = spectest.NATSStream{
					Name: streamName, Subjects: []string{streamName + ".>"}, Storage: "file",
				}

			case intent.Pattern == "custom-stream" && intent.Direction == "consume" && !intent.Ephemeral:
				streamName := intent.Exchange
				streamMap[streamName] = spectest.NATSStream{
					Name: streamName, Subjects: []string{streamName + ".>"}, Storage: "file",
				}
				durableName := runtimeName
				if intent.QueueSuffix != "" {
					durableName += "-" + intent.QueueSuffix
				}
				consumers = append(consumers, spectest.NATSConsumer{
					Stream:        streamName,
					Durable:       durableName,
					FilterSubject: messaging.NATSSubject(streamName, messaging.TranslateWildcard(intent.RoutingKey)),
					AckPolicy:     "explicit",
				})

			case intent.Pattern == "custom-stream" && intent.Direction == "consume" && intent.Ephemeral:
				streamName := intent.Exchange
				streamMap[streamName] = spectest.NATSStream{
					Name: streamName, Subjects: []string{streamName + ".>"}, Storage: "file",
				}
				consumers = append(consumers, spectest.NATSConsumer{
					Stream:        streamName,
					FilterSubject: messaging.NATSSubject(streamName, messaging.TranslateWildcard(intent.RoutingKey)),
					AckPolicy:     "explicit",
				})

			// service-request/service-response: core NATS, no JetStream resources.
			}
		}
	}

	streams := make([]spectest.NATSStream, 0, len(streamMap))
	for _, s := range streamMap {
		streams = append(streams, s)
	}

	return spectest.NATSBrokerState{
		Streams:   streams,
		Consumers: consumers,
	}
}
