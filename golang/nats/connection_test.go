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
	"bytes"
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/sparetimecoders/gomessaging/spec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewConnection(t *testing.T) {
	t.Run("valid parameters", func(t *testing.T) {
		conn, err := NewConnection("test-service", "nats://localhost:4222")
		require.NoError(t, err)
		assert.NotNil(t, conn)
		assert.Equal(t, "test-service", conn.serviceName)
	})

	t.Run("empty service name", func(t *testing.T) {
		_, err := NewConnection("", "nats://localhost:4222")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "service name must not be empty")
	})

	t.Run("empty URL", func(t *testing.T) {
		_, err := NewConnection("svc", "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "NATS URL must not be empty")
	})
}

func TestConnectionStartStop(t *testing.T) {
	s := startTestServer(t)
	url := serverURL(s)

	conn, err := NewConnection("test-service", url)
	require.NoError(t, err)

	err = conn.Start(context.Background(), WithLogger(slog.Default()))
	require.NoError(t, err)

	assert.Equal(t, "test-service", conn.Topology().ServiceName)

	err = conn.Close()
	require.NoError(t, err)
}

func TestConnectionAlreadyStarted(t *testing.T) {
	s := startTestServer(t)
	url := serverURL(s)

	conn, err := NewConnection("test-service", url)
	require.NoError(t, err)

	err = conn.Start(context.Background())
	require.NoError(t, err)
	defer conn.Close()

	err = conn.Start(context.Background())
	assert.ErrorIs(t, err, ErrAlreadyStarted)
}

func TestConnectionFailedConnect(t *testing.T) {
	conn, err := NewConnection("test-service", "nats://localhost:1")
	require.NoError(t, err)

	err = conn.Start(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to connect to NATS")
}

func TestConnectionCloseNotStarted(t *testing.T) {
	conn, err := NewConnection("test-service", "nats://localhost:4222")
	require.NoError(t, err)

	err = conn.Close()
	assert.NoError(t, err)
}

func TestEnsureStream_AppliesDefaults(t *testing.T) {
	s := startTestServer(t)
	url := serverURL(s)

	conn, err := NewConnection("retention-svc", url)
	require.NoError(t, err)

	pub := NewPublisher()
	err = conn.Start(context.Background(),
		WithStreamDefaults(StreamConfig{
			MaxAge:   24 * time.Hour,
			MaxBytes: 1024 * 1024, // 1 MiB
			MaxMsgs:  1000,
		}),
		EventStreamPublisher(pub),
	)
	require.NoError(t, err)
	defer conn.Close()

	// Verify stream config via JetStream API.
	// NOTE: conn.js is an internal field -- this works because tests are in
	// package nats (not nats_test). Do not extract to an external test package.
	info, err := conn.js.Stream(context.Background(), streamName("events"))
	require.NoError(t, err)
	cfg := info.CachedInfo().Config
	assert.Equal(t, 24*time.Hour, cfg.MaxAge)
	assert.Equal(t, int64(1024*1024), cfg.MaxBytes)
	assert.Equal(t, int64(1000), cfg.MaxMsgs)
}

func TestEnsureStream_PerStreamOverride(t *testing.T) {
	s := startTestServer(t)
	url := serverURL(s)

	conn, err := NewConnection("override-svc", url)
	require.NoError(t, err)

	eventPub := NewPublisher()
	auditPub := NewPublisher()
	err = conn.Start(context.Background(),
		WithStreamDefaults(StreamConfig{MaxAge: 24 * time.Hour}),
		WithStreamConfig("audit", StreamConfig{MaxAge: 365 * 24 * time.Hour}),
		EventStreamPublisher(eventPub),
		StreamPublisher("audit", auditPub),
	)
	require.NoError(t, err)
	defer conn.Close()

	// Events stream should use defaults
	eventsInfo, err := conn.js.Stream(context.Background(), streamName("events"))
	require.NoError(t, err)
	assert.Equal(t, 24*time.Hour, eventsInfo.CachedInfo().Config.MaxAge)

	// Audit stream should use per-stream override
	auditInfo, err := conn.js.Stream(context.Background(), streamName("audit"))
	require.NoError(t, err)
	assert.Equal(t, 365*24*time.Hour, auditInfo.CachedInfo().Config.MaxAge)
	// MaxBytes/MaxMsgs should be unlimited since override replaces entirely.
	// NATS represents unlimited as -1 in stream info.
	assert.Equal(t, int64(-1), auditInfo.CachedInfo().Config.MaxBytes)
}

func TestEnsureStream_DefaultsApplyViaConsumerPath(t *testing.T) {
	// Verifies that stream defaults also apply when the stream is created
	// via the consumer path (startPendingJSConsumers -> ensureStream), not
	// just via StreamPublisher. Both paths go through ensureStream with
	// streamName()-transformed names, so the streamConfigs key must match.
	s := startTestServer(t)
	url := serverURL(s)

	handler := func(ctx context.Context, event spec.ConsumableEvent[testMessage]) error {
		return nil
	}

	conn, err := NewConnection("consumer-retention-svc", url)
	require.NoError(t, err)

	err = conn.Start(context.Background(),
		WithStreamDefaults(StreamConfig{
			MaxAge: 48 * time.Hour,
		}),
		EventStreamConsumer("Order.Created", handler),
	)
	require.NoError(t, err)
	defer conn.Close()

	// NOTE: conn.js is an internal field -- tests are in package nats.
	info, err := conn.js.Stream(context.Background(), streamName("events"))
	require.NoError(t, err)
	assert.Equal(t, 48*time.Hour, info.CachedInfo().Config.MaxAge)
}

func TestEnsureStream_NoLimitsWarning(t *testing.T) {
	// This test verifies that a warning is logged when no limits are set.
	// Use a custom slog handler to capture log output.
	s := startTestServer(t)
	url := serverURL(s)

	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})
	logger := slog.New(handler)

	conn, err := NewConnection("warn-svc", url)
	require.NoError(t, err)

	pub := NewPublisher()
	err = conn.Start(context.Background(),
		WithLogger(logger),
		EventStreamPublisher(pub),
	)
	require.NoError(t, err)
	defer conn.Close()

	assert.Contains(t, buf.String(), "no retention limits configured")
}
