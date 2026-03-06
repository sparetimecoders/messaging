module github.com/sparetimecoders/messaging/tck

go 1.24.0

require (
	github.com/google/uuid v1.6.0
	github.com/nats-io/nats.go v1.49.0
	github.com/rabbitmq/amqp091-go v1.10.0
	github.com/sparetimecoders/messaging v0.0.0
	github.com/stretchr/testify v1.11.1
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/klauspost/compress v1.18.2 // indirect
	github.com/nats-io/nkeys v0.4.12 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	golang.org/x/crypto v0.46.0 // indirect
	golang.org/x/sys v0.39.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/sparetimecoders/messaging => ../
