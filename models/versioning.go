package models

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"time"

	modelcore "github.com/sweetrpg/model-core.go/models"
	"github.com/sweetrpg/mongodb.go/database"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	metaCollection    = "game_systems_meta"
	versionCollection = "game_systems_versions"

	// SystemIDPattern is the required format for a game system's system_id: lowercase
	// kebab-case slug.
	SystemIDPattern = `^[a-z0-9]+(-[a-z0-9]+)*$`
)

// ValidateSystemID reports whether s is a well-formed system_id slug.
func ValidateSystemID(s string) bool {
	matched, err := regexp.MatchString(SystemIDPattern, s)
	return err == nil && matched
}

// EnsureIndexes creates the indexes game system version queries rely on. Safe to call on every
// startup.
func EnsureIndexes(c context.Context) error {
	_, err := database.Db.Collection(metaCollection).Indexes().CreateOne(c, mongo.IndexModel{
		Keys:    bson.D{{Key: "system_id", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return fmt.Errorf("game system: create system_id index: %w", err)
	}
	_, err = database.Db.Collection(versionCollection).Indexes().CreateOne(c, mongo.IndexModel{
		Keys:    bson.D{{Key: "record_id", Value: 1}, {Key: "version", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return fmt.Errorf("game system: create record_id+version index: %w", err)
	}
	_, err = database.Db.Collection(versionCollection).Indexes().CreateOne(c, mongo.IndexModel{
		Keys: bson.D{{Key: "record_id", Value: 1}, {Key: "state", Value: 1}},
	})
	if err != nil {
		return fmt.Errorf("game system: create record_id+state index: %w", err)
	}
	// Covers the List aggregation's leading stages: $match state=live then $sort by name.
	_, err = database.Db.Collection(versionCollection).Indexes().CreateOne(c, mongo.IndexModel{
		Keys: bson.D{{Key: "state", Value: 1}, {Key: "name", Value: 1}},
	})
	if err != nil {
		return fmt.Errorf("game system: create state+name index: %w", err)
	}
	return nil
}

// notDeletedMeta excludes soft-deleted meta records (deleted_at absent or null) - the platform
// audit-fields read filter (PADR-0001).
var notDeletedMeta = bson.D{{Key: "deleted_at", Value: nil}}

// GetMeta fetches a live (non-soft-deleted) game system's meta record, matching id against
// either the document `_id` or the `system_id` slug.
func GetMeta(c context.Context, id string) (*EntityMeta, error) {
	filter := bson.D{
		{Key: "deleted_at", Value: nil},
		{Key: "$or", Value: bson.A{
			bson.D{{Key: "_id", Value: id}},
			bson.D{{Key: "system_id", Value: id}},
		}},
	}
	results, err := database.Query[EntityMeta](metaCollection, filter, nil, nil, 0, 1)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, nil
	}
	return results[0], nil
}

// GetVersion fetches one game system version's full field snapshot.
func GetVersion(c context.Context, recordID string, version int) (*GameSystemVersion, error) {
	filter := bson.D{{Key: "record_id", Value: recordID}, {Key: "version", Value: version}}
	results, err := database.Query[GameSystemVersion](versionCollection, filter, nil, nil, 0, 1)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, nil
	}
	results[0].EnsureTags()
	return results[0], nil
}

// getVersionByID fetches a version snapshot by its own document `_id`.
func getVersionByID(c context.Context, id string) (*GameSystemVersion, error) {
	results, err := database.Query[GameSystemVersion](versionCollection, bson.D{{Key: "_id", Value: id}}, nil, nil, 0, 1)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, nil
	}
	results[0].EnsureTags()
	return results[0], nil
}

// Get returns a game system's current version flattened with its stable record's audit block -
// the view GET /systems/:id returns. id may be the meta document `_id`, the `system_id` slug, or
// a version document's own `_id` (the detail payload exposes that as its `id`, so a client that
// round-trips it must resolve).
func Get(c context.Context, id string) (*GameSystemView, error) {
	meta, err := GetMeta(c, id)
	if err != nil {
		return nil, err
	}
	if meta == nil {
		// Not a meta _id or system_id - try it as a version document's own _id.
		version, err := getVersionByID(c, id)
		if err != nil {
			return nil, err
		}
		if version == nil {
			return nil, nil
		}
		meta, err = GetMeta(c, version.RecordID)
		if err != nil {
			return nil, err
		}
		if meta == nil {
			return nil, nil
		}
	}
	version, err := GetVersion(c, meta.ID, meta.CurrentVersion)
	if err != nil || version == nil {
		return nil, err
	}
	return &GameSystemView{GameSystemVersion: *version, Auditable: meta.Auditable}, nil
}

// ListParams controls GET /systems: a case-insensitive name-substring search, a sort key drawn
// from listSortKeys, and 1-based pagination. The zero value lists page 1 at the default sort and
// page size.
type ListParams struct {
	Search  string
	Sort    string
	Page    int
	PerPage int
}

// ListResult is one page of List output plus the total count of systems matching Search,
// independent of the page window.
type ListResult struct {
	Systems []*GameSystemView
	Total   int
	Page    int
	PerPage int
}

// ErrInvalidSort is returned by List when ListParams.Sort is not one of listSortKeys.
var ErrInvalidSort = errors.New("game system: unsupported sort key")

const (
	listDefaultPerPage = 24
	listMaxPerPage     = 100
)

// listSortKeys is the sort allowlist: it maps each accepted sort parameter to its aggregation
// sort document, so the sort string can't inject an arbitrary field path. "created" orders on
// the stable record's creation time, carried on the joined meta.
var listSortKeys = map[string]bson.D{
	"":         {{Key: "name", Value: 1}},
	"name":     {{Key: "name", Value: 1}},
	"-name":    {{Key: "name", Value: -1}},
	"created":  {{Key: "meta.created_at", Value: 1}},
	"-created": {{Key: "meta.created_at", Value: -1}},
}

// listRow decodes one $facet page element: the version document inline, plus its joined meta.
type listRow struct {
	GameSystemVersion `bson:",inline"`
	Meta              EntityMeta `bson:"meta"`
}

// List returns one page of live game systems' current views - optionally filtered by a
// case-insensitive name search, ordered by params.Sort - together with the total match count.
// The search, sort, soft-delete exclusion, skip/limit, and count are all evaluated by a single
// MongoDB aggregation on game_systems_versions. The meta-side soft-delete filter is why this is
// a $lookup rather than a version-only skip/limit (design.md decision 1). An out-of-range Page
// or PerPage is clamped, not rejected; an unknown Sort returns ErrInvalidSort.
func List(c context.Context, params ListParams) (*ListResult, error) {
	sortKey, ok := listSortKeys[params.Sort]
	if !ok {
		return nil, ErrInvalidSort
	}

	page := params.Page
	if page < 1 {
		page = 1
	}
	perPage := params.PerPage
	switch {
	case perPage < 1:
		perPage = listDefaultPerPage
	case perPage > listMaxPerPage:
		perPage = listMaxPerPage
	}

	match := bson.D{{Key: "state", Value: string(VersionStateLive)}}
	if params.Search != "" {
		match = append(match, bson.E{Key: "name", Value: primitive.Regex{
			Pattern: regexp.QuoteMeta(params.Search), Options: "i",
		}})
	}

	// Stable order: the requested key, then _id as a tiebreaker so paging can't skip or repeat
	// rows that share a sort value.
	pageSort := make(bson.D, 0, len(sortKey)+1)
	pageSort = append(pageSort, sortKey...)
	pageSort = append(pageSort, bson.E{Key: "_id", Value: 1})

	pipeline := mongo.Pipeline{
		bson.D{{Key: "$match", Value: match}},
		bson.D{{Key: "$lookup", Value: bson.D{
			{Key: "from", Value: metaCollection},
			{Key: "localField", Value: "record_id"},
			{Key: "foreignField", Value: "_id"},
			{Key: "as", Value: "meta"},
		}}},
		bson.D{{Key: "$unwind", Value: "$meta"}},
		bson.D{{Key: "$match", Value: bson.D{{Key: "meta.deleted_at", Value: nil}}}},
		bson.D{{Key: "$facet", Value: bson.D{
			{Key: "page", Value: bson.A{
				bson.D{{Key: "$sort", Value: pageSort}},
				bson.D{{Key: "$skip", Value: int64(page-1) * int64(perPage)}},
				bson.D{{Key: "$limit", Value: int64(perPage)}},
			}},
			{Key: "total", Value: bson.A{
				bson.D{{Key: "$count", Value: "n"}},
			}},
		}}},
	}

	cur, err := database.Db.Collection(versionCollection).Aggregate(c, pipeline)
	if err != nil {
		return nil, err
	}
	defer func() { _ = cur.Close(c) }()

	var facets []struct {
		Page  []listRow `bson:"page"`
		Total []struct {
			N int `bson:"n"`
		} `bson:"total"`
	}
	if err := cur.All(c, &facets); err != nil {
		return nil, err
	}

	result := &ListResult{Systems: []*GameSystemView{}, Page: page, PerPage: perPage}
	if len(facets) == 0 {
		return result, nil
	}
	if len(facets[0].Total) > 0 {
		result.Total = facets[0].Total[0].N
	}
	for i := range facets[0].Page {
		row := facets[0].Page[i]
		view := &GameSystemView{
			GameSystemVersion: row.GameSystemVersion,
			Auditable:         row.Meta.Auditable,
		}
		view.EnsureTags()
		result.Systems = append(result.Systems, view)
	}
	return result, nil
}

// ListVersions returns every version of a game system, newest first.
func ListVersions(c context.Context, id string) ([]*GameSystemVersion, error) {
	filter := bson.D{{Key: "record_id", Value: id}}
	sortOrder := bson.D{{Key: "version", Value: -1}}
	results, err := database.Query[GameSystemVersion](versionCollection, filter, sortOrder, nil, 0, 0)
	if err != nil {
		return nil, err
	}
	for _, v := range results {
		v.EnsureTags()
	}
	return results, nil
}

func setVersionState(c context.Context, recordID string, version int, fields bson.D) error {
	filter := bson.D{{Key: "record_id", Value: recordID}, {Key: "version", Value: version}}
	_, err := database.Db.Collection(versionCollection).UpdateOne(c, filter, bson.D{{Key: "$set", Value: fields}})
	return err
}

func archiveVersion(c context.Context, recordID string, version int) error {
	return setVersionState(c, recordID, version, bson.D{{Key: "state", Value: string(VersionStateArchived)}})
}

func setMetaCurrentVersion(c context.Context, recordID string, version int, actingUserID string) error {
	_, err := database.Db.Collection(metaCollection).UpdateOne(
		c,
		bson.D{{Key: "_id", Value: recordID}},
		bson.D{{Key: "$set", Value: bson.D{
			{Key: "current_version", Value: version},
			{Key: "updated_at", Value: time.Now()},
			{Key: "updated_by", Value: actingUserID},
		}}},
	)
	return err
}

func nextVersionNumber(c context.Context, recordID string) (int, error) {
	filter := bson.D{{Key: "record_id", Value: recordID}}
	sortOrder := bson.D{{Key: "version", Value: -1}}
	results, err := database.Query[GameSystemVersion](versionCollection, filter, sortOrder, nil, 0, 1)
	if err != nil {
		return 0, err
	}
	if len(results) == 0 {
		return 1, nil
	}
	return results[0].Version + 1, nil
}

// Create adds a new game system: its meta record (with systemID) and first (live) version.
func Create(c context.Context, gs *GameSystemVersion, systemID string, createdBy string) (*string, error) {
	now := time.Now()
	metaID := primitive.NewObjectID().Hex()
	meta := EntityMeta{ID: metaID, SystemID: systemID, CurrentVersion: 1}
	modelcore.StampCreate(&meta.Auditable, createdBy, now)
	if _, err := database.Insert[EntityMeta](metaCollection, meta); err != nil {
		return nil, err
	}

	gs.ID = primitive.NewObjectID().Hex()
	gs.RecordID = metaID
	gs.Version = 1
	gs.State = VersionStateLive
	gs.BaseVersion = nil
	gs.SubmittedBy = createdBy
	gs.SubmittedAt = now
	gs.EnsureTags()

	if _, err := database.Insert[GameSystemVersion](versionCollection, *gs); err != nil {
		return nil, err
	}
	return &metaID, nil
}

// CreateVersion creates a new version for an existing game system - editor/admin: state Live,
// goes current immediately and archives the previous current version; submitter: state
// Submitted, current pointer untouched.
func CreateVersion(c context.Context, id string, gs *GameSystemVersion, state VersionState, submittedBy string) (*GameSystemVersion, error) {
	meta, err := GetMeta(c, id)
	if err != nil {
		return nil, err
	}
	if meta == nil {
		return nil, nil
	}
	// id may have arrived as the system_id slug; every version/meta write below keys on the
	// canonical _id, which is also what record_id holds.
	id = meta.ID

	nextVersion, err := nextVersionNumber(c, id)
	if err != nil {
		return nil, err
	}

	baseVersion := meta.CurrentVersion
	gs.ID = primitive.NewObjectID().Hex()
	gs.RecordID = id
	gs.Version = nextVersion
	gs.State = state
	gs.BaseVersion = &baseVersion
	gs.SubmittedBy = submittedBy
	gs.SubmittedAt = time.Now()
	gs.EnsureTags()

	if _, err := database.Insert[GameSystemVersion](versionCollection, *gs); err != nil {
		return nil, err
	}

	if state == VersionStateLive {
		if err := archiveVersion(c, id, meta.CurrentVersion); err != nil {
			return nil, err
		}
		if err := setMetaCurrentVersion(c, id, nextVersion, submittedBy); err != nil {
			return nil, err
		}
	}

	return gs, nil
}

// fieldValue/setFieldValue/changedFields back AcceptVersion's selected-fields diff/apply logic -
// the same substantive fields a PATCH can change.
func fieldValue(v *GameSystemVersion, field string) any {
	switch field {
	case "name":
		return v.Name
	case "edition":
		return v.Edition
	case "publisher_id":
		return v.PublisherID
	case "notes":
		return v.Notes
	case "tags":
		return v.Tags
	default:
		return nil
	}
}

func setFieldValue(v *GameSystemVersion, field string, value any) {
	switch field {
	case "name":
		v.Name = value.(string)
	case "edition":
		v.Edition = value.(string)
	case "publisher_id":
		v.PublisherID = value.(string)
	case "notes":
		v.Notes = value.(string)
	case "tags":
		v.Tags = value.([]modelcore.Tag)
	}
}

var patchableFields = []string{"name", "edition", "publisher_id", "notes", "tags"}

func changedFields(submitted, base *GameSystemVersion) []string {
	var changed []string
	for _, field := range patchableFields {
		if !reflect.DeepEqual(fieldValue(submitted, field), fieldValue(base, field)) {
			changed = append(changed, field)
		}
	}
	sort.Strings(changed)
	return changed
}

// AcceptVersion reviews a submitted version - accept all changed fields (selectedFields nil) or
// only the ones named. A conflict is a selected field whose current value has diverged from the
// submission's base since it was submitted; conflicting fields are reported, not silently
// overwritten.
func AcceptVersion(c context.Context, id string, version int, selectedFields []string, reviewedBy string, reviewNote *string) (*GameSystemVersion, []string, error) {
	submitted, err := GetVersion(c, id, version)
	if err != nil {
		return nil, nil, err
	}
	if submitted == nil {
		return nil, nil, fmt.Errorf("game system %s: version %d not found", id, version)
	}
	if submitted.State != VersionStateSubmitted {
		return nil, nil, fmt.Errorf("game system %s: version %d is not submitted (state: %s)", id, version, submitted.State)
	}
	if submitted.BaseVersion == nil {
		return nil, nil, fmt.Errorf("game system %s: version %d has no base version to review against", id, version)
	}

	meta, err := GetMeta(c, id)
	if err != nil {
		return nil, nil, err
	}
	if meta == nil {
		return nil, nil, fmt.Errorf("game system %s: meta record not found", id)
	}
	id = meta.ID // normalize a system_id slug to the canonical _id

	current, err := GetVersion(c, id, meta.CurrentVersion)
	if err != nil {
		return nil, nil, err
	}
	if current == nil {
		return nil, nil, fmt.Errorf("game system %s: current version %d not found", id, meta.CurrentVersion)
	}

	now := time.Now()

	if selectedFields == nil && *submitted.BaseVersion == meta.CurrentVersion {
		if err := archiveVersion(c, id, meta.CurrentVersion); err != nil {
			return nil, nil, err
		}
		if err := setVersionState(c, id, version, bson.D{
			{Key: "state", Value: string(VersionStateLive)},
			{Key: "reviewed_by", Value: reviewedBy},
			{Key: "reviewed_at", Value: now},
			{Key: "review_note", Value: reviewNote},
		}); err != nil {
			return nil, nil, err
		}
		if err := setMetaCurrentVersion(c, id, version, reviewedBy); err != nil {
			return nil, nil, err
		}
		submitted.State = VersionStateLive
		return submitted, nil, nil
	}

	baseVersion, err := GetVersion(c, id, *submitted.BaseVersion)
	if err != nil {
		return nil, nil, err
	}
	if baseVersion == nil {
		return nil, nil, fmt.Errorf("game system %s: base version %d not found", id, *submitted.BaseVersion)
	}

	changed := changedFields(submitted, baseVersion)
	target := changed
	if selectedFields != nil {
		selectedSet := make(map[string]bool, len(selectedFields))
		for _, f := range selectedFields {
			selectedSet[f] = true
		}
		var filtered []string
		for _, f := range changed {
			if selectedSet[f] {
				filtered = append(filtered, f)
			}
		}
		target = filtered
	}

	derived := *current
	var conflicts []string
	for _, field := range target {
		if !reflect.DeepEqual(fieldValue(current, field), fieldValue(baseVersion, field)) {
			conflicts = append(conflicts, field)
			continue
		}
		setFieldValue(&derived, field, fieldValue(submitted, field))
	}

	nextVersion, err := nextVersionNumber(c, id)
	if err != nil {
		return nil, nil, err
	}

	derived.ID = primitive.NewObjectID().Hex()
	derived.RecordID = id
	derived.Version = nextVersion
	derived.State = VersionStateLive
	derived.BaseVersion = &meta.CurrentVersion
	derived.SubmittedBy = submitted.SubmittedBy
	derived.SubmittedAt = now
	derived.ReviewedBy = &reviewedBy
	derived.ReviewedAt = &now
	derived.ReviewNote = reviewNote
	derived.ResultingVersion = nil

	if _, err := database.Insert[GameSystemVersion](versionCollection, derived); err != nil {
		return nil, nil, err
	}
	if err := archiveVersion(c, id, meta.CurrentVersion); err != nil {
		return nil, nil, err
	}
	if err := setMetaCurrentVersion(c, id, nextVersion, reviewedBy); err != nil {
		return nil, nil, err
	}
	if err := setVersionState(c, id, version, bson.D{
		{Key: "state", Value: string(VersionStatePartiallyAccepted)},
		{Key: "reviewed_by", Value: reviewedBy},
		{Key: "reviewed_at", Value: now},
		{Key: "review_note", Value: reviewNote},
		{Key: "resulting_version", Value: nextVersion},
	}); err != nil {
		return nil, nil, err
	}

	return &derived, conflicts, nil
}

// RejectVersion marks a submitted version rejected, with an optional note.
func RejectVersion(c context.Context, id string, version int, reviewedBy string, reviewNote *string) error {
	submitted, err := GetVersion(c, id, version)
	if err != nil {
		return err
	}
	if submitted == nil {
		return fmt.Errorf("game system %s: version %d not found", id, version)
	}
	if submitted.State != VersionStateSubmitted {
		return fmt.Errorf("game system %s: version %d is not submitted", id, version)
	}
	now := time.Now()
	return setVersionState(c, id, version, bson.D{
		{Key: "state", Value: string(VersionStateRejected)},
		{Key: "reviewed_by", Value: reviewedBy},
		{Key: "reviewed_at", Value: now},
		{Key: "review_note", Value: reviewNote},
	})
}

// RetractVersion lets the original submitter withdraw their own pending submission.
func RetractVersion(c context.Context, id string, version int, submitterID string) (*GameSystemVersion, error) {
	submitted, err := GetVersion(c, id, version)
	if err != nil {
		return nil, err
	}
	if submitted == nil {
		return nil, fmt.Errorf("game system %s: version %d not found", id, version)
	}
	if submitted.State != VersionStateSubmitted {
		return nil, fmt.Errorf("game system %s: version %d is not submitted", id, version)
	}
	if submitted.SubmittedBy != submitterID {
		return nil, fmt.Errorf("game system %s: version %d was not submitted by %s", id, version, submitterID)
	}
	if err := setVersionState(c, id, version, bson.D{{Key: "state", Value: string(VersionStateWithdrawn)}}); err != nil {
		return nil, err
	}
	submitted.State = VersionStateWithdrawn
	return submitted, nil
}

// SetCurrentVersion rolls a game system back (or forward) to an arbitrary existing version.
func SetCurrentVersion(c context.Context, id string, version int, actingUserID string) (*GameSystemVersion, error) {
	meta, err := GetMeta(c, id)
	if err != nil {
		return nil, err
	}
	if meta == nil {
		return nil, nil
	}
	id = meta.ID // normalize a system_id slug to the canonical _id

	target, err := GetVersion(c, id, version)
	if err != nil {
		return nil, err
	}
	if target == nil {
		return nil, fmt.Errorf("game system %s: version %d not found", id, version)
	}
	if version == meta.CurrentVersion {
		return target, nil
	}
	if err := archiveVersion(c, id, meta.CurrentVersion); err != nil {
		return nil, err
	}
	if err := setVersionState(c, id, version, bson.D{{Key: "state", Value: string(VersionStateLive)}}); err != nil {
		return nil, err
	}
	if err := setMetaCurrentVersion(c, id, version, actingUserID); err != nil {
		return nil, err
	}
	target.State = VersionStateLive
	return target, nil
}
