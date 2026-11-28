// Command backfill-canonical-user-ids rewrites pre-adoption Auth0-subject values in the
// game_systems_meta and game_systems_versions audit fields (created_by, updated_by,
// submitted_by, reviewed_by) to the canonical users._id each subject maps to, using users-api's
// internal resolve-subjects batch endpoint. Canonical ids, "system", unrecognized values and
// subjects that don't resolve are left unchanged. Dry-run by default; pass -apply to write. Both
// modes need an admin token.
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
	"regexp"
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

// isSubjectShaped reports whether v is an identity-provider subject ("<connection>|<id>" or
// "<client-id>@clients"). Only these are ever rewritten: canonical ids, "system" and anything
// unrecognized stay as they are.
func isSubjectShaped(v string) bool {
	return strings.Contains(v, "|") || strings.HasSuffix(v, "@clients")
}

// canonicalIDRE matches the two users._id forms: a UUID, or a 24-hex id.
var canonicalIDRE = regexp.MustCompile(`^([0-9a-f]{24}|[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})$`)

type store interface {
	distinct(ctx context.Context, t target) ([]string, error)
	count(ctx context.Context, t target, value string) (int64, error)
	rewrite(ctx context.Context, t target, from, to string) (int64, error)
}

type resolver func(ctx context.Context, subjects []string) (map[string]string, error)

type summary struct {
	subjects   int
	unresolved int
	// documents counts field rewrites made, or that would be made on a dry run.
	documents int64
}

func findSubjects(ctx context.Context, s store) ([]string, error) {
	seen := map[string]bool{}
	var subjects []string
	for _, t := range targets {
		values, err := s.distinct(ctx, t)
		if err != nil {
			// Driver errors can echo stored values, so they're not wrapped.
			return nil, fmt.Errorf("distinct failed on %s.%s", t.collection, t.field)
		}
		for _, v := range values {
			if isSubjectShaped(v) && !seen[v] {
				seen[v] = true
				subjects = append(subjects, v)
			}
		}
	}
	return subjects, nil
}

// planRewrites keeps only subjects that resolved; the rest are left untouched.
func planRewrites(subjects []string, resolved map[string]string) (map[string]string, error) {
	plan := map[string]string{}
	for _, s := range subjects {
		id, ok := resolved[s]
		if !ok {
			continue
		}
		if !canonicalIDRE.MatchString(id) {
			return nil, fmt.Errorf("resolver returned a non-canonical id")
		}
		plan[s] = id
	}
	return plan, nil
}

// run resolves everything before the first write, so a failed lookup changes nothing. A write
// error can leave a partial run, which is safe to repeat.
func run(ctx context.Context, s store, resolve resolver, apply bool) (summary, error) {
	var sum summary
	subjects, err := findSubjects(ctx, s)
	if err != nil || len(subjects) == 0 {
		return sum, err
	}
	sum.subjects = len(subjects)

	resolved, err := resolve(ctx, subjects)
	if err != nil {
		return sum, fmt.Errorf("resolve subjects: %w", err)
	}
	plan, err := planRewrites(subjects, resolved)
	if err != nil {
		return sum, err
	}
	sum.unresolved = len(subjects) - len(plan)

	for _, t := range targets {
		for from, to := range plan {
			var n int64
			if apply {
				n, err = s.rewrite(ctx, t, from, to)
			} else {
				n, err = s.count(ctx, t, from)
			}
			if err != nil {
				return sum, fmt.Errorf("query failed on %s.%s after %d documents", t.collection, t.field, sum.documents)
			}
			sum.documents += n
		}
	}
	return sum, nil
}

type mongoStore struct{}

func (mongoStore) distinct(ctx context.Context, t target) ([]string, error) {
	values, err := database.Db.Collection(t.collection).Distinct(ctx, t.field, bson.M{})
	if err != nil {
		return nil, err
	}
	var out []string
	for _, v := range values {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out, nil
}

func (mongoStore) count(ctx context.Context, t target, value string) (int64, error) {
	return database.Db.Collection(t.collection).CountDocuments(ctx, bson.M{t.field: value})
}

func (mongoStore) rewrite(ctx context.Context, t target, from, to string) (int64, error) {
	res, err := database.Db.Collection(t.collection).UpdateMany(ctx, bson.M{t.field: from}, bson.D{{Key: "$set", Value: bson.D{{Key: t.field, Value: to}}}})
	if err != nil {
		return 0, err
	}
	return res.ModifiedCount, nil
}

type resolveSubjectsRequest struct {
	Subjects []string `json:"subjects"`
}

// httpResolver calls users-api's resolve-subjects with an admin user's bearer token (PADR-0037).
func httpResolver(usersBaseURL, token string) resolver {
	return func(ctx context.Context, subjects []string) (map[string]string, error) {
		if token == "" {
			return nil, fmt.Errorf("-users-admin-token / USERS_ADMIN_TOKEN is required")
		}
		return resolveSubjects(ctx, usersBaseURL, token, subjects)
	}
}

func main() {
	os.Exit(realMain())
}

func realMain() int {
	logging.Init()

	apply := flag.Bool("apply", false, "write changes; default is a dry run")
	adminToken := flag.String("users-admin-token", os.Getenv("USERS_ADMIN_TOKEN"), "admin bearer token for users-api's internal resolve-subjects endpoint")
	flag.Parse()

	database.SetupDatabase()
	defer database.TeardownDatabase()

	sum, err := run(context.Background(), mongoStore{}, httpResolver(util.GetEnv(constants.USERS_API_URL, ""), *adminToken), *apply)
	fields := []any{"apply", *apply, "subjects", sum.subjects, "unresolved_left_unchanged", sum.unresolved, "documents", sum.documents}
	if err != nil {
		logging.Logger.Error("backfill: failed", append(fields, "error", err.Error())...)
		return 1
	}
	logging.Logger.Info("backfill: complete", fields...)
	return 0
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
