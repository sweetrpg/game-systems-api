package events

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/sweetrpg/common.go/logging"
	"github.com/sweetrpg/common.go/util"
)

const (
	entityTypeSystem = "system"
	subjectPrefix    = "gamesystems.events"
)

// SystemPublisher is the seam the handlers depend on. The methods return nothing: publishing is
// fire-and-forget and fail-open, so a broker problem can never surface at a call site. A nil
// SystemPublisher is a valid no-op (publishing disabled).
type SystemPublisher interface {
	PublishSystemCreated(ctx context.Context, systemID string, revision int, data any)
	PublishSystemUpdated(ctx context.Context, systemID string, revision int, data any)
	PublishSystemDeleted(ctx context.Context, systemID string)
}

// Publisher publishes game-system change events to NATS JetStream.
type Publisher struct {
	conn           *nats.Conn
	js             jetstream.JetStream
	publishTimeout time.Duration
}

var _ SystemPublisher = (*Publisher)(nil)

// NewPublisher builds a Publisher from the environment. It returns (nil, nil) when NATS_URL is
// unset, so the caller treats event publishing as disabled rather than a startup error.
//
//	NATS_URL           NATS server URL (unset -> publishing disabled)
//	NATS_CREDS         credentials file path (optional)
//	NATS_USER/NATS_PASSWORD  username/password auth (used when NATS_CREDS is unset)
//	PUBLISH_TIMEOUT_MS bounded wait for a publish ack (default 3000)
func NewPublisher(ctx context.Context) (*Publisher, error) {
	natsURL := util.GetEnv("NATS_URL", "")
	if natsURL == "" {
		logging.Logger.Warn("NATS_URL not set; game-system event publishing disabled")
		return nil, nil
	}

	opts := []nats.Option{}
	if creds := os.Getenv("NATS_CREDS"); creds != "" {
		opts = append(opts, nats.UserCredentials(creds))
	} else if user := os.Getenv("NATS_USER"); user != "" {
		opts = append(opts, nats.UserInfo(user, os.Getenv("NATS_PASSWORD")))
	}

	conn, err := nats.Connect(natsURL, opts...)
	if err != nil {
		return nil, fmt.Errorf("nats connect: %w", err)
	}

	js, err := jetstream.New(conn)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("jetstream: %w", err)
	}

	timeoutMs := util.GetEnvInt("PUBLISH_TIMEOUT_MS", 3000)
	p := &Publisher{
		conn:           conn,
		js:             js,
		publishTimeout: time.Duration(timeoutMs) * time.Millisecond,
	}

	logging.Logger.Info("Game-system event publisher initialized", "nats_url", natsURL, "publish_timeout_ms", timeoutMs)
	return p, nil
}

// Close closes the NATS connection.
func (p *Publisher) Close() {
	if p != nil && p.conn != nil {
		p.conn.Close()
	}
}

func (p *Publisher) PublishSystemCreated(ctx context.Context, systemID string, revision int, data any) {
	p.publish(ctx, systemID, "created", revision, data)
}

func (p *Publisher) PublishSystemUpdated(ctx context.Context, systemID string, revision int, data any) {
	p.publish(ctx, systemID, "updated", revision, data)
}

func (p *Publisher) PublishSystemDeleted(ctx context.Context, systemID string) {
	p.publish(ctx, systemID, "deleted", 0, nil)
}

func (p *Publisher) publish(ctx context.Context, systemID, action string, revision int, data any) {
	if p == nil || p.conn == nil {
		return
	}

	eventID := uuid.NewString()
	envelope, err := NewEnvelope(eventID, entityTypeSystem, systemID, action, revision, data)
	if err != nil {
		logging.Logger.Error("event envelope build failed (event dropped)", "system_id", systemID, "action", action, "error", err)
		return
	}

	subject, body, msgID, err := buildPublish(envelope)
	if err != nil {
		logging.Logger.Error("event marshal failed (event dropped)", "system_id", systemID, "action", action, "error", err)
		return
	}

	ctx, cancel := context.WithTimeout(ctx, p.publishTimeout)
	defer cancel()

	// ponytail: fail-open on error/timeout. An in-process retry queue loses events on restart
	// and grows unbounded; JetStream redelivery is the consumer's job, not ours.
	if _, err := p.js.Publish(ctx, subject, body, jetstream.WithMsgID(msgID)); err != nil {
		logging.Logger.Error("event publish failed (event dropped)", "subject", subject, "system_id", systemID, "event_id", eventID, "error", err)
		return
	}

	logging.Logger.Info("event published", "subject", subject, "system_id", systemID, "event_id", eventID)
}

// buildPublish derives the subject, JSON body, and de-duplication message ID for an envelope.
// The message ID is the envelope's event_id, so a redelivered publish is de-duplicated by the
// stream's duplicate window.
func buildPublish(e *Envelope) (subject string, body []byte, msgID string, err error) {
	body, err = json.Marshal(e)
	if err != nil {
		return "", nil, "", err
	}
	return fmt.Sprintf("%s.%s.%s", subjectPrefix, e.EntityType, e.Action), body, e.EventID, nil
}
