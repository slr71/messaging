package messaging

import (
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

// AMQPHeaderCarrier adapts an amqp.Table for use as an OpenTelemetry
// TextMapCarrier, allowing trace context propagation through AMQP headers.
type AMQPHeaderCarrier amqp.Table

// Get returns the value associated with the passed key from the AMQP headers.
func (ahc AMQPHeaderCarrier) Get(key string) string {
	i := ahc[key]

	switch v := i.(type) {
	case int:
		return fmt.Sprintf("%d", v)
	case string:
		return v
	default:
		return ""
	}
}

// Set stores the key-value pair in the AMQP headers.
func (ahc AMQPHeaderCarrier) Set(key string, value string) {
	ahc[key] = value
}

// Keys returns all keys present in the AMQP headers.
func (ahc AMQPHeaderCarrier) Keys() []string {
	keys := make([]string, 0, len(ahc))
	for k := range ahc {
		keys = append(keys, k)
	}
	return keys
}
