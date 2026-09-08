package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
	apiv "github.com/sweetrpg/api-core.go/vo"
	"github.com/sweetrpg/common.go/logging"
	"github.com/sweetrpg/game-systems-api/models"
)

// gameSystemStatsResponse is a plain JSON object, not a JSON:API resource - a singleton
// aggregate over the whole game-systems collection, matching catalog-api's /stats shape.
type gameSystemStatsResponse struct {
	GameSystems int64 `json:"game_systems"`
}

func setupStatsHandlers(g *gin.Engine) {
	logging.Logger.Info("Setting up stats endpoint handlers...")

	g.GET("/stats", getGameSystemStats)
}

// Get game-system stats.
//
//	@Summary		Get game-system stats
//	@Description	Returns the current count of game-system documents in a single call, mirroring catalog-api's /stats endpoint. Unauthenticated: exposes only an aggregate count, no per-record data.
//	@Tags			stats
//	@Produce		json
//	@Success		200	{object}	gameSystemStatsResponse
//	@Failure		500	{object}	interface{}
//	@Router			/stats [get]
func getGameSystemStats(c *gin.Context) {
	count, err := models.CountGameSystems(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, apiv.ErrorVO{Error: "query_failed", Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, gameSystemStatsResponse{GameSystems: count})
}
