package server

import (
	"github.com/gin-gonic/gin"
	"github.com/sweetrpg/game-systems-api/authz"
)

func SetupHandlers(g *gin.Engine, authzClient *authz.Client) {
	setupGameSystemHandlers(g, authzClient)
	setupStatusHandlers(g)
}
