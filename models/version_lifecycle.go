package models

import (
	"time"

	modelcore "github.com/sweetrpg/model-core.go/models"
)

// VersionLifecycle is the submission/review audit trail on a version record - state,
// provenance, and review outcome. Matches catalog-api's shared VersionLifecycle shape.
type VersionLifecycle struct {
	State            VersionState `bson:"state" json:"state"`
	BaseVersion      *int         `bson:"base_version,omitempty" json:"base_version,omitempty"`
	SubmittedBy      string       `bson:"submitted_by" json:"submitted_by"`
	SubmittedAt      time.Time    `bson:"submitted_at" json:"submitted_at"`
	ReviewedBy       *string      `bson:"reviewed_by,omitempty" json:"reviewed_by,omitempty"`
	ReviewedAt       *time.Time   `bson:"reviewed_at,omitempty" json:"reviewed_at,omitempty"`
	ReviewNote       *string      `bson:"review_note,omitempty" json:"review_note,omitempty"`
	ResultingVersion *int         `bson:"resulting_version,omitempty" json:"resulting_version,omitempty"`
}

// EntityMeta is the stable-identity record: id, the version currently live, and the platform
// audit fields (created/updated/deleted). The embedded modelcore.Auditable replaces the inline
// created_at/created_by this record used to carry - same bson keys, plus updated_* and the
// deleted_* soft-delete pair. Version rows keep VersionLifecycle rather than Auditable, matching
// the platform's versioned-entity pattern (catalog-objects version_lifecycle.go).
type EntityMeta struct {
	ID             string `bson:"_id" json:"id"`
	SystemID       string `bson:"system_id" json:"system_id"`
	CurrentVersion int    `bson:"current_version" json:"current_version"`

	modelcore.Auditable `bson:",inline"`
}
