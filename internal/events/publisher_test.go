package events

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

// A nil *Publisher and a Publisher with no connection are safe no-ops: publishing is disabled
// or fail-open, never a panic at the call site.
func TestPublisherNoopWhenUnconnected(t *testing.T) {
	ctx := context.Background()
	var nilPub *Publisher
	nilPub.PublishSystemCreated(ctx, "sys1", 1, nil)
	nilPub.PublishSystemUpdated(ctx, "sys1", 2, map[string]any{"title": "x"})
	nilPub.PublishSystemDeleted(ctx, "sys1")

	empty := &Publisher{}
	empty.PublishSystemUpdated(ctx, "sys1", 3, nil)
}

func TestNewEnvelopeFields(t *testing.T) {
	id := uuid.NewString()
	env, err := NewEnvelope(id, "system", "64b000000000000000000001", "updated", 3, map[string]any{"title": "Numenera"})
	if err != nil {
		t.Fatalf("NewEnvelope: %v", err)
	}
	if env.Source != "game-systems-api" {
		t.Errorf("source = %q, want game-systems-api", env.Source)
	}
	if env.EntityType != "system" {
		t.Errorf("entity_type = %q, want system", env.EntityType)
	}
	if env.EntityID == "" {
		t.Error("entity_id is empty")
	}
	if _, err := uuid.Parse(env.EventID); err != nil {
		t.Errorf("event_id %q is not a UUID: %v", env.EventID, err)
	}
	if env.OccurredAt == "" {
		t.Error("occurred_at is empty")
	}
	var data map[string]any
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("data not JSON: %v", err)
	}
	if data["title"] != "Numenera" {
		t.Errorf("data.title = %v, want Numenera", data["title"])
	}
}

func TestNewEnvelopeDeleteHasNullData(t *testing.T) {
	env, err := NewEnvelope(uuid.NewString(), "system", "sys1", "deleted", 0, nil)
	if err != nil {
		t.Fatalf("NewEnvelope: %v", err)
	}
	if string(env.Data) != "null" {
		t.Errorf("delete data = %s, want null", env.Data)
	}
}

func TestBuildPublishSubjectAndMsgID(t *testing.T) {
	env, err := NewEnvelope(uuid.NewString(), "system", "sys1", "updated", 1, nil)
	if err != nil {
		t.Fatalf("NewEnvelope: %v", err)
	}
	subject, body, msgID, err := buildPublish(env)
	if err != nil {
		t.Fatalf("buildPublish: %v", err)
	}
	if subject != "gamesystems.events.system.updated" {
		t.Errorf("subject = %q", subject)
	}
	if msgID != env.EventID {
		t.Errorf("msgID = %q, want event_id %q (de-dup header must equal event_id)", msgID, env.EventID)
	}
	var round Envelope
	if err := json.Unmarshal(body, &round); err != nil {
		t.Fatalf("body not an Envelope: %v", err)
	}
	if round.EventID != env.EventID {
		t.Errorf("round-trip event_id = %q, want %q", round.EventID, env.EventID)
	}
}
