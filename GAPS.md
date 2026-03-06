# Known Gaps and Future Work

## Spec Gaps

### `queue-publish` Pattern Has No Topology Scenario

The `PatternQueuePublish` pattern is defined in `topology.go` but has no test scenario in `testdata/topology.json`.

### No NATS Broker State in Topology Fixtures

The `topology.json` fixture includes `broker.amqp` state expectations but no `broker.nats` equivalent for verifying JetStream stream/consumer configuration.
