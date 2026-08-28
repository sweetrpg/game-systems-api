package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sweetrpg/common.go/logging"
	"github.com/sweetrpg/game-systems-api/authz"
	"github.com/sweetrpg/game-systems-api/models"
	"github.com/sweetrpg/mongodb.go/database"
	"github.com/testcontainers/testcontainers-go/modules/mongodb"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// setupTest starts a throwaway MongoDB container (testcontainers), points the models package's
// database handle at it, seeds one game system (meta + live version), and returns a Gin engine
// wired with the real handlers and an authz stub that allows every request.
func setupTest(t *testing.T) *gin.Engine {
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
	database.Db = client.Database("game-systems-test")
	t.Cleanup(func() {
		database.Db = nil
		_ = client.Disconnect(context.Background())
	})

	if err := models.EnsureIndexes(ctx); err != nil {
		t.Fatalf("ensure indexes: %v", err)
	}

	authzStub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(authz.CheckResponse{Allowed: true, Roles: []string{authz.RoleAdmin}, Sub: "test-user"})
	}))
	t.Cleanup(authzStub.Close)

	router := gin.New()
	setupGameSystemHandlers(router, authz.NewClient(authzStub.URL))

	if err := seedGameSystem(t, "64b000000000000000000001", "numenera"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return router
}

func seedGameSystem(t *testing.T, id, systemID string) error {
	t.Helper()
	ctx := context.Background()
	now := time.Now()
	meta := models.EntityMeta{ID: id, SystemID: systemID, CurrentVersion: 1, CreatedAt: now, CreatedBy: "seed"}
	if _, err := database.Db.Collection("game_systems_meta").InsertOne(ctx, meta); err != nil {
		return err
	}
	version := models.GameSystemVersion{
		ID: id + "v1", RecordID: id, Version: 1, Name: "Numenera", Edition: "1e",
		Notes: "seeded",
		VersionLifecycle: models.VersionLifecycle{
			State: models.VersionStateLive, SubmittedBy: "seed", SubmittedAt: now,
		},
	}
	_, err := database.Db.Collection("game_systems_versions").InsertOne(ctx, version)
	return err
}

func postJSON(t *testing.T, r *gin.Engine, path string, body any) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(body)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-token")
	r.ServeHTTP(w, req)
	var out map[string]any
	if len(w.Body.Bytes()) > 0 {
		_ = json.Unmarshal(w.Body.Bytes(), &out)
	}
	return w, out
}

func getJSON(t *testing.T, r *gin.Engine, path string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	r.ServeHTTP(w, req)
	var out map[string]any
	if len(w.Body.Bytes()) > 0 {
		_ = json.Unmarshal(w.Body.Bytes(), &out)
	}
	return w, out
}

func TestCreateGameSystemRequiresSystemID(t *testing.T) {
	r := setupTest(t)

	w, _ := postJSON(t, r, "/systems", map[string]any{
		"name": "Cypher System", "edition": "2e", "notes": "",
	})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("POST /systems without system_id: got %d want 400, body %s", w.Code, w.Body.String())
	}
}

func TestCreateGameSystemDuplicateSystemIDRejected(t *testing.T) {
	r := setupTest(t)

	w, _ := postJSON(t, r, "/systems", map[string]any{
		"name": "Numenera Clone", "system_id": "numenera", "edition": "1e",
	})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("POST /systems duplicate system_id: got %d want 400, body %s", w.Code, w.Body.String())
	}
	count, err := database.Db.Collection("game_systems_meta").CountDocuments(context.Background(),
		bson.D{{Key: "system_id", Value: "numenera"}})
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected exactly one numenera record after rejected create, got %d", count)
	}
}

func TestGetGameSystemByIDOrSystemID(t *testing.T) {
	r := setupTest(t)

	byObjectID, byObj := getJSON(t, r, "/systems/64b000000000000000000001")
	bySlug, bySlugBody := getJSON(t, r, "/systems/numenera")

	if byObjectID.Code != http.StatusOK || bySlug.Code != http.StatusOK {
		t.Fatalf("GET by _id=%d by system_id=%d", byObjectID.Code, bySlug.Code)
	}
	objName, _ := byObj["name"].(string)
	slugName, _ := bySlugBody["name"].(string)
	if objName == "" || objName != slugName {
		t.Fatalf("lookup by _id (%q) and system_id (%q) returned different results", objName, slugName)
	}
}

func TestGetGameSystemUnknownIdentifier(t *testing.T) {
	r := setupTest(t)

	w, _ := getJSON(t, r, "/systems/does-not-exist")

	if w.Code != http.StatusNotFound {
		t.Fatalf("GET /systems/does-not-exist: got %d want 404", w.Code)
	}
}
