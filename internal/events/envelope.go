package events

import (
	"encoding/json"
	"time"
)

// Envelope is the JSON event payload published to NATS JetStream, conforming to the platform
// envelope schema in sweetrpg/platform's platform-messaging-nats spec.
//
// Fields: event_id (UUID string), occurred_at (RFC3339), source ("game-systems-api"),
// entity_type ("system"), entity_id (the system's meta document ID), action
// (created|updated|deleted), revision (the system's post-change version; 0 for delete),
// data (object; for system.updated it carries at least the current title; null for delete).
type Envelope struct {
	EventID    string          `json:"event_id"`
	OccurredAt string          `json:"occurred_at"`
	Source     string          `json:"source"`
	EntityType string          `json:"entity_type"`
	EntityID   string          `json:"entity_id"`
	Action     string          `json:"action"`
	Revision   int             `json:"revision"`
	Data       json.RawMessage `json:"data"`
}

const sourceName = "game-systems-api"

// NewEnvelope builds an event envelope. eventID must be a UUID string, revision is the entity's
// post-change version (0 for delete), and data is marshalled to JSON (null when nil).
func NewEnvelope(eventID, entityType, entityID, action string, revision int, data any) (*Envelope, error) {
	rawData := json.RawMessage("null")
	if data != nil {
		b, err := json.Marshal(data)
		if err != nil {
			return nil, err
		}
		rawData = b
	}

	return &Envelope{
		EventID:    eventID,
		OccurredAt: time.Now().UTC().Format(time.RFC3339),
		Source:     sourceName,
		EntityType: entityType,
		EntityID:   entityID,
		Action:     action,
		Revision:   revision,
		Data:       rawData,
	}, nil
}
