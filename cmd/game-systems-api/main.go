package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/gin-gonic/gin"
	sloggin "github.com/samber/slog-gin"
	apiconstants "github.com/sweetrpg/api-core.go/constants"
	"github.com/sweetrpg/api-core.go/tracing"
	"github.com/sweetrpg/common.go/logging"
	"github.com/sweetrpg/common.go/util"
	"github.com/sweetrpg/game-systems-api/authz"
	"github.com/sweetrpg/game-systems-api/constants"
	"github.com/sweetrpg/game-systems-api/internal/events"
	"github.com/sweetrpg/game-systems-api/models"
	"github.com/sweetrpg/game-systems-api/server"
	"github.com/sweetrpg/mongodb.go/database"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
)

func main() {
	logging.Init()

	httpLogger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	r := gin.New()
	r.Use(sloggin.New(httpLogger))
	r.Use(gin.Recovery())

	tracing.SetupTracing(constants.ServiceName)
	defer tracing.TeardownTracing()
	r.Use(otelgin.Middleware(constants.ServiceName))

	database.SetupDatabase()
	defer database.TeardownDatabase()
	if err := models.EnsureIndexes(context.Background()); err != nil {
		logging.Logger.Error("Error while ensuring game system versioning indexes", "error", err.Error())
	}

	authzClient := authz.NewClient(util.GetEnv(constants.AUTH_API_URL, ""))

	var systemPublisher events.SystemPublisher
	if pub, err := events.NewPublisher(context.Background()); err != nil {
		logging.Logger.Error("Failed to initialize game-system event publisher", "error", err.Error())
	} else if pub != nil {
		defer pub.Close()
		systemPublisher = pub
	}

	server.SetupHandlers(r, authzClient, systemPublisher)

	_ = r.Run(util.GetEnv(apiconstants.BIND_ADDRESS, ":8000"))
}
