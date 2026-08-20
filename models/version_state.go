package models

// VersionState is one of a fixed set of states a version record can be in - matching
// catalog-api's version model exactly (see platform's catalog-entity-versioning spec).
type VersionState string

const (
	VersionStateSubmitted         VersionState = "submitted"
	VersionStateLive              VersionState = "live"
	VersionStateArchived          VersionState = "archived"
	VersionStateRejected          VersionState = "rejected"
	VersionStatePartiallyAccepted VersionState = "partially_accepted"
	VersionStateWithdrawn         VersionState = "withdrawn"
)
