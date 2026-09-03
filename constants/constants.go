package constants

// ServiceName identifies this service to auth-api's /authz/check and to tracing/telemetry.
const ServiceName = "game-systems-api"

// Environment variable names specific to this service.
const (
	AUTH_API_URL = "AUTH_API_URL"
	// USERS_API_URL points at users-api's base URL, used to resolve a verified subject to its
	// canonical users._id for the audit fields.
	USERS_API_URL = "USERS_API_URL"
)
