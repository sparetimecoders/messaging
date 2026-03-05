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
	"testing"

	"github.com/sparetimecoders/messaging/specification/spec"
	"github.com/sparetimecoders/messaging/specification/spec/spectest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComputeExpectedEndpointsAMQPEventStream(t *testing.T) {
	mapper := NewNameMapper([]string{"orders", "notifications"})

	services := map[string]spectest.ServiceConfig{
		"orders":        {Setups: []spectest.SetupIntent{{Pattern: "event-stream", Direction: "publish"}}},
		"notifications": {Setups: []spectest.SetupIntent{{Pattern: "event-stream", Direction: "consume", RoutingKey: "Order.Created"}}},
	}

	endpoints := ComputeExpectedEndpoints("amqp", services, mapper)

	ordersEP := endpoints["orders"]
	require.Len(t, ordersEP, 1)
	assert.Equal(t, "publish", ordersEP[0].Direction)
	assert.Equal(t, "events.topic.exchange", ordersEP[0].ExchangeName)
	assert.Equal(t, "topic", ordersEP[0].ExchangeKind)

	notifEP := endpoints["notifications"]
	require.Len(t, notifEP, 1)
	assert.Equal(t, "consume", notifEP[0].Direction)
	assert.Equal(t, "events.topic.exchange", notifEP[0].ExchangeName)
	assert.Equal(t, "events.topic.exchange.queue."+mapper.Runtime("notifications"), notifEP[0].QueueName)
	assert.Equal(t, "Order.Created", notifEP[0].RoutingKey)
}

func TestComputeExpectedEndpointsNATSEventStream(t *testing.T) {
	mapper := NewNameMapper([]string{"orders", "notifications"})

	services := map[string]spectest.ServiceConfig{
		"orders":        {Setups: []spectest.SetupIntent{{Pattern: "event-stream", Direction: "publish"}}},
		"notifications": {Setups: []spectest.SetupIntent{{Pattern: "event-stream", Direction: "consume", RoutingKey: "Order.Created"}}},
	}

	endpoints := ComputeExpectedEndpoints("nats", services, mapper)

	ordersEP := endpoints["orders"]
	require.Len(t, ordersEP, 1)
	assert.Equal(t, "events", ordersEP[0].ExchangeName)
	assert.Equal(t, "topic", ordersEP[0].ExchangeKind)

	notifEP := endpoints["notifications"]
	require.Len(t, notifEP, 1)
	assert.Equal(t, mapper.Runtime("notifications"), notifEP[0].QueueName)
	assert.Equal(t, "Order.Created", notifEP[0].RoutingKey)
}

func TestComputeExpectedEndpointsAMQPServiceRequest(t *testing.T) {
	mapper := NewNameMapper([]string{"email-svc", "web-app"})

	services := map[string]spectest.ServiceConfig{
		"email-svc": {Setups: []spectest.SetupIntent{
			{Pattern: "service-request", Direction: "consume", RoutingKey: "email.send"},
		}},
		"web-app": {Setups: []spectest.SetupIntent{
			{Pattern: "service-request", Direction: "publish", TargetService: "email-svc"},
			{Pattern: "service-response", Direction: "consume", TargetService: "email-svc", RoutingKey: "email.send"},
		}},
	}

	endpoints := ComputeExpectedEndpoints("amqp", services, mapper)

	emailEP := endpoints["email-svc"]
	require.Len(t, emailEP, 1)
	runtimeEmail := mapper.Runtime("email-svc")
	assert.Equal(t, spec.ServiceRequestExchangeName(runtimeEmail), emailEP[0].ExchangeName)
	assert.Equal(t, spec.ServiceRequestQueueName(runtimeEmail), emailEP[0].QueueName)

	webEP := endpoints["web-app"]
	require.Len(t, webEP, 2)
	// Publish endpoint uses the target's request exchange.
	assert.Equal(t, "publish", webEP[0].Direction)
	assert.Equal(t, spec.ServiceRequestExchangeName(runtimeEmail), webEP[0].ExchangeName)
	// Response consume endpoint uses the target's response exchange.
	runtimeWeb := mapper.Runtime("web-app")
	assert.Equal(t, "consume", webEP[1].Direction)
	assert.Equal(t, spec.ServiceResponseExchangeName(runtimeEmail), webEP[1].ExchangeName)
	assert.Equal(t, spec.ServiceResponseQueueName(runtimeEmail, runtimeWeb), webEP[1].QueueName)
}

