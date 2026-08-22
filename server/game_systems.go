package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	apiv "github.com/sweetrpg/api-core.go/vo"
	"github.com/sweetrpg/common.go/logging"
	"github.com/sweetrpg/gamesystems-api/authz"
	"github.com/sweetrpg/gamesystems-api/constants"
	"github.com/sweetrpg/gamesystems-api/models"
	"go.mongodb.org/mongo-driver/mongo"
)

type submittedVersionResponse struct {
	Version int    `json:"version"`
	State   string `json:"state"`
	Message string `json:"message"`
}

type reviewVersionResponse struct {
	Version   int      `json:"version"`
	State     string   `json:"state"`
	Conflicts []string `json:"conflicts,omitempty"`
}

type acceptVersionRequest struct {
	// Fields lists which changed field names to accept; the rest are left unaccepted. Nil (the
	// key omitted entirely) means accept every changed field.
	Fields *[]string `json:"fields"`
}

type rejectVersionRequest struct {
	Note string `json:"note"`
}

func setupGameSystemHandlers(g *gin.Engine, authzClient *authz.Client) {
	logging.Logger.Info("Setting up game system endpoint handlers...")

	g.GET("/systems", listGameSystems)
	g.GET("/systems/:id", getGameSystem)
	g.GET("/systems/:id/versions", listGameSystemVersions)
	g.GET("/systems/:id/versions/:version", getGameSystemVersion)

	writeRoles := authz.RequireAnyRole(authzClient, constants.ServiceName, authz.RoleAdmin, authz.RoleEditor, authz.RoleSubmitter)
	reviewRoles := authz.RequireAnyRole(authzClient, constants.ServiceName, authz.RoleAdmin, authz.RoleModerator, authz.RoleApprover)

	g.POST("/systems", writeRoles, createGameSystem)
	g.PATCH("/systems/:id", writeRoles, patchGameSystem)
	g.POST("/systems/:id/versions/:version/retract", writeRoles, retractGameSystemVersion)
	g.POST("/systems/:id/versions/:version/accept", reviewRoles, acceptGameSystemVersion)
	g.POST("/systems/:id/versions/:version/reject", reviewRoles, rejectGameSystemVersion)
	g.POST("/systems/:id/versions/:version/current", reviewRoles, setCurrentGameSystemVersion)
}

func listGameSystems(c *gin.Context) {
	results, err := models.List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, apiv.ErrorVO{Error: "query_failed", Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, results)
}

func getGameSystem(c *gin.Context) {
	result, err := models.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, apiv.ErrorVO{Error: "query_failed", Message: err.Error()})
		return
	}
	if result == nil {
		c.JSON(http.StatusNotFound, apiv.ErrorVO{})
		return
	}
	c.JSON(http.StatusOK, result)
}

type createGameSystemRequest struct {
	models.GameSystemVersion `bson:",inline"`
	SystemID                 string `json:"system_id" binding:"required"`
}

func createGameSystem(c *gin.Context) {
	var req createGameSystemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, apiv.ErrorVO{Error: "invalid_request", Message: err.Error()})
		return
	}
	if !models.ValidateSystemID(req.SystemID) {
		c.JSON(http.StatusBadRequest, apiv.ErrorVO{Error: "invalid_request", Message: "system_id must be a lowercase kebab-case slug"})
		return
	}
	entity := req.GameSystemVersion

	id, err := models.Create(c.Request.Context(), &entity, req.SystemID, authz.Subject(c))
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			c.JSON(http.StatusBadRequest, apiv.ErrorVO{Error: "invalid_request", Message: "system_id already in use"})
			return
		}
		c.JSON(http.StatusInternalServerError, apiv.ErrorVO{Error: "create_failed", Message: err.Error()})
		return
	}
	if id == nil {
		c.JSON(http.StatusInternalServerError, apiv.ErrorVO{Error: "create_failed", Message: "no id returned"})
		return
	}

	result, err := models.Get(c.Request.Context(), *id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, apiv.ErrorVO{Error: "query_failed", Message: err.Error()})
		return
	}
	c.JSON(http.StatusCreated, result)
}

