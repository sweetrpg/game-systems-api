package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strings"

	"github.com/sweetrpg/common.go/logging"
	"github.com/sweetrpg/game-systems-api/models"
	"github.com/sweetrpg/mongodb.go/database"
	"go.mongodb.org/mongo-driver/bson"
)

// One-off backfill: for every game system meta record missing system_id, slugify the name on
// its current version record and set it as system_id. Collisions are resolved by appending
// -2, -3, ... and each resolved collision is logged for manual review.
//
// Run AFTER deploying the service code but BEFORE the unique index is expected to hold:
//
//	go run ./cmd/migrate-system-ids
const (
	metaCollection    = "game_systems_meta"
	versionCollection = "game_systems_versions"
)

// versionName is just the name field off a game system version record.
type versionName struct {
	Name string `bson:"name"`
}

func main() {
	logging.Init()

	c := context.Background()
	database.SetupDatabase()
	defer database.TeardownDatabase()

	metas, err := database.Query[models.EntityMeta](metaCollection, bson.D{}, nil, nil, 0, 0)
	if err != nil {
		logging.Logger.Error("failed to list meta records", "error", err.Error())
		os.Exit(1)
	}

	taken := map[string]bool{}
	for _, meta := range metas {
		if meta.SystemID != "" {
			taken[meta.SystemID] = true
		}
	}

	var updated int
	for _, meta := range metas {
		if meta.SystemID != "" {
			continue
		}
		version, err := getVersion(c, meta.ID, meta.CurrentVersion)
		if err != nil || version == nil {
			logging.Logger.Error("skipping record: cannot load current version", "record", meta.ID, "error", fmt.Sprint(err))
			continue
		}
		base := slugify(version.Name)
		systemID := base
		for n := 2; taken[systemID]; n++ {
			systemID = fmt.Sprintf("%s-%d", base, n)
			logging.Logger.Warn("system_id collision resolved with suffix; rename manually if needed",
				"record", meta.ID, "name", version.Name, "system_id", systemID)
		}

		filter := bson.D{{Key: "_id", Value: meta.ID}, {Key: "$or", Value: bson.A{
			bson.D{{Key: "system_id", Value: ""}},
			bson.D{{Key: "system_id", Value: bson.M{"$exists": false}}},
		}}}
		res, err := database.Db.Collection(metaCollection).UpdateOne(c, filter,
			bson.D{{Key: "$set", Value: bson.D{{Key: "system_id", Value: systemID}}}})
		if err != nil {
			logging.Logger.Error("failed to update record", "record", meta.ID, "error", err.Error())
			os.Exit(1)
		}
		if res.MatchedCount == 1 {
			taken[systemID] = true
			updated++
		}
	}

	logging.Logger.Info("backfill complete", "updated", updated)
}

func getVersion(c context.Context, recordID string, version int) (*versionName, error) {
	filter := bson.D{{Key: "record_id", Value: recordID}, {Key: "version", Value: version}}
	results, err := database.Query[versionName](versionCollection, filter, nil, nil, 0, 1)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, nil
	}
	return results[0], nil
}

var (
	nonAlnum   = regexp.MustCompile(`[^a-z0-9]+`)
	edgeDashes = regexp.MustCompile(`^-+|-+$`)
)

func slugify(s string) string {
	lower := strings.ToLower(s)
	slug := nonAlnum.ReplaceAllString(lower, "-")
	slug = edgeDashes.ReplaceAllString(slug, "")
	if slug == "" {
		slog.Warn("slugify produced an empty slug", "input", s)
	}
	return slug
}
