package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/sweetrpg/common.go/logging"
)

func TestMetricsAreServedAtSlashMetrics(t *testing.T) {
	logging.Init()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	setupMetrics(r)
	r.GET("/status/ping", func(c *gin.Context) { c.String(http.StatusOK, "pong") })

	// A request first, so the request counters have something to report.
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/status/ping", nil))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("/metrics = %d, want 200", w.Code)
	}
	if body := w.Body.String(); !strings.Contains(body, "gin_request_total") {
		t.Errorf("/metrics does not expose the gin request metrics:\n%.300s", body)
	}
}
