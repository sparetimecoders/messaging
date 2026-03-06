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

package spec

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var nonAlphanumeric = regexp.MustCompile(`[^a-zA-Z0-9_]`)

type exchangeEntry struct {
	kind      ExchangeKind
	transport Transport
}

// Mermaid generates a Mermaid flowchart diagram from one or more topologies.
// The diagram shows services as rectangle nodes, exchanges as shaped nodes
// (based on exchange kind), and edges for publish/consume relationships.
// When topologies use multiple transports, exchanges are grouped into
// subgraphs by transport (e.g. AMQP, NATS).
func Mermaid(topologies []Topology) string {
	services := map[string]bool{}
	exchanges := map[string]exchangeEntry{} // key: exchange name

	type edge struct {
		from      string
		to        string
		label     string
		style     string // "-->" or "-.->"
		transport Transport
	}
	var edges []edge

	transports := map[Transport]bool{}

	for _, t := range topologies {
		if t.ServiceName == "" {
			continue
		}
		services[t.ServiceName] = true
		if t.Transport != "" {
			transports[t.Transport] = true
		}

		for _, ep := range t.Endpoints {
			if ep.ExchangeName == "" {
				continue
			}
			exchanges[ep.ExchangeName] = exchangeEntry{
				kind:      ep.ExchangeKind,
				transport: t.Transport,
			}

			arrow := "-->"
			if ep.Pattern == PatternServiceResponse {
				arrow = "-.->"
			}

			if ep.Direction == DirectionPublish {
				edges = append(edges, edge{
					from:      t.ServiceName,
					to:        ep.ExchangeName,
					label:     ep.RoutingKey,
					style:     arrow,
					transport: t.Transport,
				})
			} else if ep.Direction == DirectionConsume {
				edges = append(edges, edge{
					from:      ep.ExchangeName,
					to:        t.ServiceName,
					label:     ep.RoutingKey,
					style:     arrow,
					transport: t.Transport,
				})
			}
		}
	}

	useSubgraphs := len(transports) > 1

	// exchangeID returns the Mermaid node ID for an exchange.
	// When multiple transports are present, IDs are prefixed with the
	// transport name to avoid collisions (e.g. NATS service-request
	// exchanges use the service name, which would collide with the
	// service node ID).
	exchangeID := func(transport Transport, name string) string {
		if useSubgraphs {
			return sanitizeID(string(transport) + "_" + name)
		}
		return sanitizeID(name)
	}

	var b strings.Builder
	b.WriteString("graph LR\n")

	// Service nodes (sorted)
	sortedServices := sortedKeys(services)
	for _, svc := range sortedServices {
		b.WriteString(fmt.Sprintf("    %s[\"%s\"]\n", sanitizeID(svc), svc))
	}

	sortedExchangeNames := sortedExchangeEntryNames(exchanges)

	if useSubgraphs {
		// Group exchanges by transport into subgraphs
		sortedTransports := sortedTransportKeys(transports)
		for _, tr := range sortedTransports {
			label := strings.ToUpper(string(tr))
			b.WriteString(fmt.Sprintf("    subgraph %s\n", label))
			for _, name := range sortedExchangeNames {
				entry := exchanges[name]
				if entry.transport != tr {
					continue
				}
				writeExchangeNode(&b, exchangeID(tr, name), name, entry.kind, "        ")
			}
			b.WriteString("    end\n")
		}
	} else {
		// Single transport or no transport — flat list (backward compatible)
		for _, name := range sortedExchangeNames {
			entry := exchanges[name]
			writeExchangeNode(&b, exchangeID(entry.transport, name), name, entry.kind, "    ")
		}
	}

	// Edges (sorted for deterministic output)
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].from != edges[j].from {
			return edges[i].from < edges[j].from
		}
		if edges[i].to != edges[j].to {
			return edges[i].to < edges[j].to
		}
		return edges[i].label < edges[j].label
	})

	for _, e := range edges {
		var fromID, toID string
		if services[e.from] {
			fromID = sanitizeID(e.from)
		} else {
			fromID = exchangeID(e.transport, e.from)
		}
		if services[e.to] {
			toID = sanitizeID(e.to)
		} else {
			toID = exchangeID(e.transport, e.to)
		}
		if e.label != "" {
			b.WriteString(fmt.Sprintf("    %s %s|\"%s\"| %s\n", fromID, e.style, e.label, toID))
		} else {
			b.WriteString(fmt.Sprintf("    %s %s %s\n", fromID, e.style, toID))
		}
	}

	// Style declarations
	if len(sortedServices) > 0 {
		var ids []string
		for _, svc := range sortedServices {
			ids = append(ids, sanitizeID(svc))
		}
		b.WriteString(fmt.Sprintf("    style %s fill:#f9f,stroke:#333\n", strings.Join(ids, ",")))
	}
	if len(sortedExchangeNames) > 0 {
		var ids []string
		for _, name := range sortedExchangeNames {
			entry := exchanges[name]
			ids = append(ids, exchangeID(entry.transport, name))
		}
		b.WriteString(fmt.Sprintf("    style %s fill:#bbf,stroke:#333\n", strings.Join(ids, ",")))
	}

	return b.String()
}

func writeExchangeNode(b *strings.Builder, id, name string, kind ExchangeKind, indent string) {
	switch kind {
	case ExchangeTopic:
		b.WriteString(fmt.Sprintf("%s%s{{\"%s\"}}\n", indent, id, name))
	case ExchangeDirect:
		b.WriteString(fmt.Sprintf("%s%s[\"%s\"]\n", indent, id, name))
	case ExchangeHeaders:
		b.WriteString(fmt.Sprintf("%s%s((\"%s\"))\n", indent, id, name))
	default:
		b.WriteString(fmt.Sprintf("%s%s[\"%s\"]\n", indent, id, name))
	}
}

func sanitizeID(name string) string {
	return nonAlphanumeric.ReplaceAllString(name, "_")
}

func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedExchangeEntryNames(m map[string]exchangeEntry) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedTransportKeys(m map[Transport]bool) []Transport {
	keys := make([]Transport, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return keys
}
