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

package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
)

// uuidSuffix matches a trailing UUID (8-4-4-4-12 hex) used in transient queue names.
var uuidSuffix = regexp.MustCompile(`-[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// BrokerConfig holds connection details for the RabbitMQ Management API.
type BrokerConfig struct {
	// URL is the base URL of the management API (e.g. "http://localhost:15672").
	URL string
	// Username for HTTP basic auth (default: "guest").
	Username string
	// Password for HTTP basic auth (default: "guest").
	Password string
	// Vhost to query (default: "/").
	Vhost string
	// Client is an optional HTTP client. If nil, http.DefaultClient is used.
	Client *http.Client
}

// rabbitmq management API response types
type rmqExchange struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type rmqBinding struct {
	Source          string `json:"source"`
	Destination     string `json:"destination"`
	DestinationType string `json:"destination_type"`
	RoutingKey      string `json:"routing_key"`
}

// DiscoverTopologies connects to the RabbitMQ Management API and reconstructs
// per-service topologies from the broker's exchanges, queues, and bindings.
// It uses the spec naming conventions to infer service names from queue names.
func DiscoverTopologies(cfg BrokerConfig) ([]Topology, error) {
	if cfg.Username == "" {
		cfg.Username = "guest"
	}
	if cfg.Password == "" {
		cfg.Password = "guest"
	}
	if cfg.Vhost == "" {
		cfg.Vhost = "/"
	}
	if cfg.Client == nil {
		cfg.Client = http.DefaultClient
	}

	vhost := url.PathEscape(cfg.Vhost)

	exchanges, err := fetchJSON[[]rmqExchange](cfg, fmt.Sprintf("/api/exchanges/%s", vhost))
	if err != nil {
		return nil, fmt.Errorf("fetching exchanges: %w", err)
	}

	bindings, err := fetchJSON[[]rmqBinding](cfg, fmt.Sprintf("/api/bindings/%s", vhost))
	if err != nil {
		return nil, fmt.Errorf("fetching bindings: %w", err)
	}

	// Build exchange kind lookup, filtering out internal exchanges.
	exchangeKinds := map[string]ExchangeKind{}
	for _, ex := range exchanges {
		if ex.Name == "" || strings.HasPrefix(ex.Name, "amq.") {
			continue
		}
		switch ex.Type {
		case "topic":
			exchangeKinds[ex.Name] = ExchangeTopic
		case "direct":
			exchangeKinds[ex.Name] = ExchangeDirect
		case "headers":
			exchangeKinds[ex.Name] = ExchangeHeaders
		default:
			continue
		}
	}

	// From bindings, derive per-service endpoints.
	// Each binding is: source exchange → destination queue (with routing key).
	// We infer the service name from the queue name using naming conventions.
	type serviceEndpoints struct {
		endpoints []Endpoint
	}
	services := map[string]*serviceEndpoints{}

	ensureService := func(name string) *serviceEndpoints {
		if s, ok := services[name]; ok {
			return s
		}
		s := &serviceEndpoints{}
		services[name] = s
		return s
	}

	// Track which exchanges have publishers (bindings from queue → exchange don't
	// tell us about publishers; we need to infer from the exchange existing and
	// having consumers).
	exchangeHasConsumer := map[string]bool{}

	for _, b := range bindings {
		if b.Source == "" || b.DestinationType != "queue" {
			continue
		}
		kind, ok := exchangeKinds[b.Source]
		if !ok {
			continue
		}

		exchangeHasConsumer[b.Source] = true

		svcName, pattern := inferServiceAndPattern(b.Destination, b.Source, kind)
		if svcName == "" {
			continue
		}

		svc := ensureService(svcName)
		svc.endpoints = append(svc.endpoints, Endpoint{
			Direction:    DirectionConsume,
			Pattern:      pattern,
			ExchangeName: b.Source,
			ExchangeKind: kind,
			QueueName:    b.Destination,
			RoutingKey:   b.RoutingKey,
		})
	}

	// Build result sorted by service name.
	var names []string
	for name := range services {
		names = append(names, name)
	}
	sort.Strings(names)

	topologies := make([]Topology, 0, len(names))
	for _, name := range names {
		svc := services[name]
		topologies = append(topologies, Topology{
			Transport:   TransportAMQP,
			ServiceName: name,
			Endpoints:   svc.endpoints,
		})
	}
	return topologies, nil
}

// inferServiceAndPattern extracts the service name and messaging pattern from
// a queue name, using the spec naming conventions.
func inferServiceAndPattern(queueName, exchangeName string, kind ExchangeKind) (service string, pattern Pattern) {
	switch kind {
	case ExchangeTopic:
		// Event queue: <exchange>.queue.<service> or <exchange>.queue.<service>-<uuid>
		prefix := exchangeName + ".queue."
		if strings.HasPrefix(queueName, prefix) {
			svc := queueName[len(prefix):]
			// Strip UUID suffix for transient queues
			svc = uuidSuffix.ReplaceAllString(svc, "")
			if exchangeName == TopicExchangeName(DefaultEventExchangeName) {
				return svc, PatternEventStream
			}
			return svc, PatternCustomStream
		}
	case ExchangeDirect:
		// Request queue: <service>.direct.exchange.request.queue
		if strings.HasSuffix(exchangeName, ".direct.exchange.request") {
			svc := strings.TrimSuffix(exchangeName, ".direct.exchange.request")
			return svc, PatternServiceRequest
		}
	case ExchangeHeaders:
		// Response queue: <target>.headers.exchange.response.queue.<service>
		if strings.HasSuffix(exchangeName, ".headers.exchange.response") {
			prefix := exchangeName + ".queue."
			if strings.HasPrefix(queueName, prefix) {
				svc := queueName[len(prefix):]
				return svc, PatternServiceResponse
			}
		}
	}
	return "", ""
}

func fetchJSON[T any](cfg BrokerConfig, path string) (T, error) {
	var zero T

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	reqURL := strings.TrimRight(cfg.URL, "/") + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return zero, err
	}
	req.SetBasicAuth(cfg.Username, cfg.Password)

	resp, err := cfg.Client.Do(req)
	if err != nil {
		return zero, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return zero, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result T
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return zero, fmt.Errorf("decoding response: %w", err)
	}
	return result, nil
}
