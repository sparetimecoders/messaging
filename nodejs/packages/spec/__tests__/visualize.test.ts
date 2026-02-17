import { describe, expect, it } from "vitest";
import { mermaid } from "../src/visualize.js";
import type { Topology } from "../src/types.js";

describe("mermaid", () => {
  it("empty topologies returns graph header only", () => {
    expect(mermaid([])).toBe("graph LR\n");
  });

  it("single publisher", () => {
    const topologies: Topology[] = [
      {
        serviceName: "orders",
        endpoints: [
          {
            direction: "publish",
            pattern: "event-stream",
            exchangeName: "events.topic.exchange",
            exchangeKind: "topic",
            routingKey: "Order.Created",
          },
        ],
      },
    ];

    expect(mermaid(topologies)).toBe(
      [
        "graph LR",
        '    orders["orders"]',
        '    events_topic_exchange{{"events.topic.exchange"}}',
        '    orders -->|"Order.Created"| events_topic_exchange',
        "    style orders fill:#f9f,stroke:#333",
        "    style events_topic_exchange fill:#bbf,stroke:#333",
        "",
      ].join("\n"),
    );
  });

  it("single consumer", () => {
    const topologies: Topology[] = [
      {
        serviceName: "notifications",
        endpoints: [
          {
            direction: "consume",
            pattern: "event-stream",
            exchangeName: "events.topic.exchange",
            exchangeKind: "topic",
            queueName: "events.topic.exchange.queue.notifications",
            routingKey: "Order.Created",
          },
        ],
      },
    ];

    expect(mermaid(topologies)).toBe(
      [
        "graph LR",
        '    notifications["notifications"]',
        '    events_topic_exchange{{"events.topic.exchange"}}',
        '    events_topic_exchange -->|"Order.Created"| notifications',
        "    style notifications fill:#f9f,stroke:#333",
        "    style events_topic_exchange fill:#bbf,stroke:#333",
        "",
      ].join("\n"),
    );
  });

  it("publisher and consumer across services", () => {
    const topologies: Topology[] = [
      {
        serviceName: "orders",
        endpoints: [
          {
            direction: "publish",
            pattern: "event-stream",
            exchangeName: "events.topic.exchange",
            exchangeKind: "topic",
            routingKey: "Order.Created",
          },
        ],
      },
      {
        serviceName: "notifications",
        endpoints: [
          {
            direction: "consume",
            pattern: "event-stream",
            exchangeName: "events.topic.exchange",
            exchangeKind: "topic",
            queueName: "events.topic.exchange.queue.notifications",
            routingKey: "Order.Created",
          },
        ],
      },
    ];

    expect(mermaid(topologies)).toBe(
      [
        "graph LR",
        '    notifications["notifications"]',
        '    orders["orders"]',
        '    events_topic_exchange{{"events.topic.exchange"}}',
        '    events_topic_exchange -->|"Order.Created"| notifications',
        '    orders -->|"Order.Created"| events_topic_exchange',
        "    style notifications,orders fill:#f9f,stroke:#333",
        "    style events_topic_exchange fill:#bbf,stroke:#333",
        "",
      ].join("\n"),
    );
  });

  it("request-response pattern uses dashed arrows", () => {
    const topologies: Topology[] = [
      {
        serviceName: "email-svc",
        endpoints: [
          {
            direction: "consume",
            pattern: "service-request",
            exchangeName: "email-svc.direct.exchange.request",
            exchangeKind: "direct",
            queueName: "email-svc.direct.exchange.request.queue",
            routingKey: "email.send",
          },
          {
            direction: "publish",
            pattern: "service-response",
            exchangeName: "email-svc.headers.exchange.response",
            exchangeKind: "headers",
          },
        ],
      },
      {
        serviceName: "web-app",
        endpoints: [
          {
            direction: "publish",
            pattern: "service-request",
            exchangeName: "email-svc.direct.exchange.request",
            exchangeKind: "direct",
            routingKey: "email.send",
          },
          {
            direction: "consume",
            pattern: "service-response",
            exchangeName: "email-svc.headers.exchange.response",
            exchangeKind: "headers",
            queueName: "email-svc.headers.exchange.response.queue.web-app",
          },
        ],
      },
    ];

    expect(mermaid(topologies)).toBe(
      [
        "graph LR",
        '    email_svc["email-svc"]',
        '    web_app["web-app"]',
        '    email_svc_direct_exchange_request["email-svc.direct.exchange.request"]',
        '    email_svc_headers_exchange_response(("email-svc.headers.exchange.response"))',
        "    email_svc -.-> email_svc_headers_exchange_response",
        '    email_svc_direct_exchange_request -->|"email.send"| email_svc',
        "    email_svc_headers_exchange_response -.-> web_app",
        '    web_app -->|"email.send"| email_svc_direct_exchange_request',
        "    style email_svc,web_app fill:#f9f,stroke:#333",
        "    style email_svc_direct_exchange_request,email_svc_headers_exchange_response fill:#bbf,stroke:#333",
        "",
      ].join("\n"),
    );
  });

  it("multiple exchanges", () => {
    const topologies: Topology[] = [
      {
        serviceName: "analytics",
        endpoints: [
          {
            direction: "publish",
            pattern: "custom-stream",
            exchangeName: "audit.topic.exchange",
            exchangeKind: "topic",
            routingKey: "Audit.Entry",
          },
          {
            direction: "consume",
            pattern: "event-stream",
            exchangeName: "events.topic.exchange",
            exchangeKind: "topic",
            queueName: "events.topic.exchange.queue.analytics",
            routingKey: "Order.Created",
          },
        ],
      },
    ];

    expect(mermaid(topologies)).toBe(
      [
        "graph LR",
        '    analytics["analytics"]',
        '    audit_topic_exchange{{"audit.topic.exchange"}}',
        '    events_topic_exchange{{"events.topic.exchange"}}',
        '    analytics -->|"Audit.Entry"| audit_topic_exchange',
        '    events_topic_exchange -->|"Order.Created"| analytics',
        "    style analytics fill:#f9f,stroke:#333",
        "    style audit_topic_exchange,events_topic_exchange fill:#bbf,stroke:#333",
        "",
      ].join("\n"),
    );
  });

  it("skips topologies with empty service name", () => {
    const topologies: Topology[] = [
      {
        serviceName: "",
        endpoints: [
          {
            direction: "publish",
            pattern: "event-stream",
            exchangeName: "events.topic.exchange",
            exchangeKind: "topic",
            routingKey: "Test",
          },
        ],
      },
    ];

    expect(mermaid(topologies)).toBe("graph LR\n");
  });

  it("multi-transport subgraphs", () => {
    const topologies: Topology[] = [
      {
        transport: "amqp",
        serviceName: "orders",
        endpoints: [
          {
            direction: "publish",
            pattern: "event-stream",
            exchangeName: "events.topic.exchange",
            exchangeKind: "topic",
            routingKey: "Order.Created",
          },
        ],
      },
      {
        transport: "nats",
        serviceName: "orders",
        endpoints: [
          {
            direction: "publish",
            pattern: "event-stream",
            exchangeName: "events",
            exchangeKind: "topic",
            routingKey: "Order.Created",
          },
        ],
      },
    ];

    expect(mermaid(topologies)).toBe(
      [
        "graph LR",
        '    orders["orders"]',
        "    subgraph AMQP",
        '        amqp_events_topic_exchange{{"events.topic.exchange"}}',
        "    end",
        "    subgraph NATS",
        '        nats_events{{"events"}}',
        "    end",
        '    orders -->|"Order.Created"| nats_events',
        '    orders -->|"Order.Created"| amqp_events_topic_exchange',
        "    style orders fill:#f9f,stroke:#333",
        "    style nats_events,amqp_events_topic_exchange fill:#bbf,stroke:#333",
        "",
      ].join("\n"),
    );
  });

  it("skips endpoints with empty exchange name", () => {
    const topologies: Topology[] = [
      {
        serviceName: "svc",
        endpoints: [
          {
            direction: "publish",
            pattern: "event-stream",
            exchangeName: "",
            exchangeKind: "topic",
            routingKey: "Test",
          },
        ],
      },
    ];

    expect(mermaid(topologies)).toBe(
      [
        "graph LR",
        '    svc["svc"]',
        "    style svc fill:#f9f,stroke:#333",
        "",
      ].join("\n"),
    );
  });
});
