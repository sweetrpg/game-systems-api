package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
	apiv "github.com/sweetrpg/api-core.go/vo"
	"github.com/sweetrpg/authz-client.go/authz"
	"github.com/sweetrpg/common.go/logging"
	"github.com/sweetrpg/game-systems-api/constants"
	"github.com/sweetrpg/game-systems-api/models"
)

// listDeletedResponse is the GET /deleted-systems envelope.
type listDeletedResponse struct {
	Systems []*models.DeletedSystem `json:"systems"`
	Total   int                     `json:"total"`
	Page    int                     `json:"page"`
	PerPage int                     `json:"per_page"`
}

// setupDeletionHandlers registers soft delete (admin only) and the deleted-systems list. The
// list is unauthenticated like the other reads, so a service holding references can reconcile
// without a credential (PADR-0037, PADR-0047). It isn't under /systems/:id because "deleted" is
// a valid system_id.
func setupDeletionHandlers(g *gin.Engine, authzClient *authz.Client) {
	adminOnly := authz.RequireAnyRole(authzClient, constants.ServiceName, authz.RoleAdmin)

	g.DELETE("/systems/:id", adminOnly, deleteGameSystem)
	g.GET("/deleted-systems", listDeletedGameSystems)
}

// deleteGameSystem soft deletes a game system and then announces it. Only the request that
// changed the record publishes; if that publish is dropped, consumers recover through
// GET /deleted-systems.
//
//	@Summary		Delete a game system
//	@Description	Soft deletes a game system by document ID or system_id. Admin only.
//	@Tags			systems
//	@Param			id	path	string	true	"Document ID or system_id"
//	@Success		204
//	@Failure		401	{object}	apiv.ErrorVO
//	@Failure		403	{object}	apiv.ErrorVO
//	@Failure		404	{object}	apiv.ErrorVO	"unknown or already deleted"
//	@Router			/systems/{id} [delete]
func deleteGameSystem(c *gin.Context) {
	deleted, err := models.SoftDelete(c.Request.Context(), c.Param("id"), authz.Viewer(c))
	if err != nil {
		logging.Logger.Error("game system delete failed", "id", c.Param("id"), "error", err)
		c.JSON(http.StatusInternalServerError, apiv.ErrorVO{Error: "delete_failed", Message: "Unable to delete the game system"})
		return
	}
	if deleted == nil {
		c.JSON(http.StatusNotFound, apiv.ErrorVO{})
		return
	}
	if systemEvents != nil {
		systemEvents.PublishSystemDeleted(c.Request.Context(), deleted.ID, map[string]any{
			"system_id":  deleted.SystemID,
			"deleted_at": deleted.DeletedAt,
		})
	}
	c.Status(http.StatusNoContent)
}

// List deleted game systems.
//
//	@Summary		List deleted game systems
//	@Description	IDs and deletion times of soft-deleted systems, oldest first. Unauthenticated.
//	@Tags			systems
//	@Produce		json
//	@Param			page		query		int	false	"Page, from 1"
//	@Param			per_page	query		int	false	"Page size, at most 100"
//	@Success		200			{object}	listDeletedResponse
//	@Router			/deleted-systems [get]
func listDeletedGameSystems(c *gin.Context) {
	params, err := parseListParams(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, apiv.ErrorVO{Error: "invalid_request", Message: err.Error()})
		return
	}
	result, err := models.ListDeleted(c.Request.Context(), params.Page, params.PerPage)
	if err != nil {
		logging.Logger.Error("deleted game system list failed", "error", err)
		c.JSON(http.StatusInternalServerError, apiv.ErrorVO{Error: "query_failed", Message: "Unable to list deleted game systems"})
		return
	}
	c.JSON(http.StatusOK, listDeletedResponse{
		Systems: result.Systems, Total: result.Total, Page: result.Page, PerPage: result.PerPage,
	})
}
