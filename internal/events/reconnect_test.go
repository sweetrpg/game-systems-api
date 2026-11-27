package events

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/sweetrpg/common.go/logging"
)

func TestPublisherRecoversWhenBrokerStartsLater(t *testing.T) {
	logging.Init()
	// Reserve a port, then free it so nothing is listening when the publisher starts.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	if err := l.Close(); err != nil {
		t.Fatalf("free port: %v", err)
	}
	t.Setenv("NATS_URL", "nats://127.0.0.1:"+strconv.Itoa(port))
	t.Setenv("NATS_CREDS", "")
	t.Setenv("NATS_USER", "")
	t.Setenv("PUBLISH_TIMEOUT_MS", "500")

	pub, err := NewPublisher(context.Background())
	if err != nil || pub == nil {
		t.Fatalf("NewPublisher with the broker down = %v, %v; want a publisher that retries", pub, err)
	}
	t.Cleanup(pub.Close)

	broker, err := natsserver.NewServer(&natsserver.Options{Host: "127.0.0.1", Port: port, JetStream: true, StoreDir: t.TempDir()})
	if err != nil {
		t.Fatalf("new NATS server: %v", err)
	}
	go broker.Start()
	t.Cleanup(broker.Shutdown)
	if !broker.ReadyForConnections(5 * time.Second) {
		t.Fatal("NATS server not ready")
	}
	for deadline := time.Now().Add(10 * time.Second); !pub.conn.IsConnected(); time.Sleep(50 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("publisher did not connect after the broker started")
		}
	}
	stream, err := pub.js.CreateStream(context.Background(), jetstream.StreamConfig{Name: "GAMESYSTEMS_EVENTS", Subjects: []string{"gamesystems.events.>"}})
	if err != nil {
		t.Fatalf("create stream: %v", err)
	}

	pub.PublishSystemDeleted(context.Background(), "sys1", map[string]any{"system_id": "numenera"})

	msg, err := stream.GetLastMsgForSubject(context.Background(), "gamesystems.events.system.deleted")
	if err != nil {
		t.Fatalf("deleted event not stored: %v", err)
	}
	if len(msg.Data) == 0 {
		t.Error("stored event is empty")
	}
}
