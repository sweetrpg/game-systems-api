package server

import (
	"context"
	"net/http"
	"testing"

	"github.com/sweetrpg/game-systems-api/models"
	"github.com/sweetrpg/mongodb.go/database"
	"go.mongodb.org/mongo-driver/bson"
)

// Asserts the platform audit-fields convention (PADR-0001) on game_systems_meta. Container-backed
// like the rest of this package - runs in CI, skipped locally without Docker.

func loadMeta(t *testing.T, systemID string) models.EntityMeta {
	t.Helper()
	var m models.EntityMeta
	err := database.Db.Collection("game_systems_meta").
		FindOne(context.Background(), bson.D{{Key: "system_id", Value: systemID}}).Decode(&m)
	if err != nil {
		t.Fatalf("load meta %q: %v", systemID, err)
	}
	return m
}

func TestCreateGameSystem_StampsMetaAuditFields(t *testing.T) {
	r, _ := setupTest(t)

	w, _ := postJSON(t, r, "/systems", map[string]any{
		"name": "Cypher System", "system_id": "cypher-system", "edition": "2e",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("POST /systems: got %d, body %s", w.Code, w.Body.String())
	}

	m := loadMeta(t, "cypher-system")
	if m.CreatedAt.IsZero() || m.UpdatedAt.IsZero() {
		t.Errorf("meta created_at/updated_at are zero: %+v", m.Auditable)
	}
	if !m.CreatedAt.Equal(m.UpdatedAt) {
		t.Errorf("fresh meta created_at %v != updated_at %v", m.CreatedAt, m.UpdatedAt)
	}
	if m.CreatedBy != "test-user" || m.UpdatedBy != "test-user" {
		t.Errorf("meta *_by = (%q, %q), want the resolved caller \"test-user\"", m.CreatedBy, m.UpdatedBy)
	}
	if m.DeletedAt != nil || m.DeletedBy != nil {
		t.Errorf("fresh meta has non-nil deleted_at/deleted_by: %+v", m.Auditable)
	}
}

func TestPatchGameSystemLive_AdvancesMetaUpdateAudit(t *testing.T) {
	r, _ := setupTestAs(t, []string{"admin"}) // admin patch -> Live -> setMetaCurrentVersion

	before := loadMeta(t, "numenera")

	w, _ := patchJSON(t, r, "/systems/numenera", map[string]any{"edition": "discovery"})
	if w.Code != http.StatusOK {
		t.Fatalf("PATCH /systems/numenera: got %d, body %s", w.Code, w.Body.String())
	}

	after := loadMeta(t, "numenera")
	if after.UpdatedBy != "test-user" {
		t.Errorf("updated_by = %q after live patch, want \"test-user\"", after.UpdatedBy)
	}
	if after.UpdatedAt.Before(before.CreatedAt) {
		t.Errorf("updated_at %v is before created_at %v", after.UpdatedAt, before.CreatedAt)
	}
	if after.CreatedBy != before.CreatedBy {
		t.Errorf("created_by changed on patch: %q -> %q", before.CreatedBy, after.CreatedBy)
	}
}
