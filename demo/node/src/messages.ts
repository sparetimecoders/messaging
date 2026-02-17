export interface OrderCreated {
  orderId: string;
  customer: string;
  total: number;
  source: string;
  transport: string;
}

export interface PingMessage {
  message: string;
  source: string;
  transport: string;
}

export interface EmailRequest {
  to: string;
  subject: string;
  body: string;
}

export interface EmailResponse {
  messageId: string;
  status: string;
}