func patchGameSystem(c *gin.Context) {
	id := c.Param("id")

	var req map[string]json.RawMessage
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, apiv.ErrorVO{Error: "invalid_request", Message: err.Error()})
		return
	}

	existing, err := models.Get(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, apiv.ErrorVO{Error: "query_failed", Message: err.Error()})
		return
	}
	if existing == nil {
		c.JSON(http.StatusNotFound, apiv.ErrorVO{})
		return
	}

	updated := *existing
	hasKnownField := false
	for field, raw := range req {
		if len(raw) == 0 || string(raw) == "null" {
			continue
		}
		switch field {
		case "name":
			if err := json.Unmarshal(raw, &updated.Name); err != nil {
				c.JSON(http.StatusBadRequest, apiv.ErrorVO{Error: "invalid_request", Message: field + ": " + err.Error()})
				return
			}
		case "edition":
			if err := json.Unmarshal(raw, &updated.Edition); err != nil {
				c.JSON(http.StatusBadRequest, apiv.ErrorVO{Error: "invalid_request", Message: field + ": " + err.Error()})
				return
			}
		case "publisher_id":
			if err := json.Unmarshal(raw, &updated.PublisherID); err != nil {
				c.JSON(http.StatusBadRequest, apiv.ErrorVO{Error: "invalid_request", Message: field + ": " + err.Error()})
				return
			}
		case "notes":
			if err := json.Unmarshal(raw, &updated.Notes); err != nil {
				c.JSON(http.StatusBadRequest, apiv.ErrorVO{Error: "invalid_request", Message: field + ": " + err.Error()})
				return
			}
		case "tags":
			if err := json.Unmarshal(raw, &updated.Tags); err != nil {
				c.JSON(http.StatusBadRequest, apiv.ErrorVO{Error: "invalid_request", Message: field + ": " + err.Error()})
				return
			}
		default:
			continue
		}
		hasKnownField = true
	}
	if !hasKnownField {
		c.JSON(http.StatusBadRequest, apiv.ErrorVO{Error: "invalid_request", Message: "no recognized fields to change"})
		return
	}

	roles := authz.Roles(c)
	state := models.VersionStateSubmitted
	if authz.HasRole(roles, authz.RoleAdmin) || authz.HasRole(roles, authz.RoleEditor) {
		state = models.VersionStateLive
	}

	version, err := models.CreateVersion(c.Request.Context(), id, &updated, state, authz.Subject(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, apiv.ErrorVO{Error: "update_failed", Message: err.Error()})
		return
	}
	if version == nil {
		c.JSON(http.StatusNotFound, apiv.ErrorVO{})
		return
	}

	if state != models.VersionStateLive {
		c.JSON(http.StatusAccepted, submittedVersionResponse{
			Version: version.Version, State: string(version.State), Message: "Change submitted for review",
		})
		return
	}

	result, err := models.Get(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, apiv.ErrorVO{Error: "query_failed", Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

func versionParam(c *gin.Context) (int, bool) {
	version, err := strconv.Atoi(c.Param("version"))
	if err != nil || version < 1 {
		c.JSON(http.StatusBadRequest, apiv.ErrorVO{Error: "invalid_request", Message: "version must be a positive integer"})
		return 0, false
	}
	return version, true
}

func listGameSystemVersions(c *gin.Context) {
	meta, err := models.GetMeta(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, apiv.ErrorVO{Error: "query_failed", Message: err.Error()})
		return
	}
	if meta == nil {
		c.JSON(http.StatusNotFound, apiv.ErrorVO{})
		return
	}
	versions, err := models.ListVersions(c.Request.Context(), meta.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, apiv.ErrorVO{Error: "query_failed", Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, versions)
}

func getGameSystemVersion(c *gin.Context) {
	version, ok := versionParam(c)
	if !ok {
		return
	}
	meta, err := models.GetMeta(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, apiv.ErrorVO{Error: "query_failed", Message: err.Error()})
		return
	}
	if meta == nil {
		c.JSON(http.StatusNotFound, apiv.ErrorVO{})
		return
	}
	result, err := models.GetVersion(c.Request.Context(), meta.ID, version)
	if err != nil {
		c.JSON(http.StatusInternalServerError, apiv.ErrorVO{Error: "query_failed", Message: err.Error()})
		return
	}
	if result == nil {
		c.JSON(http.StatusNotFound, apiv.ErrorVO{})
		return
	}
	c.JSON(http.StatusOK, result)
}

func acceptGameSystemVersion(c *gin.Context) {
	id := c.Param("id")
	version, ok := versionParam(c)
	if !ok {
		return
	}
	var req acceptVersionRequest
	if err := c.ShouldBindJSON(&req); err != nil && err.Error() != "EOF" {
		c.JSON(http.StatusBadRequest, apiv.ErrorVO{Error: "invalid_request", Message: err.Error()})
		return
	}
	var selectedFields []string
	if req.Fields != nil {
		selectedFields = *req.Fields
	}
	accepted, conflicts, err := models.AcceptVersion(c.Request.Context(), id, version, selectedFields, authz.Subject(c), nil)
	if err != nil {
		c.JSON(http.StatusBadRequest, apiv.ErrorVO{Error: "accept_failed", Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, reviewVersionResponse{Version: accepted.Version, State: string(accepted.State), Conflicts: conflicts})
}

func rejectGameSystemVersion(c *gin.Context) {
	id := c.Param("id")
	version, ok := versionParam(c)
	if !ok {
		return
	}
	var req rejectVersionRequest
	if err := c.ShouldBindJSON(&req); err != nil && err.Error() != "EOF" {
		c.JSON(http.StatusBadRequest, apiv.ErrorVO{Error: "invalid_request", Message: err.Error()})
		return
	}
	var note *string
	if req.Note != "" {
		note = &req.Note
	}
	if err := models.RejectVersion(c.Request.Context(), id, version, authz.Subject(c), note); err != nil {
		c.JSON(http.StatusBadRequest, apiv.ErrorVO{Error: "reject_failed", Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, reviewVersionResponse{Version: version, State: "rejected"})
}

func retractGameSystemVersion(c *gin.Context) {
	id := c.Param("id")
	version, ok := versionParam(c)
	if !ok {
		return
	}
	retracted, err := models.RetractVersion(c.Request.Context(), id, version, authz.Subject(c))
	if err != nil {
		c.JSON(http.StatusBadRequest, apiv.ErrorVO{Error: "retract_failed", Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, reviewVersionResponse{Version: retracted.Version, State: string(retracted.State)})
}

func setCurrentGameSystemVersion(c *gin.Context) {
	id := c.Param("id")
	version, ok := versionParam(c)
	if !ok {
		return
	}
	result, err := models.SetCurrentVersion(c.Request.Context(), id, version)
	if err != nil {
		c.JSON(http.StatusBadRequest, apiv.ErrorVO{Error: "rollback_failed", Message: err.Error()})
		return
	}
	if result == nil {
		c.JSON(http.StatusNotFound, apiv.ErrorVO{})
		return
	}
	c.JSON(http.StatusOK, result)
}
