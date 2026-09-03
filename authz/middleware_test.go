package authz

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/sweetrpg/common.go/logging"
)

func init() { logging.Init() }

// newStub returns an httptest server that always allows the role check, and serves /profile with
// the given userID (empty userID => /profile 404, i.e. unresolvable).
func newStub(t *testing.T, userID string) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/profile" {
			if userID == "" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"user_id": userID})
			return
		}
		_ = json.NewEncoder(w).Encode(CheckResponse{Allowed: true, Roles: []string{RoleAdmin}, Sub: "sub-123"})
	}))
	t.Cleanup(s.Close)
	return s
}

func run(t *testing.T, client *Client) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/x", RequireAnyRole(client, "game-systems-api", RoleAdmin), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"viewer": Viewer(c)})
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	req.Header.Set("Authorization", "Bearer t")
	r.ServeHTTP(w, req)
	return w
}

func TestRequireAnyRole_StashesResolvedViewer(t *testing.T) {
	s := newStub(t, "canonical-id")
	w := run(t, NewClient(s.URL, s.URL))
	if w.Code != http.StatusOK {
		t.Fatalf("got %d, want 200; body %s", w.Code, w.Body.String())
	}
	if w.Body.String() != `{"viewer":"canonical-id"}` {
		t.Errorf("viewer body = %s, want canonical-id", w.Body.String())
	}
}

func TestRequireAnyRole_FailsClosedWhenViewerUnresolvable(t *testing.T) {
	s := newStub(t, "") // /profile 404
	w := run(t, NewClient(s.URL, s.URL))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("unresolvable caller: got %d, want 503; body %s", w.Code, w.Body.String())
	}
}

func TestRequireAnyRole_FailsClosedWhenUsersURLUnset(t *testing.T) {
	s := newStub(t, "canonical-id")
	w := run(t, NewClient(s.URL, "")) // no users-api configured
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("no users-api: got %d, want 503; body %s", w.Code, w.Body.String())
	}
}