func TestComputeExpectedEndpointsEphemeral(t *testing.T) {
	mapper := NewNameMapper([]string{"orders", "dashboard"})

	services := map[string]spectest.ServiceConfig{
		"orders":    {Setups: []spectest.SetupIntent{{Pattern: "event-stream", Direction: "publish"}}},
		"dashboard": {Setups: []spectest.SetupIntent{{Pattern: "event-stream", Direction: "consume", RoutingKey: "Order.Created", Ephemeral: true}}},
	}

	// AMQP: ephemeral consumer has QueueNamePrefix.
	amqpEP := ComputeExpectedEndpoints("amqp", services, mapper)
	dashEP := amqpEP["dashboard"]
	require.Len(t, dashEP, 1)
	assert.True(t, dashEP[0].Ephemeral)
	runtimeDash := mapper.Runtime("dashboard")
	assert.Equal(t, spec.ServiceEventQueueName("events.topic.exchange", runtimeDash)+"-", dashEP[0].QueueNamePrefix)
	assert.Empty(t, dashEP[0].QueueName)

	// NATS: ephemeral consumer has no QueueName.
	natsEP := ComputeExpectedEndpoints("nats", services, mapper)
	natsDashEP := natsEP["dashboard"]
	require.Len(t, natsDashEP, 1)
	assert.True(t, natsDashEP[0].Ephemeral)
	assert.Empty(t, natsDashEP[0].QueueName)
}

func TestComputeExpectedBrokerStateAMQP(t *testing.T) {
	mapper := NewNameMapper([]string{"orders", "notifications"})

	services := map[string]spectest.ServiceConfig{
		"orders":        {Setups: []spectest.SetupIntent{{Pattern: "event-stream", Direction: "publish"}}},
		"notifications": {Setups: []spectest.SetupIntent{{Pattern: "event-stream", Direction: "consume", RoutingKey: "Order.Created"}}},
	}

	broker := ComputeExpectedBrokerState("amqp", services, mapper)

	require.Len(t, broker.AMQP.Exchanges, 1)
	assert.Equal(t, "events.topic.exchange", broker.AMQP.Exchanges[0].Name)
	assert.Equal(t, "topic", broker.AMQP.Exchanges[0].Type)
	assert.True(t, broker.AMQP.Exchanges[0].Durable)

	runtimeNotif := mapper.Runtime("notifications")
	expectedQueue := "events.topic.exchange.queue." + runtimeNotif

	require.Len(t, broker.AMQP.Queues, 1)
	assert.Equal(t, expectedQueue, broker.AMQP.Queues[0].Name)
	assert.True(t, broker.AMQP.Queues[0].Durable)
	assert.Equal(t, "quorum", broker.AMQP.Queues[0].Arguments.XQueueType)
	assert.Equal(t, 432000000, broker.AMQP.Queues[0].Arguments.XExpires)

	require.Len(t, broker.AMQP.Bindings, 1)
	assert.Equal(t, "events.topic.exchange", broker.AMQP.Bindings[0].Source)
	assert.Equal(t, expectedQueue, broker.AMQP.Bindings[0].Destination)
	assert.Equal(t, "Order.Created", broker.AMQP.Bindings[0].RoutingKey)
}

func TestComputeExpectedBrokerStateNATS(t *testing.T) {
	mapper := NewNameMapper([]string{"orders", "notifications"})

	services := map[string]spectest.ServiceConfig{
		"orders":        {Setups: []spectest.SetupIntent{{Pattern: "event-stream", Direction: "publish"}}},
		"notifications": {Setups: []spectest.SetupIntent{{Pattern: "event-stream", Direction: "consume", RoutingKey: "Order.Created"}}},
	}

	broker := ComputeExpectedBrokerState("nats", services, mapper)

	require.Len(t, broker.NATS.Streams, 1)
	assert.Equal(t, "events", broker.NATS.Streams[0].Name)
	assert.Equal(t, []string{"events.>"}, broker.NATS.Streams[0].Subjects)
	assert.Equal(t, "file", broker.NATS.Streams[0].Storage)

	require.Len(t, broker.NATS.Consumers, 1)
	assert.Equal(t, "events", broker.NATS.Consumers[0].Stream)
	assert.Equal(t, mapper.Runtime("notifications"), broker.NATS.Consumers[0].Durable)
	assert.Equal(t, "events.Order.Created", broker.NATS.Consumers[0].FilterSubject)
	assert.Equal(t, "explicit", broker.NATS.Consumers[0].AckPolicy)
}

