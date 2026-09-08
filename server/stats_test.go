package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sweetrpg/common.go/logging"
	"github.com/sweetrpg/game-systems-api/models"
	modelcore "github.com/sweetrpg/model-core.go/models"
	"github.com/sweetrpg/mongodb.go/database"
	"github.com/testcontainers/testcontainers-go/modules/mongodb"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// setupStatsTest starts a throwaway MongoDB container, points the models package at an empty
// database, and returns a Gin engine wired with only the stats handler (no authz).
func setupStatsTest(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	logging.Init()

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
	database.Db = client.Database("game-systems-stats-test")
	t.Cleanup(func() {
		database.Db = nil
		_ = client.Disconnect(context.Background())
	})

	router := gin.New()
	setupStatsHandlers(router)
	return router
}

func seedMeta(t *testing.T, id string) {
	t.Helper()
	now := time.Now()
	meta := models.EntityMeta{ID: id, SystemID: id, CurrentVersion: 1}
	modelcore.StampCreate(&meta.Auditable, "seed", now)
	if _, err := database.Db.Collection("game_systems_meta").InsertOne(context.Background(), meta); err != nil {
		t.Fatalf("seed meta %s: %v", id, err)
	}
}

func TestStatsReturnsCountAndShape(t *testing.T) {
	r := setupStatsTest(t)
	seedMeta(t, "gs-1")
	seedMeta(t, "gs-2")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/stats", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("GET /stats: got %d want 200, body %s", w.Code, w.Body.String())
	}
	var body map[string]json.Number
	dec := json.NewDecoder(w.Body)
	dec.UseNumber()
	if err := dec.Decode(&body); err != nil {
		t.Fatalf("decode body %q: %v", w.Body.String(), err)
	}
	got, ok := body["game_systems"]
	if !ok {
		t.Fatalf("response missing game_systems key: %s", w.Body.String())
	}
	if got.String() != "2" {
		t.Fatalf("game_systems = %s, want 2", got.String())
	}
}

func TestStatsEmptyCollectionReturnsZero(t *testing.T) {
	r := setupStatsTest(t)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/stats", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("GET /stats: got %d want 200", w.Code)
	}
	var body struct {
		GameSystems int `json:"game_systems"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.GameSystems != 0 {
		t.Fatalf("game_systems = %d, want 0", body.GameSystems)
	}
}

func TestStatsRequiresNoCredentials(t *testing.T) {
	r := setupStatsTest(t)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/stats", nil)
	// No Authorization header set.
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /stats with no credentials: got %d want 200, body %s", w.Code, w.Body.String())
	}
}
