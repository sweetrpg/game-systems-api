package models

import modelcore "github.com/sweetrpg/model-core.go/models"

// GameSystemVersion is one full-snapshot version of a game system's substantive fields, plus
// the shared submission/review audit trail (VersionLifecycle).
type GameSystemVersion struct {
	ID               string          `bson:"_id" json:"id"`
	RecordID         string          `bson:"record_id" json:"record_id"`
	Version          int             `bson:"version" json:"version"`
	Name             string          `bson:"name" json:"name"`
	Edition          string          `bson:"edition" json:"edition"`
	PublisherID      string          `bson:"publisher_id,omitempty" json:"publisher_id,omitempty"`
	Notes            string          `bson:"notes" json:"notes"`
	Tags             []modelcore.Tag `bson:"tags" json:"tags"`
	VersionLifecycle `bson:",inline"`
}