func TestComputeExpectedBrokerStateServiceRequest(t *testing.T) {
	mapper := NewNameMapper([]string{"email-svc", "web-app"})

	services := map[string]spectest.ServiceConfig{
		"email-svc": {Setups: []spectest.SetupIntent{
			{Pattern: "service-request", Direction: "consume", RoutingKey: "email.send"},
		}},
		"web-app": {Setups: []spectest.SetupIntent{
			{Pattern: "service-request", Direction: "publish", TargetService: "email-svc"},
			{Pattern: "service-response", Direction: "consume", TargetService: "email-svc", RoutingKey: "email.send"},
		}},
	}

	// NATS: service-request uses core NATS, no JetStream resources.
	natsBroker := ComputeExpectedBrokerState("nats", services, mapper)
	assert.Empty(t, natsBroker.NATS.Streams)
	assert.Empty(t, natsBroker.NATS.Consumers)

	// AMQP: should have request + response exchanges, queues, bindings.
	amqpBroker := ComputeExpectedBrokerState("amqp", services, mapper)

	runtimeEmail := mapper.Runtime("email-svc")
	runtimeWeb := mapper.Runtime("web-app")

	require.Len(t, amqpBroker.AMQP.Exchanges, 2)
	require.Len(t, amqpBroker.AMQP.Queues, 2)
	require.Len(t, amqpBroker.AMQP.Bindings, 2)

	// Verify request exchange exists.
	reqExName := spec.ServiceRequestExchangeName(runtimeEmail)
	var foundReqExchange bool
	for _, ex := range amqpBroker.AMQP.Exchanges {
		if ex.Name == reqExName {
			foundReqExchange = true
			assert.Equal(t, "direct", ex.Type)
		}
	}
	assert.True(t, foundReqExchange, "request exchange not found")

	// Verify response exchange exists.
	respExName := spec.ServiceResponseExchangeName(runtimeEmail)
	var foundRespExchange bool
	for _, ex := range amqpBroker.AMQP.Exchanges {
		if ex.Name == respExName {
			foundRespExchange = true
			assert.Equal(t, "headers", ex.Type)
		}
	}
	assert.True(t, foundRespExchange, "response exchange not found")

	// Verify response queue.
	respQName := spec.ServiceResponseQueueName(runtimeEmail, runtimeWeb)
	var foundRespQueue bool
	for _, q := range amqpBroker.AMQP.Queues {
		if q.Name == respQName {
			foundRespQueue = true
		}
	}
	assert.True(t, foundRespQueue, "response queue %s not found", respQName)
}

func TestComputeExpectedEndpointsAMQPSuffix(t *testing.T) {
	mapper := NewNameMapper([]string{"orders", "backend"})

	services := map[string]spectest.ServiceConfig{
		"orders": {Setups: []spectest.SetupIntent{{Pattern: "event-stream", Direction: "publish"}}},
		"backend": {Setups: []spectest.SetupIntent{
			{Pattern: "event-stream", Direction: "consume", RoutingKey: "Order.Created", QueueSuffix: "reminders"},
			{Pattern: "event-stream", Direction: "consume", RoutingKey: "Order.Created", QueueSuffix: "reporting"},
		}},
	}

	endpoints := ComputeExpectedEndpoints("amqp", services, mapper)

	backendEP := endpoints["backend"]
	require.Len(t, backendEP, 2)

	runtimeBackend := mapper.Runtime("backend")
	exName := spec.TopicExchangeName(spec.DefaultEventExchangeName)
	baseQueue := spec.ServiceEventQueueName(exName, runtimeBackend)

	assert.Equal(t, baseQueue+"-reminders", backendEP[0].QueueName)
	assert.Equal(t, baseQueue+"-reporting", backendEP[1].QueueName)
}

