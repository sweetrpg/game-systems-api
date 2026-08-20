package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
	apicores "github.com/sweetrpg/api-core.go/server"
	"github.com/sweetrpg/common.go/logging"
)

func setupStatusHandlers(g *gin.Engine) {
	logging.Logger.Info("Setting up status endpoint handlers...")

	g.GET("/status/health", healthHandler)
	g.GET("/status/ping", pingHandler)
}

func healthHandler(c *gin.Context) {
	resp := apicores.HealthHandler(c.Request.Context())
	status := http.StatusOK
	if resp.Errors > 0 {
		status = http.StatusServiceUnavailable
	}
	c.JSON(status, resp)
}

func pingHandler(c *gin.Context) {
	c.JSON(http.StatusOK, apicores.PingHandler())
}
