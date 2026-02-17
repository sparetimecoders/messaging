import express from "express";
import { fileURLToPath } from "node:url";
import { dirname, join } from "node:path";
import type { Connection as AmqpConnection } from "@gomessaging/amqp";
import type { Connection as NatsConnection } from "@gomessaging/nats";
import { mermaid } from "@gomessaging/spec";

const __dirname = dirname(fileURLToPath(import.meta.url));

export interface SSEEvent {
  type: string;
  transport: string;
  source: string;
  routingKey: string;
  payload: unknown;
  traceId?: string;
}

export interface WebServerOptions {
  amqpConn: AmqpConnection;
  natsConn: NatsConnection;
  publishAmqp: (routingKey: string, payload: unknown) => Promise<void>;
  publishNats: (routingKey: string, payload: unknown) => Promise<void>;
  broadcaster: SSEBroadcaster;
}

export class SSEBroadcaster {
  private clients: Set<express.Response> = new Set();

  subscribe(res: express.Response): void {
    this.clients.add(res);
  }

  unsubscribe(res: express.Response): void {
    this.clients.delete(res);
  }

  broadcast(event: SSEEvent): void {
    const data = JSON.stringify(event);
    for (const client of this.clients) {
      client.write(`data: ${data}\n\n`);
    }
  }
}

export function createApp(opts: WebServerOptions): express.Express {
  const app = express();
  app.use(express.json());

  // CORS
  app.use((_req, res, next) => {
    res.header("Access-Control-Allow-Origin", "*");
    res.header("Access-Control-Allow-Methods", "GET, POST, OPTIONS");
    res.header("Access-Control-Allow-Headers", "Content-Type");
    next();
  });

  // Serve static UI
  app.get("/", (_req, res) => {
    res.sendFile(join(__dirname, "..", "ui", "index.html"));
  });

  // SSE endpoint
  app.get("/api/events", (req, res) => {
    res.setHeader("Content-Type", "text/event-stream");
    res.setHeader("Cache-Control", "no-cache");
    res.setHeader("Connection", "keep-alive");
    res.flushHeaders();

    opts.broadcaster.subscribe(res);
    req.on("close", () => {
      opts.broadcaster.unsubscribe(res);
    });
  });

  // Publish message
  app.post("/api/publish", async (req, res) => {
    const { transport, routingKey, payload } = req.body;
    payload.source = "node-demo";
    payload.transport = transport;

    try {
      if (transport === "amqp") {
        await opts.publishAmqp(routingKey, payload);
      } else if (transport === "nats") {
        await opts.publishNats(routingKey, payload);
      } else {
        res.status(400).json({ error: "unknown transport" });
        return;
      }

      opts.broadcaster.broadcast({
        type: "sent",
        transport,
        source: "node-demo",
        routingKey,
        payload,
      });

      res.json({ status: "ok" });
    } catch (err) {
      console.error("publish failed:", err);
      res.status(500).json({ error: String(err) });
    }
  });

  // Request-reply (placeholder)
  app.post("/api/request", async (_req, res) => {
    res
      .status(501)
      .json({ error: "request-reply via HTTP not yet implemented for node-demo" });
  });

  // Topology
  app.get("/api/topology", (_req, res) => {
    res.json([opts.amqpConn.topology(), opts.natsConn.topology()]);
  });

  // Mermaid diagram
  app.get("/api/topology/mermaid", (_req, res) => {
    const diagram = mermaid([
      opts.amqpConn.topology(),
      opts.natsConn.topology(),
    ]);
    res.type("text/plain").send(diagram);
  });

  return app;
}
