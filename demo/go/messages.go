package main

// OrderCreated is published when a new order is placed.
type OrderCreated struct {
	OrderID   string  `json:"orderId"`
	Customer  string  `json:"customer"`
	Total     float64 `json:"total"`
	Source    string  `json:"source"`
	Transport string  `json:"transport"`
}

// PingMessage is a simple message for cross-language testing.
type PingMessage struct {
	Message   string `json:"message"`
	Source    string `json:"source"`
	Transport string `json:"transport"`
}

// EmailRequest represents a request to send an email.
type EmailRequest struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

// EmailResponse represents the result of sending an email.
type EmailResponse struct {
	MessageID string `json:"messageId"`
	Status    string `json:"status"`
}
