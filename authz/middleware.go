package authz

import (
	"net/http"

	"github.com/gin-gonic/gin"
	apiv "github.com/sweetrpg/api-core.go/vo"
	"github.com/sweetrpg/common.go/logging"
)

// Gin context keys set by RequireAnyRole; read via Roles/Subject.
const (
	rolesContextKey   = "authz.roles"
	subjectContextKey = "authz.subject"
)

// RequireAnyRole returns Gin middleware that verifies the caller's bearer token against
// auth-api's /authz/check for the given service name, then requires the caller hold at least
// one of allowedRoles. On success, the verified roles and subject are stashed in the Gin
// context (read via Roles(c) / Subject(c)) for the handler to use.
func RequireAnyRole(client *Client, service string, allowedRoles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := bearerToken(c)
		if token == "" {
			unauthorized(c)
			return
		}

		result, err := client.Check(c.Request.Context(), token, service)
		if err != nil {
			if _, ok := err.(InvalidTokenError); ok {
				unauthorized(c)
				return
			}
			logging.Logger.Error("authz check failed", "error", err.Error())
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, apiv.ErrorVO{
				Error:   "authz_unavailable",
				Message: "Unable to verify authorization",
			})
			return
		}

		if !result.Allowed {
			forbidden(c)
			return
		}

		if !hasAnyRole(result.Roles, allowedRoles) {
			forbidden(c)
			return
		}

		c.Set(rolesContextKey, result.Roles)
		c.Set(subjectContextKey, result.Sub)
		c.Next()
	}
}

// Roles returns the verified roles stashed in the Gin context by RequireAnyRole.
func Roles(c *gin.Context) []string {
	if v, ok := c.Get(rolesContextKey); ok {
		if roles, ok := v.([]string); ok {
			return roles
		}
	}
	return nil
}

// Subject returns the verified Auth0 subject stashed in the Gin context by RequireAnyRole.
func Subject(c *gin.Context) string {
	if v, ok := c.Get(subjectContextKey); ok {
		if sub, ok := v.(string); ok {
			return sub
		}
	}
	return ""
}

// HasRole reports whether roles contains want.
func HasRole(roles []string, want string) bool {
	for _, r := range roles {
		if r == want {
			return true
		}
	}
	return false
}

func hasAnyRole(have []string, want []string) bool {
	for _, w := range want {
		if HasRole(have, w) {
			return true
		}
	}
	return false
}

func bearerToken(c *gin.Context) string {
	const prefix = "Bearer "
	auth := c.GetHeader("Authorization")
	if len(auth) <= len(prefix) || auth[:len(prefix)] != prefix {
		return ""
	}
	return auth[len(prefix):]
}

func unauthorized(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusUnauthorized, apiv.ErrorVO{
		Error:   "invalid_token",
		Message: "Missing or invalid bearer token",
	})
}

func forbidden(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusForbidden, apiv.ErrorVO{
		Error:   "forbidden",
		Message: "Caller does not have a qualifying role",
	})
}