func TestComputeExpectedEndpointsNATSSuffix(t *testing.T) {
	mapper := NewNameMapper([]string{"orders", "backend"})

	services := map[string]spectest.ServiceConfig{
		"orders": {Setups: []spectest.SetupIntent{{Pattern: "event-stream", Direction: "publish"}}},
		"backend": {Setups: []spectest.SetupIntent{
			{Pattern: "event-stream", Direction: "consume", RoutingKey: "Order.Created", QueueSuffix: "reminders"},
			{Pattern: "event-stream", Direction: "consume", RoutingKey: "Order.Created", QueueSuffix: "reporting"},
		}},
	}

	endpoints := ComputeExpectedEndpoints("nats", services, mapper)

	backendEP := endpoints["backend"]
	require.Len(t, backendEP, 2)

	runtimeBackend := mapper.Runtime("backend")
	assert.Equal(t, runtimeBackend+"-reminders", backendEP[0].QueueName)
	assert.Equal(t, runtimeBackend+"-reporting", backendEP[1].QueueName)
}

func TestComputeExpectedEndpointsAMQPServiceResponsePublish(t *testing.T) {
	mapper := NewNameMapper([]string{"task-svc"})

	services := map[string]spectest.ServiceConfig{
		"task-svc": {Setups: []spectest.SetupIntent{
			{Pattern: "service-response", Direction: "publish"},
		}},
	}

	endpoints := ComputeExpectedEndpoints("amqp", services, mapper)

	// PublishServiceResponse doesn't register an endpoint — no topology entry.
	taskEP := endpoints["task-svc"]
	assert.Empty(t, taskEP)
}

func TestComputeExpectedEndpointsAMQPQueuePublish(t *testing.T) {
	mapper := NewNameMapper([]string{"sender"})

	services := map[string]spectest.ServiceConfig{
		"sender": {Setups: []spectest.SetupIntent{
			{Pattern: "queue-publish", Direction: "publish", DestinationQueue: "task-queue"},
		}},
	}

	endpoints := ComputeExpectedEndpoints("amqp", services, mapper)

	senderEP := endpoints["sender"]
	require.Len(t, senderEP, 1)
	assert.Equal(t, "publish", senderEP[0].Direction)
	assert.Equal(t, "(default)", senderEP[0].ExchangeName)
	assert.Empty(t, senderEP[0].ExchangeKind)
	assert.Equal(t, "task-queue", senderEP[0].QueueName)
}

func TestComputeExpectedBrokerStateAMQPSuffix(t *testing.T) {
	mapper := NewNameMapper([]string{"orders", "backend"})

	services := map[string]spectest.ServiceConfig{
		"orders": {Setups: []spectest.SetupIntent{{Pattern: "event-stream", Direction: "publish"}}},
		"backend": {Setups: []spectest.SetupIntent{
			{Pattern: "event-stream", Direction: "consume", RoutingKey: "Order.Created", QueueSuffix: "reminders"},
			{Pattern: "event-stream", Direction: "consume", RoutingKey: "Order.Created", QueueSuffix: "reporting"},
		}},
	}

	broker := ComputeExpectedBrokerState("amqp", services, mapper)

	runtimeBackend := mapper.Runtime("backend")
	exName := spec.TopicExchangeName(spec.DefaultEventExchangeName)
	baseQueue := spec.ServiceEventQueueName(exName, runtimeBackend)

	require.Len(t, broker.AMQP.Queues, 2)
	queueNames := []string{broker.AMQP.Queues[0].Name, broker.AMQP.Queues[1].Name}
	assert.Contains(t, queueNames, baseQueue+"-reminders")
	assert.Contains(t, queueNames, baseQueue+"-reporting")

	require.Len(t, broker.AMQP.Bindings, 2)
}

func TestComputeExpectedBrokerStateNATSSuffix(t *testing.T) {
	mapper := NewNameMapper([]string{"orders", "backend"})

	services := map[string]spectest.ServiceConfig{
		"orders": {Setups: []spectest.SetupIntent{{Pattern: "event-stream", Direction: "publish"}}},
		"backend": {Setups: []spectest.SetupIntent{
			{Pattern: "event-stream", Direction: "consume", RoutingKey: "Order.Created", QueueSuffix: "reminders"},
			{Pattern: "event-stream", Direction: "consume", RoutingKey: "Order.Created", QueueSuffix: "reporting"},
		}},
	}

	broker := ComputeExpectedBrokerState("nats", services, mapper)

	runtimeBackend := mapper.Runtime("backend")
	require.Len(t, broker.NATS.Consumers, 2)
	durables := []string{broker.NATS.Consumers[0].Durable, broker.NATS.Consumers[1].Durable}
	assert.Contains(t, durables, runtimeBackend+"-reminders")
	assert.Contains(t, durables, runtimeBackend+"-reporting")
}

