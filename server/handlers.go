package server

import (
	"github.com/gin-gonic/gin"
	"github.com/sweetrpg/game-systems-api/authz"
	"github.com/sweetrpg/game-systems-api/internal/events"
)

func SetupHandlers(g *gin.Engine, authzClient *authz.Client, pub events.SystemPublisher) {
	setupGameSystemHandlers(g, authzClient, pub)
	setupStatusHandlers(g)
}
