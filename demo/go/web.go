package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/sparetimecoders/go-messaging-amqp"
	"github.com/sparetimecoders/go-messaging-nats"
	"github.com/sparetimecoders/messaging"
)

// SSEEvent is an event sent to the browser via Server-Sent Events.
type SSEEvent struct {
	Type       string `json:"type"`
	Transport  string `json:"transport"`
	Source     string `json:"source"`
	RoutingKey string `json:"routingKey"`
	Payload    any    `json:"payload"`
	TraceID    string `json:"traceId,omitempty"`
}

// SSEBroadcaster manages SSE client connections.
type SSEBroadcaster struct {
	mu      sync.RWMutex
	clients map[chan SSEEvent]struct{}
}

func newSSEBroadcaster() *SSEBroadcaster {
	return &SSEBroadcaster{clients: make(map[chan SSEEvent]struct{})}
}

func (b *SSEBroadcaster) subscribe() chan SSEEvent {
	ch := make(chan SSEEvent, 64)
	b.mu.Lock()
	b.clients[ch] = struct{}{}
	b.mu.Unlock()
	return ch
}

func (b *SSEBroadcaster) unsubscribe(ch chan SSEEvent) {
	b.mu.Lock()
	delete(b.clients, ch)
	b.mu.Unlock()
	close(ch)
}

func (b *SSEBroadcaster) broadcast(event SSEEvent) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.clients {
		select {
		case ch <- event:
		default:
			// Drop if client is slow
		}
	}
}

// PublishRequest is the JSON body for POST /api/publish.
type PublishRequest struct {
	Transport  string         `json:"transport"`
	RoutingKey string         `json:"routingKey"`
	Payload    map[string]any `json:"payload"`
}

// RequestRequest is the JSON body for POST /api/request.
type RequestRequest struct {
	Transport  string         `json:"transport"`
	RoutingKey string         `json:"routingKey"`
	Payload    map[string]any `json:"payload"`
}

type webServer struct {
	amqpPub     *amqp.Publisher
	natsPub     *nats.Publisher
	amqpConn    *amqp.Connection
	natsConn    *nats.Connection
	broadcaster *SSEBroadcaster
}

func newWebServer(amqpPub *amqp.Publisher, natsPub *nats.Publisher, amqpConn *amqp.Connection, natsConn *nats.Connection, broadcaster *SSEBroadcaster) *webServer {
	return &webServer{
		amqpPub:     amqpPub,
		natsPub:     natsPub,
		amqpConn:    amqpConn,
		natsConn:    natsConn,
		broadcaster: broadcaster,
	}
}

func (ws *webServer) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", ws.handleIndex)
	mux.HandleFunc("GET /api/events", ws.handleSSE)
	mux.HandleFunc("POST /api/publish", ws.handlePublish)
	mux.HandleFunc("POST /api/request", ws.handleRequest)
	mux.HandleFunc("GET /api/topology", ws.handleTopology)
	mux.HandleFunc("GET /api/topology/mermaid", ws.handleMermaid)
	return corsMiddleware(mux)
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (ws *webServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, "ui/index.html")
}

func (ws *webServer) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := ws.broadcaster.subscribe()
	defer ws.broadcaster.unsubscribe(ch)

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case event := <-ch:
			data, err := json.Marshal(event)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

func (ws *webServer) handlePublish(w http.ResponseWriter, r *http.Request) {
	var req PublishRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Add source and transport to payload
	req.Payload["source"] = "go-demo"
	req.Payload["transport"] = req.Transport

	ctx := r.Context()
	var err error
	switch req.Transport {
	case "amqp":
		err = ws.amqpPub.Publish(ctx, req.RoutingKey, req.Payload)
	case "nats":
		err = ws.natsPub.Publish(ctx, req.RoutingKey, req.Payload)
	default:
		http.Error(w, "unknown transport", http.StatusBadRequest)
		return
	}

	if err != nil {
		slog.Error("publish failed", "error", err, "transport", req.Transport, "routingKey", req.RoutingKey)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	ws.broadcaster.broadcast(SSEEvent{
		Type:       "sent",
		Transport:  req.Transport,
		Source:     "go-demo",
		RoutingKey: req.RoutingKey,
		Payload:    req.Payload,
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (ws *webServer) handleRequest(w http.ResponseWriter, r *http.Request) {
	var req RequestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// For request-reply, we need a service publisher, which is more involved.
	// For now, return a simulated response.
	http.Error(w, "request-reply via HTTP not yet implemented for go-demo", http.StatusNotImplemented)
}

func (ws *webServer) handleTopology(w http.ResponseWriter, r *http.Request) {
	topologies := []spec.Topology{
		ws.amqpConn.Topology(),
		ws.natsConn.Topology(),
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(topologies)
}

func (ws *webServer) handleMermaid(w http.ResponseWriter, r *http.Request) {
	topologies := []spec.Topology{
		ws.amqpConn.Topology(),
		ws.natsConn.Topology(),
	}
	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprint(w, spec.Mermaid(topologies))
}

func startHTTPServer(ctx context.Context, port int, handler http.Handler) *http.Server {
	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		slog.Info("HTTP server starting", "port", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server error", "error", err)
			os.Exit(1)
		}
	}()

	return srv
}
