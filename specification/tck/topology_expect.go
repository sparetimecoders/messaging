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
	"github.com/sparetimecoders/gomessaging/spec"
	"github.com/sparetimecoders/gomessaging/spec/spectest"
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
			Exchange:   spec.TopicExchangeName(baseExchange),
			RoutingKey: probe.RoutingKey,
		}
	case "nats":
		return spectest.ProbeTarget{
			Stream:  baseExchange,
			Subject: spec.NATSSubject(baseExchange, probe.RoutingKey),
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
		return spec.DefaultEventExchangeName
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
			ExchangeName: spec.TopicExchangeName(spec.DefaultEventExchangeName),
			ExchangeKind: spec.KindTopic,
		}}

	case intent.Pattern == "event-stream" && intent.Direction == "consume" && !intent.Ephemeral:
		exName := spec.TopicExchangeName(spec.DefaultEventExchangeName)
		qName := spec.ServiceEventQueueName(exName, runtimeService)
		if intent.QueueSuffix != "" {
			qName += "-" + intent.QueueSuffix
		}
		return []spectest.ExpectedEndpoint{{
			Direction:    "consume",
			Pattern:      "event-stream",
			ExchangeName: exName,
			ExchangeKind: spec.KindTopic,
			QueueName:    qName,
			RoutingKey:   intent.RoutingKey,
		}}

	case intent.Pattern == "event-stream" && intent.Direction == "consume" && intent.Ephemeral:
		exName := spec.TopicExchangeName(spec.DefaultEventExchangeName)
		return []spectest.ExpectedEndpoint{{
			Direction:       "consume",
			Pattern:         "event-stream",
			ExchangeName:    exName,
			ExchangeKind:    spec.KindTopic,
			QueueNamePrefix: spec.ServiceEventQueueName(exName, runtimeService) + "-",
			RoutingKey:      intent.RoutingKey,
			Ephemeral:       true,
		}}

	case intent.Pattern == "custom-stream" && intent.Direction == "publish":
		return []spectest.ExpectedEndpoint{{
			Direction:    "publish",
			Pattern:      "custom-stream",
			ExchangeName: spec.TopicExchangeName(intent.Exchange),
			ExchangeKind: spec.KindTopic,
		}}

	case intent.Pattern == "custom-stream" && intent.Direction == "consume" && !intent.Ephemeral:
		exName := spec.TopicExchangeName(intent.Exchange)
		qName := spec.ServiceEventQueueName(exName, runtimeService)
		if intent.QueueSuffix != "" {
			qName += "-" + intent.QueueSuffix
		}
		return []spectest.ExpectedEndpoint{{
			Direction:    "consume",
			Pattern:      "custom-stream",
			ExchangeName: exName,
			ExchangeKind: spec.KindTopic,
			QueueName:    qName,
			RoutingKey:   intent.RoutingKey,
		}}

	case intent.Pattern == "custom-stream" && intent.Direction == "consume" && intent.Ephemeral:
		exName := spec.TopicExchangeName(intent.Exchange)
		return []spectest.ExpectedEndpoint{{
			Direction:       "consume",
			Pattern:         "custom-stream",
			ExchangeName:    exName,
			ExchangeKind:    spec.KindTopic,
			QueueNamePrefix: spec.ServiceEventQueueName(exName, runtimeService) + "-",
			RoutingKey:      intent.RoutingKey,
			Ephemeral:       true,
		}}

	case intent.Pattern == "service-request" && intent.Direction == "consume":
		return []spectest.ExpectedEndpoint{{
			Direction:    "consume",
			Pattern:      "service-request",
			ExchangeName: spec.ServiceRequestExchangeName(runtimeService),
			ExchangeKind: spec.KindDirect,
			QueueName:    spec.ServiceRequestQueueName(runtimeService),
			RoutingKey:   intent.RoutingKey,
		}}

	case intent.Pattern == "service-request" && intent.Direction == "publish":
		runtimeTarget := mapper.Runtime(intent.TargetService)
		return []spectest.ExpectedEndpoint{{
			Direction:    "publish",
			Pattern:      "service-request",
			ExchangeName: spec.ServiceRequestExchangeName(runtimeTarget),
			ExchangeKind: spec.KindDirect,
		}}

	case intent.Pattern == "service-response" && intent.Direction == "consume":
		runtimeTarget := mapper.Runtime(intent.TargetService)
		return []spectest.ExpectedEndpoint{{
			Direction:    "consume",
			Pattern:      "service-response",
			ExchangeName: spec.ServiceResponseExchangeName(runtimeTarget),
			ExchangeKind: spec.KindHeaders,
			QueueName:    spec.ServiceResponseQueueName(runtimeTarget, runtimeService),
			RoutingKey:   intent.RoutingKey,
		}}

	case intent.Pattern == "service-response" && intent.Direction == "publish":
		return []spectest.ExpectedEndpoint{{
			Direction:    "publish",
			Pattern:      "service-response",
			ExchangeName: spec.ServiceResponseExchangeName(runtimeService),
			ExchangeKind: spec.KindHeaders,
		}}

	case intent.Pattern == "queue-publish" && intent.Direction == "publish":
		return []spectest.ExpectedEndpoint{{
			Direction:    "publish",
			Pattern:      "queue-publish",
			ExchangeName: "(default)",
			ExchangeKind: spec.KindDirect,
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
			ExchangeName: spec.DefaultEventExchangeName,
			ExchangeKind: spec.KindTopic,
		}}

	case intent.Pattern == "event-stream" && intent.Direction == "consume" && !intent.Ephemeral:
		qName := runtimeService
		if intent.QueueSuffix != "" {
			qName += "-" + intent.QueueSuffix
		}
		return []spectest.ExpectedEndpoint{{
			Direction:    "consume",
			Pattern:      "event-stream",
			ExchangeName: spec.DefaultEventExchangeName,
			ExchangeKind: spec.KindTopic,
			QueueName:    qName,
			RoutingKey:   intent.RoutingKey,
		}}

	case intent.Pattern == "event-stream" && intent.Direction == "consume" && intent.Ephemeral:
		return []spectest.ExpectedEndpoint{{
			Direction:    "consume",
			Pattern:      "event-stream",
			ExchangeName: spec.DefaultEventExchangeName,
			ExchangeKind: spec.KindTopic,
			RoutingKey:   intent.RoutingKey,
			Ephemeral:    true,
		}}

	case intent.Pattern == "custom-stream" && intent.Direction == "publish":
		return []spectest.ExpectedEndpoint{{
			Direction:    "publish",
			Pattern:      "custom-stream",
			ExchangeName: intent.Exchange,
			ExchangeKind: spec.KindTopic,
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
			ExchangeKind: spec.KindTopic,
			QueueName:    qName,
			RoutingKey:   intent.RoutingKey,
		}}

	case intent.Pattern == "custom-stream" && intent.Direction == "consume" && intent.Ephemeral:
		return []spectest.ExpectedEndpoint{{
			Direction:    "consume",
			Pattern:      "custom-stream",
			ExchangeName: intent.Exchange,
			ExchangeKind: spec.KindTopic,
			RoutingKey:   intent.RoutingKey,
			Ephemeral:    true,
		}}

	case intent.Pattern == "service-request" && intent.Direction == "consume":
		return []spectest.ExpectedEndpoint{{
			Direction:    "consume",
			Pattern:      "service-request",
			ExchangeName: runtimeService,
			ExchangeKind: spec.KindDirect,
			QueueName:    runtimeService,
			RoutingKey:   intent.RoutingKey,
		}}

	case intent.Pattern == "service-request" && intent.Direction == "publish":
		runtimeTarget := mapper.Runtime(intent.TargetService)
		return []spectest.ExpectedEndpoint{{
			Direction:    "publish",
			Pattern:      "service-request",
			ExchangeName: runtimeTarget,
			ExchangeKind: spec.KindDirect,
		}}

	case intent.Pattern == "service-response" && intent.Direction == "consume":
		runtimeTarget := mapper.Runtime(intent.TargetService)
		return []spectest.ExpectedEndpoint{{
			Direction:    "consume",
			Pattern:      "service-response",
			ExchangeName: runtimeTarget,
			ExchangeKind: spec.KindHeaders,
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
				exName := spec.TopicExchangeName(spec.DefaultEventExchangeName)
				exchangeMap[exName] = spectest.AMQPExchange{
					Name: exName, Type: spec.KindTopic, Durable: true,
				}

			case intent.Pattern == "event-stream" && intent.Direction == "consume" && !intent.Ephemeral:
				exName := spec.TopicExchangeName(spec.DefaultEventExchangeName)
				exchangeMap[exName] = spectest.AMQPExchange{
					Name: exName, Type: spec.KindTopic, Durable: true,
				}
				qName := spec.ServiceEventQueueName(exName, runtimeName)
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
				exName := spec.TopicExchangeName(spec.DefaultEventExchangeName)
				exchangeMap[exName] = spectest.AMQPExchange{
					Name: exName, Type: spec.KindTopic, Durable: true,
				}
				qPrefix := spec.ServiceEventQueueName(exName, runtimeName) + "-"
				queues = append(queues, spectest.AMQPQueue{
					NamePrefix: qPrefix, Durable: true,
					Arguments: spectest.QueueArguments{XQueueType: "quorum", XExpires: ephemeralQueueExpiry},
				})
				bindings = append(bindings, spectest.AMQPBinding{
					Source: exName, DestinationPrefix: qPrefix, RoutingKey: intent.RoutingKey,
				})

			case intent.Pattern == "custom-stream" && intent.Direction == "publish":
				exName := spec.TopicExchangeName(intent.Exchange)
				exchangeMap[exName] = spectest.AMQPExchange{
					Name: exName, Type: spec.KindTopic, Durable: true,
				}

			case intent.Pattern == "custom-stream" && intent.Direction == "consume" && !intent.Ephemeral:
				exName := spec.TopicExchangeName(intent.Exchange)
				exchangeMap[exName] = spectest.AMQPExchange{
					Name: exName, Type: spec.KindTopic, Durable: true,
				}
				qName := spec.ServiceEventQueueName(exName, runtimeName)
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
				exName := spec.TopicExchangeName(intent.Exchange)
				exchangeMap[exName] = spectest.AMQPExchange{
					Name: exName, Type: spec.KindTopic, Durable: true,
				}
				qPrefix := spec.ServiceEventQueueName(exName, runtimeName) + "-"
				queues = append(queues, spectest.AMQPQueue{
					NamePrefix: qPrefix, Durable: true,
					Arguments: spectest.QueueArguments{XQueueType: "quorum", XExpires: ephemeralQueueExpiry},
				})
				bindings = append(bindings, spectest.AMQPBinding{
					Source: exName, DestinationPrefix: qPrefix, RoutingKey: intent.RoutingKey,
				})

			case intent.Pattern == "service-request" && intent.Direction == "consume":
				exName := spec.ServiceRequestExchangeName(runtimeName)
				exchangeMap[exName] = spectest.AMQPExchange{
					Name: exName, Type: spec.KindDirect, Durable: true,
				}
				qName := spec.ServiceRequestQueueName(runtimeName)
				queues = append(queues, spectest.AMQPQueue{
					Name: qName, Durable: true,
					Arguments: spectest.QueueArguments{XQueueType: "quorum", XExpires: durableQueueExpiry},
				})
				bindings = append(bindings, spectest.AMQPBinding{
					Source: exName, Destination: qName, RoutingKey: intent.RoutingKey,
				})

			case intent.Pattern == "service-request" && intent.Direction == "publish":
				runtimeTarget := mapper.Runtime(intent.TargetService)
				exName := spec.ServiceRequestExchangeName(runtimeTarget)
				exchangeMap[exName] = spectest.AMQPExchange{
					Name: exName, Type: spec.KindDirect, Durable: true,
				}

			case intent.Pattern == "service-response" && intent.Direction == "consume":
				runtimeTarget := mapper.Runtime(intent.TargetService)
				exName := spec.ServiceResponseExchangeName(runtimeTarget)
				exchangeMap[exName] = spectest.AMQPExchange{
					Name: exName, Type: spec.KindHeaders, Durable: true,
				}
				qName := spec.ServiceResponseQueueName(runtimeTarget, runtimeName)
				queues = append(queues, spectest.AMQPQueue{
					Name: qName, Durable: true,
					Arguments: spectest.QueueArguments{XQueueType: "quorum", XExpires: durableQueueExpiry},
				})
				bindings = append(bindings, spectest.AMQPBinding{
					Source: exName, Destination: qName, RoutingKey: intent.RoutingKey,
				})

			case intent.Pattern == "service-response" && intent.Direction == "publish":
				exName := spec.ServiceResponseExchangeName(runtimeName)
				exchangeMap[exName] = spectest.AMQPExchange{
					Name: exName, Type: spec.KindHeaders, Durable: true,
				}

			case intent.Pattern == "queue-publish" && intent.Direction == "publish":
				queues = append(queues, spectest.AMQPQueue{
					Name: intent.DestinationQueue, Durable: true,
					Arguments: spectest.QueueArguments{XQueueType: "quorum", XExpires: durableQueueExpiry},
				})

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
				streamName := spec.DefaultEventExchangeName
				streamMap[streamName] = spectest.NATSStream{
					Name: streamName, Subjects: []string{streamName + ".>"}, Storage: "file",
				}

			case intent.Pattern == "event-stream" && intent.Direction == "consume" && !intent.Ephemeral:
				streamName := spec.DefaultEventExchangeName
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
					FilterSubject: spec.NATSSubject(streamName, spec.TranslateWildcard(intent.RoutingKey)),
					AckPolicy:     "explicit",
				})

			case intent.Pattern == "event-stream" && intent.Direction == "consume" && intent.Ephemeral:
				streamName := spec.DefaultEventExchangeName
				streamMap[streamName] = spectest.NATSStream{
					Name: streamName, Subjects: []string{streamName + ".>"}, Storage: "file",
				}
				consumers = append(consumers, spectest.NATSConsumer{
					Stream:        streamName,
					FilterSubject: spec.NATSSubject(streamName, spec.TranslateWildcard(intent.RoutingKey)),
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
					FilterSubject: spec.NATSSubject(streamName, spec.TranslateWildcard(intent.RoutingKey)),
					AckPolicy:     "explicit",
				})

			case intent.Pattern == "custom-stream" && intent.Direction == "consume" && intent.Ephemeral:
				streamName := intent.Exchange
				streamMap[streamName] = spectest.NATSStream{
					Name: streamName, Subjects: []string{streamName + ".>"}, Storage: "file",
				}
				consumers = append(consumers, spectest.NATSConsumer{
					Stream:        streamName,
					FilterSubject: spec.NATSSubject(streamName, spec.TranslateWildcard(intent.RoutingKey)),
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
