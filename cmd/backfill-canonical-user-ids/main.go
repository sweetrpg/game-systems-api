// Command backfill-canonical-user-ids rewrites pre-adoption Auth0-subject values in the
// game_systems_meta and game_systems_versions audit fields (created_by, updated_by,
// submitted_by, reviewed_by) to the canonical users._id each subject maps to, using users-api's
// internal resolve-subjects batch endpoint. Subjects that cannot be resolved become the
// "system" actor. Idempotent: a value that is not subject-shaped (no "|", not "<id>@clients")
// is left alone, so "system" and canonical users._id values are skipped and re-runs are safe.
// Dry-run by default; pass -apply to write.
//
//	go run ./cmd/backfill-canonical-user-ids          # report counts, write nothing
//	go run ./cmd/backfill-canonical-user-ids -apply   # perform the writes
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/sweetrpg/common.go/logging"
	"github.com/sweetrpg/common.go/util"
	"github.com/sweetrpg/game-systems-api/constants"
	"github.com/sweetrpg/mongodb.go/database"
	"go.mongodb.org/mongo-driver/bson"
)

type target struct {
	collection string
	field      string
}

var targets = []target{
	{"game_systems_meta", "created_by"},
	{"game_systems_meta", "updated_by"},
	{"game_systems_versions", "submitted_by"},
	{"game_systems_versions", "reviewed_by"},
}

// isSubjectShaped reports whether v looks like a third-party IdP subject rather than a canonical
// users._id or the "system" sentinel. Auth0 subjects are "<connection>|<id>" (e.g. "auth0|abc",
// "github|419457") or "<client-id>@clients" for M2M tokens; users._id is a bare UUID.
func isSubjectShaped(v string) bool {
	if v == "" || v == "system" {
		return false
	}
	return strings.Contains(v, "|") || strings.HasSuffix(v, "@clients")
}

// planRewrites maps each subject to its resolved users._id, falling back to "system" when
// users-api could not resolve it.
func planRewrites(subjects []string, resolved map[string]string) map[string]string {
	out := make(map[string]string, len(subjects))
	for _, s := range subjects {
		if id := resolved[s]; id != "" {
			out[s] = id
		} else {
			out[s] = "system"
		}
	}
	return out
}

type resolveSubjectsRequest struct {
	Subjects []string `json:"subjects"`
}

func main() {
	logging.Init()

	apply := flag.Bool("apply", false, "write changes; default is a dry run")
	adminToken := flag.String("users-admin-token", os.Getenv("USERS_ADMIN_TOKEN"), "admin bearer token for users-api's internal resolve-subjects endpoint")
	flag.Parse()

	database.SetupDatabase()
	defer database.TeardownDatabase()

	usersBaseURL := util.GetEnv(constants.USERS_API_URL, "")
	ctx := context.Background()

	distinct := map[string]struct{}{}
	for _, t := range targets {
		values, err := database.Db.Collection(t.collection).Distinct(ctx, t.field, bson.M{})
		if err != nil {
			logging.Logger.Error("backfill: distinct failed", "collection", t.collection, "field", t.field, "error", err.Error())
			return
		}
		for _, v := range values {
			if s, ok := v.(string); ok && isSubjectShaped(s) {
				distinct[s] = struct{}{}
			}
		}
	}
	if len(distinct) == 0 {
		logging.Logger.Info("backfill: no subject-shaped values found; nothing to do")
		return
	}

	subjects := make([]string, 0, len(distinct))
	for s := range distinct {
		subjects = append(subjects, s)
	}
	logging.Logger.Info("backfill: distinct subjects found", "count", len(subjects))

	if *adminToken == "" {
		logging.Logger.Error("backfill: -users-admin-token / USERS_ADMIN_TOKEN is required to resolve subjects")
		return
	}
	resolved, err := resolveSubjects(ctx, usersBaseURL, *adminToken, subjects)
	if err != nil {
		logging.Logger.Error("backfill: resolve subjects failed", "error", err.Error())
		return
	}
	rewrites := planRewrites(subjects, resolved)
	for s, id := range rewrites {
		if id == "system" {
			logging.Logger.Warn("backfill: unmappable subject -> system", "subject", s)
		}
	}

	var updated int64
	for _, t := range targets {
		coll := database.Db.Collection(t.collection)
		for oldValue, newValue := range rewrites {
			if *apply {
				res, err := coll.UpdateMany(ctx, bson.M{t.field: oldValue}, bson.D{{Key: "$set", Value: bson.D{{Key: t.field, Value: newValue}}}})
				if err != nil {
					logging.Logger.Error("backfill: update failed", "collection", t.collection, "field", t.field, "value", oldValue, "error", err.Error())
					return
				}
				if res.ModifiedCount > 0 {
					logging.Logger.Info("backfill: updated", "collection", t.collection, "field", t.field, "value", oldValue, "replacement", newValue, "documents", res.ModifiedCount)
				}
				updated += res.ModifiedCount
				continue
			}
			count, err := coll.CountDocuments(ctx, bson.M{t.field: oldValue})
			if err != nil {
				logging.Logger.Error("backfill: count failed", "collection", t.collection, "field", t.field, "value", oldValue, "error", err.Error())
				return
			}
			if count > 0 {
				logging.Logger.Info("backfill: would update", "collection", t.collection, "field", t.field, "value", oldValue, "replacement", newValue, "documents", count)
			}
			updated += count
		}
	}

	if *apply {
		logging.Logger.Info("backfill: complete", "documents_updated", updated)
	} else {
		fmt.Printf("dry run complete - %d documents would change; pass -apply to write\n", updated)
	}
}

func resolveSubjects(ctx context.Context, usersBaseURL, token string, subjects []string) (map[string]string, error) {
	if usersBaseURL == "" {
		return nil, fmt.Errorf("%s is not set", constants.USERS_API_URL)
	}

	body, err := json.Marshal(resolveSubjectsRequest{Subjects: subjects})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, usersBaseURL+"/internal/resolve-subjects", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := (&http.Client{Timeout: 60 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("resolve-subjects returned %s", resp.Status)
	}

	out := map[string]string{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}
