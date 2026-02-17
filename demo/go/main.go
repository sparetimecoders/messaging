package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	goamqp "github.com/sparetimecoders/gomessaging/amqp"
	gonats "github.com/sparetimecoders/gomessaging/nats"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func main() {
	amqpURL := flag.String("amqp-url", "amqp://guest:guest@localhost:5672/", "AMQP broker URL")
	natsURL := flag.String("nats-url", "nats://localhost:4222", "NATS server URL")
	httpPort := flag.Int("http-port", 3000, "HTTP server port")
	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// OTel tracing setup
	tp, err := initTracer(ctx)
	if err != nil {
		slog.Error("failed to init tracer", "error", err)
		os.Exit(1)
	}
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		if err := tp.Shutdown(shutdownCtx); err != nil {
			slog.Error("failed to shut down tracer", "error", err)
		}
	}()
	prop := propagation.TraceContext{}

	broadcaster := newSSEBroadcaster()

	// Create publishers
	amqpPub := goamqp.NewPublisher()
	natsPub := gonats.NewPublisher()

	// Create AMQP connection
	amqpConn, err := goamqp.NewFromURL("go-demo", *amqpURL)
	if err != nil {
		slog.Error("failed to create AMQP connection", "error", err)
		os.Exit(1)
	}

	amqpCloseCh := make(chan error, 1)
	amqpOpts := append([]goamqp.Setup{
		goamqp.WithLogger(slog.Default()),
		goamqp.WithTracing(tp),
		goamqp.WithPropagator(prop),
		goamqp.CloseListener(amqpCloseCh),
	}, amqpSetups(amqpPub, broadcaster.broadcast)...)

	if err := amqpConn.Start(ctx, amqpOpts...); err != nil {
		slog.Error("failed to start AMQP connection", "error", err)
		os.Exit(1)
	}
	slog.Info("AMQP connection started")

	// Fail-fast: exit process on unexpected AMQP disconnect
	go func() {
		if err := <-amqpCloseCh; err != nil {
			slog.Error("AMQP connection lost, shutting down", "error", err)
			os.Exit(1)
		}
	}()

	// Create NATS connection
	natsConn, err := gonats.NewConnection("go-demo", *natsURL)
	if err != nil {
		slog.Error("failed to create NATS connection", "error", err)
		os.Exit(1)
	}

	natsOpts := append([]gonats.Setup{
		gonats.WithLogger(slog.Default()),
		gonats.WithTracing(tp),
		gonats.WithPropagator(prop),
	}, natsSetups(natsPub, broadcaster.broadcast)...)

	if err := natsConn.Start(ctx, natsOpts...); err != nil {
		slog.Error("failed to start NATS connection", "error", err)
		os.Exit(1)
	}
	slog.Info("NATS connection started")

	// Start HTTP server
	ws := newWebServer(amqpPub, natsPub, amqpConn, natsConn, broadcaster)
	srv := startHTTPServer(ctx, *httpPort, ws.handler())

	// Wait for signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	slog.Info("received signal, shutting down", "signal", sig)

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("HTTP server shutdown error", "error", err)
	}
	if err := natsConn.Close(); err != nil {
		slog.Error("NATS close error", "error", err)
	}
	if err := amqpConn.Close(); err != nil {
		slog.Error("AMQP close error", "error", err)
	}

	slog.Info("go-demo stopped")
}

func initTracer(ctx context.Context) (*sdktrace.TracerProvider, error) {
	// OTLP exporter → Jaeger
	otlpExporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpointURL("http://localhost:4318/v1/traces"),
	)
	if err != nil {
		return nil, err
	}

	// Stdout exporter for debugging
	stdoutExporter, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
	if err != nil {
		return nil, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(otlpExporter),
		sdktrace.WithBatcher(stdoutExporter),
	)
	return tp, nil
}
