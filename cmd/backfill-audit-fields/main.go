// Command backfill-audit-fields populates the platform audit fields (PADR-0001) on
// game_systems_meta records that predate the convention: updated_at / updated_by (mirrored from
// created_at / created_by), leaving deleted_at / deleted_by null. Version records are unchanged -
// their submission/review trail (VersionLifecycle) is their audit record.
//
// Idempotent - a record that already has updated_at is skipped. Dry run by default.
//
//	go run ./cmd/backfill-audit-fields          # report counts, write nothing
//	go run ./cmd/backfill-audit-fields -apply   # perform the writes
package main

import (
	"context"
	"flag"
	"time"

	"github.com/sweetrpg/common.go/logging"
	"github.com/sweetrpg/mongodb.go/database"
	"go.mongodb.org/mongo-driver/bson"
)

const metaCollection = "game_systems_meta"

// metaAudit is the subset of a meta record this backfill reads.
type metaAudit struct {
	ID        string     `bson:"_id"`
	CreatedAt time.Time  `bson:"created_at"`
	CreatedBy string     `bson:"created_by"`
	UpdatedAt *time.Time `bson:"updated_at"`
}

func main() {
	apply := flag.Bool("apply", false, "perform writes (default: dry run)")
	flag.Parse()

	logging.Init()
	c := context.Background()
	database.SetupDatabase()
	defer database.TeardownDatabase()

	mode := "DRY RUN"
	if *apply {
		mode = "APPLY"
	}

	// Idempotency guard: updated_at missing or null.
	filter := bson.D{{Key: "$or", Value: bson.A{
		bson.D{{Key: "updated_at", Value: bson.D{{Key: "$exists", Value: false}}}},
		bson.D{{Key: "updated_at", Value: nil}},
	}}}

	metas, err := database.Query[metaAudit](metaCollection, filter, nil, nil, 0, 0)
	if err != nil {
		logging.Logger.Error("failed to list meta records", "error", err.Error())
		return
	}

	var updated int
	for _, m := range metas {
		createdAt := m.CreatedAt
		if createdAt.IsZero() {
			createdAt = time.Now().UTC()
		}
		set := bson.D{
			{Key: "updated_at", Value: createdAt},
			{Key: "updated_by", Value: m.CreatedBy},
		}
		if m.CreatedAt.IsZero() {
			set = append(set, bson.E{Key: "created_at", Value: createdAt})
		}
		if !*apply {
			updated++
			continue
		}
		res, err := database.Db.Collection(metaCollection).UpdateOne(c,
			bson.D{{Key: "_id", Value: m.ID}},
			bson.D{{Key: "$set", Value: set}})
		if err != nil {
			logging.Logger.Error("failed to update meta record", "record", m.ID, "error", err.Error())
			return
		}
		updated += int(res.ModifiedCount)
	}

	logging.Logger.Info("backfill-audit-fields done", "mode", mode, "meta_records_stamped", updated)
}
