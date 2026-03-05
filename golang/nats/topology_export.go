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

package nats

import (
	"io"
	"log/slog"

	"github.com/sparetimecoders/messaging/specification/spec"
)

// CollectTopology runs the given setup functions against a no-op connection to
// collect the topology that would be declared, without connecting to a broker.
func CollectTopology(serviceName string, setups ...Setup) (spec.Topology, error) {
	c := &Connection{
		serviceName: serviceName,
		logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		topology:    spec.Topology{Transport: spec.TransportNATS, ServiceName: serviceName},
		collectMode: true,
	}

	for _, f := range setups {
		if err := f(c); err != nil {
			return spec.Topology{}, err
		}
	}

	return c.topology, nil
}
