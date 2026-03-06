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
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	amqplib "github.com/rabbitmq/amqp091-go"
	"github.com/sparetimecoders/messaging"
	"github.com/sparetimecoders/messaging/spectest"
)

// amqpBrokerClient implements BrokerClient for RabbitMQ.
type amqpBrokerClient struct {
	url           string
	managementURL string
}

func (c *amqpBrokerClient) QueryState(t spectest.T) spectest.BrokerState {
	t.Helper()

	exchanges := queryManagementAPI[[]managementExchange](t, c.managementURL, "/api/exchanges/%2f")
	queues := queryManagementAPI[[]managementQueue](t, c.managementURL, "/api/queues/%2f")
	bindings := queryManagementAPI[[]managementBinding](t, c.managementURL, "/api/bindings/%2f")

	var filteredExchanges []spectest.AMQPExchange
	for _, e := range exchanges {
		if e.Name == "" || strings.HasPrefix(e.Name, "amq.") {
			continue
		}
		filteredExchanges = append(filteredExchanges, spectest.AMQPExchange{
			Name:       e.Name,
			Type:       e.Type,
			Durable:    e.Durable,
			AutoDelete: e.AutoDelete,
		})
	}

	var filteredQueues []spectest.AMQPQueue
	for _, q := range queues {
		filteredQueues = append(filteredQueues, spectest.AMQPQueue{
			Name:       q.Name,
			Durable:    q.Durable,
			AutoDelete: q.AutoDelete,
			Arguments: spectest.QueueArguments{
				XQueueType: stringFromTable(q.Arguments, "x-queue-type"),
				XExpires:   intFromTable(q.Arguments, "x-expires"),
			},
		})
	}

	var filteredBindings []spectest.AMQPBinding
	for _, b := range bindings {
		if b.Source == "" {
			continue
		}
		filteredBindings = append(filteredBindings, spectest.AMQPBinding{
			Source:      b.Source,
			Destination: b.Destination,
			RoutingKey:  b.RoutingKey,
		})
	}

	return spectest.BrokerState{
		AMQP: spectest.AMQPBrokerState{
			Exchanges: filteredExchanges,
			Queues:    filteredQueues,
			Bindings:  filteredBindings,
		},
	}
}

func (c *amqpBrokerClient) PublishRaw(t spectest.T, target spectest.ProbeTarget, payload json.RawMessage, headers map[string]string) error {
	t.Helper()
	conn, err := amqplib.Dial(c.url)
	spectest.RequireNoError(t, err)
	defer conn.Close()

	ch, err := conn.Channel()
	spectest.RequireNoError(t, err)
	defer ch.Close()

	amqpHeaders := amqplib.Table{}
	for attr, val := range headers {
		amqpHeaders[spec.AMQPCEHeaderKey(attr)] = val
	}
	amqpHeaders[spec.AMQPCEHeaderKey(spec.CEAttrID)] = uuid.New().String()
	amqpHeaders[spec.AMQPCEHeaderKey(spec.CEAttrTime)] = time.Now().UTC().Format(time.RFC3339)

	return ch.PublishWithContext(context.Background(), target.Exchange, target.RoutingKey, false, false, amqplib.Publishing{
		Body:         payload,
		ContentType:  "application/json",
		DeliveryMode: 2,
		Headers:      amqpHeaders,
	})
}

func (c *amqpBrokerClient) CreateProbeConsumer(t spectest.T, target spectest.ProbeTarget) *spectest.ProbeConsumer {
	t.Helper()
	conn, err := amqplib.Dial(c.url)
	spectest.RequireNoError(t, err)

	ch, err := conn.Channel()
	spectest.RequireNoError(t, err)

	q, err := ch.QueueDeclare("", false, true, true, false, nil)
	spectest.RequireNoError(t, err)

	err = ch.QueueBind(q.Name, target.RoutingKey, target.Exchange, false, nil)
	spectest.RequireNoError(t, err)

	deliveries, err := ch.Consume(q.Name, "", true, true, false, false, nil)
	spectest.RequireNoError(t, err)

	return &spectest.ProbeConsumer{
		Receive: func(timeout time.Duration) *spectest.RawMessage {
			select {
			case d := <-deliveries:
				hdrs := make(map[string]string)
				normalized := spec.NormalizeCEHeaders(amqpTableToHeaders(d.Headers))
				for k, v := range normalized {
					if strings.HasPrefix(k, "ce-") {
						if s, ok := v.(string); ok {
							hdrs[strings.TrimPrefix(k, "ce-")] = s
						}
					}
				}
				return &spectest.RawMessage{
					Payload: d.Body,
					Headers: hdrs,
				}
			case <-time.After(timeout):
				return nil
			}
		},
		Close: func() {
			_ = ch.Close()
			_ = conn.Close()
		},
	}
}

func (c *amqpBrokerClient) Cleanup(t spectest.T) {
	t.Helper()

	// Delete all non-default queues.
	queues := queryManagementAPI[[]managementQueue](t, c.managementURL, "/api/queues/%2f")
	for _, q := range queues {
		deleteResource(t, c.managementURL, fmt.Sprintf("/api/queues/%%2f/%s", q.Name))
	}

	// Delete all non-default exchanges.
	exchanges := queryManagementAPI[[]managementExchange](t, c.managementURL, "/api/exchanges/%2f")
	for _, e := range exchanges {
		if e.Name == "" || strings.HasPrefix(e.Name, "amq.") {
			continue
		}
		deleteResource(t, c.managementURL, fmt.Sprintf("/api/exchanges/%%2f/%s", e.Name))
	}
}

func amqpTableToHeaders(table amqplib.Table) spec.Headers {
	headers := make(spec.Headers, len(table))
	for k, v := range table {
		headers[k] = v
	}
	return headers
}

// RabbitMQ Management API types.

type managementExchange struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Durable    bool   `json:"durable"`
	AutoDelete bool   `json:"auto_delete"`
}

type managementQueue struct {
	Name       string         `json:"name"`
	Durable    bool           `json:"durable"`
	AutoDelete bool           `json:"auto_delete"`
	Arguments  map[string]any `json:"arguments"`
}

type managementBinding struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
	RoutingKey  string `json:"routing_key"`
}

func queryManagementAPI[R any](t spectest.T, baseURL, path string) R {
	t.Helper()
	resp, err := http.Get(baseURL + path)
	spectest.RequireNoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	spectest.RequireNoError(t, err)
	spectest.RequireEqual(t, http.StatusOK, resp.StatusCode, "management API %s: %s", path, string(body))

	var result R
	spectest.RequireNoError(t, json.Unmarshal(body, &result))
	return result
}

func stringFromTable(args map[string]any, key string) string {
	if v, ok := args[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func intFromTable(args map[string]any, key string) int {
	if v, ok := args[key]; ok {
		switch n := v.(type) {
		case float64:
			return int(n)
		case int:
			return n
		}
	}
	return 0
}

func deleteResource(t spectest.T, baseURL, path string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodDelete, baseURL+path, nil)
	spectest.RequireNoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	spectest.RequireNoError(t, err)
	resp.Body.Close()
}
