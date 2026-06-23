package protocol

import (
	"encoding/json"
	"time"
)

type MessageType string

const (
	MessageTypeHello     MessageType = "hello"
	MessageTypeHelloAck  MessageType = "hello_ack"
	MessageTypeHeartbeat MessageType = "heartbeat"
	MessageTypeCommand   MessageType = "command"
	MessageTypeAck       MessageType = "ack"
	MessageTypeResult    MessageType = "result"
	MessageTypeStatus    MessageType = "status"
	MessageTypeError     MessageType = "error"
)

type Envelope struct {
	Version       string          `json:"version"`
	Type          MessageType     `json:"type"`
	MessageID     string          `json:"message_id"`
	CorrelationID string          `json:"correlation_id"`
	DeviceID      string          `json:"device_id"`
	Timestamp     time.Time       `json:"timestamp"`
	Payload       json.RawMessage `json:"payload"`
}
