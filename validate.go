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
	"errors"
	"fmt"
	"strings"
)

// Validate checks a single service's topology for internal consistency.
// It verifies structural rules (common to all transports) and transport-specific
// naming conventions when Transport is set.
func Validate(t Topology) error {
	if t.ServiceName == "" {
		return errors.New("service name must not be empty")
	}

	var errs []error
	for i, ep := range t.Endpoints {
		if ep.ExchangeName == "" {
			errs = append(errs, fmt.Errorf("endpoint[%d]: exchange name must not be empty", i))
		}
		if ep.Direction == DirectionConsume && ep.QueueName == "" && !ep.Ephemeral {
			errs = append(errs, fmt.Errorf("endpoint[%d]: consume endpoint must have a queue name", i))
		}
		if ep.RoutingKey == "" && ep.ExchangeKind != ExchangeHeaders {
			errs = append(errs, fmt.Errorf("endpoint[%d]: routing key must not be empty for %s exchange", i, ep.ExchangeKind))
		}
		if strings.Contains(ep.RoutingKey, ">") {
			errs = append(errs, fmt.Errorf("endpoint[%d]: routing key must not contain '>' (use '#' for multi-level wildcard)", i))
		}
		if err := validateExchangeNameForTransport(t.Transport, ep); err != nil {
			errs = append(errs, fmt.Errorf("endpoint[%d]: %w", i, err))
		}
	}
	return errors.Join(errs...)
}

func validateExchangeNameForTransport(transport Transport, ep Endpoint) error {
	switch transport {
	case TransportAMQP:
		return validateAMQPExchangeName(ep)
	case TransportNATS:
		return validateNATSExchangeName(ep)
	default:
		// Unknown or empty transport: skip naming checks.
		return nil
	}
}

func validateAMQPExchangeName(ep Endpoint) error {
	name := ep.ExchangeName
	switch ep.ExchangeKind {
	case ExchangeTopic:
		if !strings.HasSuffix(name, ".topic.exchange") {
			return fmt.Errorf("topic exchange %q must end with .topic.exchange", name)
		}
	case ExchangeDirect:
		if !strings.HasSuffix(name, ".direct.exchange.request") {
			return fmt.Errorf("direct exchange %q must end with .direct.exchange.request", name)
		}
	case ExchangeHeaders:
		if !strings.HasSuffix(name, ".headers.exchange.response") {
			return fmt.Errorf("headers exchange %q must end with .headers.exchange.response", name)
		}
	}
	return nil
}

func validateNATSExchangeName(ep Endpoint) error {
	name := ep.ExchangeName
	for _, suffix := range []string{".topic.exchange", ".direct.exchange.request", ".headers.exchange.response"} {
		if strings.HasSuffix(name, suffix) {
			return fmt.Errorf("NATS exchange %q must not use AMQP suffix %q", name, suffix)
		}
	}
	if strings.ContainsAny(name, " \t\n\r") {
		return fmt.Errorf("NATS exchange %q must not contain whitespace", name)
	}
	return nil
}

// ValidateTopologies checks cross-service consistency across multiple topologies.
// It verifies that consumer routing keys have matching publishers.
// Topologies are grouped by Transport before cross-validation; different
// transports cannot communicate with each other.
func ValidateTopologies(topologies []Topology) error {
	var errs []error

	for _, t := range topologies {
		if err := Validate(t); err != nil {
			errs = append(errs, fmt.Errorf("service %q: %w", t.ServiceName, err))
		}
	}

	// Group topologies by transport for cross-validation.
	groups := map[Transport][]Topology{}
	for _, t := range topologies {
		groups[t.Transport] = append(groups[t.Transport], t)
	}

	for _, group := range groups {
		if err := crossValidateGroup(group); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

func crossValidateGroup(topologies []Topology) error {
	var errs []error

	// Build a set of all published routing keys per exchange.
	type exchangeKey struct {
		exchange   string
		routingKey string
	}
	published := make(map[exchangeKey]string) // value = publishing service name
	for _, t := range topologies {
		for _, ep := range t.Endpoints {
			if ep.Direction == DirectionPublish && ep.RoutingKey != "" {
				published[exchangeKey{ep.ExchangeName, ep.RoutingKey}] = t.ServiceName
			}
		}
	}

	// Check that each consumer has a matching publisher (exact match only;
	// wildcard consumers are expected to match a superset of publishers).
	for _, t := range topologies {
		for _, ep := range t.Endpoints {
			if ep.Direction != DirectionConsume {
				continue
			}
			if ep.RoutingKey == "" {
				continue
			}
			// Skip wildcard consumers — they intentionally match multiple keys.
			if strings.ContainsAny(ep.RoutingKey, "#*>") {
				continue
			}
			ek := exchangeKey{ep.ExchangeName, ep.RoutingKey}
			if _, ok := published[ek]; !ok {
				errs = append(errs, fmt.Errorf(
					"service %q consumes %q on exchange %q but no service publishes it",
					t.ServiceName, ep.RoutingKey, ep.ExchangeName))
			}
		}
	}

	return errors.Join(errs...)
}
