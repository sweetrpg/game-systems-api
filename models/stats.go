package models

import (
	"context"

	"github.com/sweetrpg/mongodb.go/database"
)

// CountGameSystems returns the number of live game systems - one per distinct system, matching
// what List returns and what catalog-api's /stats reports per entity type. Soft-deleted records
// (deleted_at set on the meta record, per PADR-0001) are excluded.
func CountGameSystems(c context.Context) (int64, error) {
	return database.Db.Collection(metaCollection).CountDocuments(c, notDeletedMeta)
}