func TestComputeExpectedBrokerStateAMQPQueuePublish(t *testing.T) {
	mapper := NewNameMapper([]string{"sender"})

	services := map[string]spectest.ServiceConfig{
		"sender": {Setups: []spectest.SetupIntent{
			{Pattern: "queue-publish", Direction: "publish", DestinationQueue: "task-queue"},
		}},
	}

	broker := ComputeExpectedBrokerState("amqp", services, mapper)

	// QueuePublisher doesn't declare any resources — queue must already exist.
	assert.Empty(t, broker.AMQP.Exchanges)
	assert.Empty(t, broker.AMQP.Queues)
	assert.Empty(t, broker.AMQP.Bindings)
}

func TestComputeExpectedBrokerStateAMQPServiceResponsePublish(t *testing.T) {
	mapper := NewNameMapper([]string{"task-svc"})

	services := map[string]spectest.ServiceConfig{
		"task-svc": {Setups: []spectest.SetupIntent{
			{Pattern: "service-response", Direction: "publish"},
		}},
	}

	broker := ComputeExpectedBrokerState("amqp", services, mapper)

	// PublishServiceResponse doesn't declare resources — the response exchange
	// is declared by the consumer side.
	assert.Empty(t, broker.AMQP.Exchanges)
	assert.Empty(t, broker.AMQP.Queues)
	assert.Empty(t, broker.AMQP.Bindings)
}

func TestComputeProbeTarget(t *testing.T) {
	mapper := NewNameMapper([]string{"orders", "notifications"})

	services := map[string]spectest.ServiceConfig{
		"orders":        {Setups: []spectest.SetupIntent{{Pattern: "event-stream", Direction: "publish"}}},
		"notifications": {Setups: []spectest.SetupIntent{{Pattern: "event-stream", Direction: "consume", RoutingKey: "Order.Created"}}},
	}

	// Outbound probe: TCK consumes from broker after orders publishes.
	outbound := ProbeMessage{
		Direction:  "outbound",
		PublishVia: "orders",
		RoutingKey: "Probe.Outbound",
	}

	amqpTarget := ComputeProbeTarget("amqp", outbound, services, mapper)
	assert.Equal(t, "events.topic.exchange", amqpTarget.Exchange)
	assert.Equal(t, "Probe.Outbound", amqpTarget.RoutingKey)

	natsTarget := ComputeProbeTarget("nats", outbound, services, mapper)
	assert.Equal(t, "events", natsTarget.Stream)
	assert.Equal(t, "events.Probe.Outbound", natsTarget.Subject)

	// Inbound probe: TCK publishes to broker, notifications consumer receives.
	inbound := ProbeMessage{
		Direction:        "inbound",
		ExpectReceivedBy: "notifications",
		RoutingKey:       "Order.Created",
	}

	amqpInTarget := ComputeProbeTarget("amqp", inbound, services, mapper)
	assert.Equal(t, "events.topic.exchange", amqpInTarget.Exchange)
	assert.Equal(t, "Order.Created", amqpInTarget.RoutingKey)

	natsInTarget := ComputeProbeTarget("nats", inbound, services, mapper)
	assert.Equal(t, "events.Order.Created", natsInTarget.Subject)
}

func TestComputeProbeTargetCustomStream(t *testing.T) {
	mapper := NewNameMapper([]string{"analytics"})

	services := map[string]spectest.ServiceConfig{
		"analytics": {Setups: []spectest.SetupIntent{
			{Pattern: "custom-stream", Direction: "publish", Exchange: "audit"},
			{Pattern: "custom-stream", Direction: "consume", Exchange: "audit", RoutingKey: "Audit.Entry"},
		}},
	}

	outbound := ProbeMessage{
		Direction:  "outbound",
		PublishVia: "analytics",
		RoutingKey: "Probe.AuditOut",
	}

	amqpTarget := ComputeProbeTarget("amqp", outbound, services, mapper)
	assert.Equal(t, "audit.topic.exchange", amqpTarget.Exchange)

	natsTarget := ComputeProbeTarget("nats", outbound, services, mapper)
	assert.Equal(t, "audit", natsTarget.Stream)
	assert.Equal(t, "audit.Probe.AuditOut", natsTarget.Subject)
}
