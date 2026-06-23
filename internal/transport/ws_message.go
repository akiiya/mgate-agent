package transport

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"mgate-agent/internal/protocol"
)

const envelopeVersion = "1"

func makeEnvelope(messageType protocol.MessageType, deviceID, correlationID string, payload any) (protocol.Envelope, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return protocol.Envelope{}, err
	}
	return protocol.Envelope{
		Version:       envelopeVersion,
		Type:          messageType,
		MessageID:     newMessageID(),
		CorrelationID: correlationID,
		DeviceID:      deviceID,
		Timestamp:     time.Now().UTC(),
		Payload:       raw,
	}, nil
}

func newMessageID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("msg_%d", time.Now().UnixNano())
	}
	return "msg_" + hex.EncodeToString(b[:])
}
