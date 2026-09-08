package models

import (
	"context"
	"testing"
	"time"

	modelcore "github.com/sweetrpg/model-core.go/models"
	"github.com/sweetrpg/mongodb.go/database"
	"github.com/testcontainers/testcontainers-go/modules/mongodb"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// setupCountTestDB starts a throwaway MongoDB container and points the models package's database
// handle at an empty database for the duration of the test.
func setupCountTestDB(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	container, err := mongodb.Run(ctx, "mongo:7")
	if err != nil {
		t.Fatalf("start mongo container: %v", err)
	}
	t.Cleanup(func() { _ = container.Terminate(ctx) })

	uri, err := container.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatalf("connect mongo: %v", err)
	}
	database.Db = client.Database("game-systems-count-test")
	t.Cleanup(func() {
		database.Db = nil
		_ = client.Disconnect(context.Background())
	})
}

func insertMeta(t *testing.T, id string, deleted bool) {
	t.Helper()
	now := time.Now()
	meta := EntityMeta{ID: id, SystemID: id, CurrentVersion: 1}
	modelcore.StampCreate(&meta.Auditable, "seed", now)
	if deleted {
		meta.DeletedAt = &now
	}
	if _, err := database.Db.Collection(metaCollection).InsertOne(context.Background(), meta); err != nil {
		t.Fatalf("insert meta %s: %v", id, err)
	}
}

func TestCountGameSystemsEmpty(t *testing.T) {
	setupCountTestDB(t)

	got, err := CountGameSystems(context.Background())
	if err != nil {
		t.Fatalf("CountGameSystems: %v", err)
	}
	if got != 0 {
		t.Fatalf("CountGameSystems on empty collection = %d, want 0", got)
	}
}

func TestCountGameSystemsCountsLiveRecords(t *testing.T) {
	setupCountTestDB(t)

	for _, id := range []string{"gs-1", "gs-2", "gs-3"} {
		insertMeta(t, id, false)
	}

	got, err := CountGameSystems(context.Background())
	if err != nil {
		t.Fatalf("CountGameSystems: %v", err)
	}
	if got != 3 {
		t.Fatalf("CountGameSystems = %d, want 3", got)
	}
}

func TestCountGameSystemsExcludesSoftDeleted(t *testing.T) {
	setupCountTestDB(t)

	insertMeta(t, "gs-live-1", false)
	insertMeta(t, "gs-live-2", false)
	insertMeta(t, "gs-deleted", true)

	got, err := CountGameSystems(context.Background())
	if err != nil {
		t.Fatalf("CountGameSystems: %v", err)
	}
	if got != 2 {
		t.Fatalf("CountGameSystems with one soft-deleted record = %d, want 2", got)
	}
}
