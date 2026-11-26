package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/sweetrpg/authz-client.go/authz"
	"github.com/sweetrpg/game-systems-api/models"
	"github.com/sweetrpg/mongodb.go/database"
	"go.mongodb.org/mongo-driver/bson"
)

const seededID = "64b000000000000000000001"

func serve(r *gin.Engine, method, path, body, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func storedMeta(t *testing.T) models.EntityMeta {
	t.Helper()
	var meta models.EntityMeta
	if err := database.Db.Collection("game_systems_meta").FindOne(context.Background(), bson.D{{Key: "_id", Value: seededID}}).Decode(&meta); err != nil {
		t.Fatalf("read meta: %v", err)
	}
	return meta
}

func TestAdminDeleteSoftDeletesAndPublishes(t *testing.T) {
	for _, id := range []string{seededID, "numenera"} {
		t.Run(id, func(t *testing.T) {
			r, pub := setupTest(t)

			if w := serve(r, http.MethodDelete, "/systems/"+id, "", "t"); w.Code != http.StatusNoContent {
				t.Fatalf("delete = %d, body = %s", w.Code, w.Body.String())
			}

			meta := storedMeta(t)
			if meta.DeletedAt == nil || meta.DeletedBy == nil || *meta.DeletedBy != "test-user" || meta.UpdatedBy != "test-user" {
				t.Errorf("audit fields not stamped: %+v", meta.Auditable)
			}
			versions, err := database.Db.Collection("game_systems_versions").CountDocuments(context.Background(), bson.D{{Key: "record_id", Value: seededID}})
			if err != nil || versions != 1 {
				t.Errorf("versions = %d (err %v), want 1 kept", versions, err)
			}
			want := recordedEvent{action: "deleted", systemID: seededID, slug: "numenera"}
			if got := pub.calls(); len(got) != 1 || got[0] != want {
				t.Errorf("events = %+v, want [%+v]", got, want)
			}
			for _, path := range []string{"/systems/" + seededID, "/systems/numenera", "/systems/" + seededID + "v1", "/systems/numenera/versions", "/systems/numenera/versions/1"} {
				if w := serve(r, http.MethodGet, path, "", ""); w.Code != http.StatusNotFound {
					t.Errorf("GET %s = %d, want 404", path, w.Code)
				}
			}
			var list listSystemsResponse
			if err := json.Unmarshal(serve(r, http.MethodGet, "/systems", "", "").Body.Bytes(), &list); err != nil {
				t.Fatalf("decode list: %v", err)
			}
			if list.Total != 0 || len(list.Systems) != 0 {
				t.Errorf("deleted system still listed: %+v", list)
			}
		})
	}
}

func TestDeleteAgainIsNotFoundAndKeepsAudit(t *testing.T) {
	r, pub := setupTest(t)
	serve(r, http.MethodDelete, "/systems/"+seededID, "", "t")
	first := storedMeta(t)

	for _, id := range []string{seededID, "no-such-system"} {
		if w := serve(r, http.MethodDelete, "/systems/"+id, "", "t"); w.Code != http.StatusNotFound {
			t.Errorf("delete %s = %d, want 404", id, w.Code)
		}
	}
	if got := storedMeta(t); !got.DeletedAt.Equal(*first.DeletedAt) {
		t.Errorf("deleted_at changed: %v -> %v", first.DeletedAt, got.DeletedAt)
	}
	if got := len(pub.calls()); got != 1 {
		t.Errorf("published %d events, want 1", got)
	}
}

func TestDeleteRequiresAdmin(t *testing.T) {
	for _, tc := range []struct {
		name, role, token string
		want              int
	}{
		{"editor", authz.RoleEditor, "t", http.StatusForbidden},
		{"moderator", authz.RoleModerator, "t", http.StatusForbidden},
		{"submitter", authz.RoleSubmitter, "t", http.StatusForbidden},
		{"anonymous", authz.RoleAdmin, "", http.StatusUnauthorized},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, pub := setupTestAs(t, []string{tc.role})

			if w := serve(r, http.MethodDelete, "/systems/"+seededID, "", tc.token); w.Code != tc.want {
				t.Errorf("delete = %d, want %d", w.Code, tc.want)
			}
			if storedMeta(t).DeletedAt != nil || len(pub.calls()) != 0 {
				t.Error("refused delete changed the record or published")
			}
		})
	}
}

func TestDeletedSystemKeepsItsSlugAndIsListed(t *testing.T) {
	r, _ := setupTest(t)
	if err := seedGameSystem(t, "64b000000000000000000002", "cypher"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	serve(r, http.MethodDelete, "/systems/numenera", "", "t")

	w := serve(r, http.MethodPost, "/systems", `{"system_id":"numenera","name":"Again","edition":"2e"}`, "t")
	if w.Code != http.StatusBadRequest {
		t.Errorf("create with a deleted slug = %d, want 400, body = %s", w.Code, w.Body.String())
	}

	var got listDeletedResponse
	w = serve(r, http.MethodGet, "/deleted-systems", "", "")
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || w.Code != http.StatusOK {
		t.Fatalf("deleted list = %d, err %v, body = %s", w.Code, err, w.Body.String())
	}
	if got.Total != 1 || len(got.Systems) != 1 || got.Systems[0].ID != seededID || got.Systems[0].SystemID != "numenera" || got.Systems[0].DeletedAt.IsZero() {
		t.Errorf("deleted list = %s", w.Body.String())
	}
	if err := json.Unmarshal(serve(r, http.MethodGet, "/deleted-systems?page=2&per_page=1", "", "").Body.Bytes(), &got); err != nil {
		t.Fatalf("decode page 2: %v", err)
	}
	if got.Total != 1 || len(got.Systems) != 0 {
		t.Errorf("page 2 = %+v, want empty with total 1", got)
	}
}

func TestPatchBySlugPublishesDocumentID(t *testing.T) {
	r, pub := setupTest(t)

	if w := serve(r, http.MethodPatch, "/systems/numenera", `{"name":"Numenera 2"}`, "t"); w.Code != http.StatusOK {
		t.Fatalf("patch = %d, body = %s", w.Code, w.Body.String())
	}
	if got := pub.calls(); len(got) != 1 || got[0].systemID != seededID {
		t.Errorf("events = %+v, want one with entity ID %s", got, seededID)
	}
}
