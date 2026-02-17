// Import tracing first to ensure OTel SDK is initialized before any other imports
import "./tracing.js";

import { setupAmqp, setupNats } from "./handlers.js";
import { createApp, SSEBroadcaster } from "./web.js";

const amqpUrl = process.env.AMQP_URL ?? "amqp://guest:guest@localhost:5672/";
const natsUrl = process.env.NATS_URL ?? "nats://localhost:4222";
const httpPort = parseInt(process.env.HTTP_PORT ?? "3001", 10);

async function main(): Promise<void> {
  const broadcaster = new SSEBroadcaster();

  // Setup AMQP
  console.log(`Connecting to AMQP at ${amqpUrl}...`);
  const amqp = setupAmqp(amqpUrl, broadcaster.broadcast.bind(broadcaster));
  await amqp.conn.start();
  console.log("AMQP connection started");

  // Setup NATS
  console.log(`Connecting to NATS at ${natsUrl}...`);
  const nats = setupNats(natsUrl, broadcaster.broadcast.bind(broadcaster));
  await nats.conn.start();
  console.log("NATS connection started");

  // Start HTTP server
  const app = createApp({
    amqpConn: amqp.conn,
    natsConn: nats.conn,
    publishAmqp: (routingKey, payload) =>
      amqp.publisher.publish(routingKey, payload),
    publishNats: (routingKey, payload) =>
      nats.publisher.publish(routingKey, payload),
    broadcaster,
  });

  const server = app.listen(httpPort, () => {
    console.log(`node-demo HTTP server listening on http://localhost:${httpPort}`);
  });

  // Graceful shutdown
  const shutdown = async () => {
    console.log("Shutting down...");
    server.close();
    await nats.conn.close();
    await amqp.conn.close();
    console.log("node-demo stopped");
    process.exit(0);
  };

  process.on("SIGINT", shutdown);
  process.on("SIGTERM", shutdown);
}

main().catch((err) => {
  console.error("Fatal error:", err);
  process.exit(1);
});
