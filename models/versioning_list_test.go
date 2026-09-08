package models

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/sweetrpg/mongodb.go/database"
	"go.mongodb.org/mongo-driver/bson"
)

// insertLiveSystem seeds one live game system: a meta record plus a live version carrying name.
func insertLiveSystem(t *testing.T, recordID, name string) {
	t.Helper()
	insertMeta(t, recordID, false)
	now := time.Now()
	v := GameSystemVersion{
		ID: recordID + "-v1", RecordID: recordID, Version: 1, Name: name, Edition: "1e",
		VersionLifecycle: VersionLifecycle{
			State: VersionStateLive, SubmittedBy: "seed", SubmittedAt: now,
		},
	}
	if _, err := database.Db.Collection(versionCollection).InsertOne(context.Background(), v); err != nil {
		t.Fatalf("insert version %s: %v", recordID, err)
	}
}

func names(result *ListResult) []string {
	out := make([]string, len(result.Systems))
	for i, s := range result.Systems {
		out[i] = s.Name
	}
	return out
}

func TestListNoParamsReturnsFirstPageAndTotal(t *testing.T) {
	setupCountTestDB(t)
	for i := 1; i <= 30; i++ {
		insertLiveSystem(t, fmt.Sprintf("gs-%02d", i), fmt.Sprintf("System %02d", i))
	}

	got, err := List(context.Background(), ListParams{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if got.Total != 30 {
		t.Errorf("Total = %d, want 30", got.Total)
	}
	if got.Page != 1 || got.PerPage != listDefaultPerPage {
		t.Errorf("Page/PerPage = %d/%d, want 1/%d", got.Page, got.PerPage, listDefaultPerPage)
	}
	if len(got.Systems) != listDefaultPerPage {
		t.Fatalf("len(Systems) = %d, want %d", len(got.Systems), listDefaultPerPage)
	}
	if got.Systems[0].Name != "System 01" || got.Systems[listDefaultPerPage-1].Name != "System 24" {
		t.Errorf("default sort not name asc: first=%q last=%q", got.Systems[0].Name, got.Systems[listDefaultPerPage-1].Name)
	}
	// The audit block is grafted on from the joined meta, not left zero.
	if got.Systems[0].CreatedBy != "seed" {
		t.Errorf("CreatedBy = %q, want seed (meta audit block not carried through)", got.Systems[0].CreatedBy)
	}
}

func TestListSearchNarrowsAndCounts(t *testing.T) {
	setupCountTestDB(t)
	insertLiveSystem(t, "gs-1", "Alpha Quadrant")
	insertLiveSystem(t, "gs-2", "Alpine Strike")
	insertLiveSystem(t, "gs-3", "Beta Rising")

	got, err := List(context.Background(), ListParams{Search: "alp"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if got.Total != 2 || len(got.Systems) != 2 {
		t.Fatalf("search 'alp': Total=%d len=%d, want 2/2 (%v)", got.Total, len(got.Systems), names(got))
	}
	for _, s := range got.Systems {
		if s.Name != "Alpha Quadrant" && s.Name != "Alpine Strike" {
			t.Errorf("unexpected match %q", s.Name)
		}
	}
}

func TestListSearchNoMatchIsEmptyNotError(t *testing.T) {
	setupCountTestDB(t)
	insertLiveSystem(t, "gs-1", "Alpha")

	got, err := List(context.Background(), ListParams{Search: "nothing-matches"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if got.Total != 0 {
		t.Errorf("Total = %d, want 0", got.Total)
	}
	if got.Systems == nil || len(got.Systems) != 0 {
		t.Errorf("Systems = %v, want non-nil empty slice", got.Systems)
	}
}

func TestListSearchIsEscaped(t *testing.T) {
	setupCountTestDB(t)
	insertLiveSystem(t, "gs-1", "a.b")
	insertLiveSystem(t, "gs-2", "aXb")

	got, err := List(context.Background(), ListParams{Search: "a.b"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if got.Total != 1 || len(got.Systems) != 1 || got.Systems[0].Name != "a.b" {
		t.Fatalf("search 'a.b' should match only the literal: got %v (total %d)", names(got), got.Total)
	}
}

func TestListPageBeyondFirstReturnsCorrectSlice(t *testing.T) {
	setupCountTestDB(t)
	for i := 1; i <= 30; i++ {
		insertLiveSystem(t, fmt.Sprintf("gs-%02d", i), fmt.Sprintf("System %02d", i))
	}

	got, err := List(context.Background(), ListParams{Page: 2, PerPage: 10})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if got.Total != 30 {
		t.Errorf("Total = %d, want 30", got.Total)
	}
	if len(got.Systems) != 10 {
		t.Fatalf("len(Systems) = %d, want 10", len(got.Systems))
	}
	if got.Systems[0].Name != "System 11" || got.Systems[9].Name != "System 20" {
		t.Errorf("page 2 slice = %v, want System 11..System 20", names(got))
	}
}

func TestListExcludesSoftDeleted(t *testing.T) {
	setupCountTestDB(t)
	insertLiveSystem(t, "gs-live-1", "Live One")
	insertLiveSystem(t, "gs-live-2", "Live Two")
	// A live version whose meta record is soft-deleted must not appear.
	insertMeta(t, "gs-deleted", true)
	now := time.Now()
	dv := GameSystemVersion{
		ID: "gs-deleted-v1", RecordID: "gs-deleted", Version: 1, Name: "Deleted System", Edition: "1e",
		VersionLifecycle: VersionLifecycle{State: VersionStateLive, SubmittedBy: "seed", SubmittedAt: now},
	}
	if _, err := database.Db.Collection(versionCollection).InsertOne(context.Background(), dv); err != nil {
		t.Fatal(err)
	}

	got, err := List(context.Background(), ListParams{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if got.Total != 2 {
		t.Errorf("Total = %d, want 2", got.Total)
	}
	for _, s := range got.Systems {
		if s.Name == "Deleted System" {
			t.Errorf("soft-deleted system leaked into the list")
		}
	}
}

func TestListStateNameIndexCreatedAndUsed(t *testing.T) {
	setupCountTestDB(t)
	if err := EnsureIndexes(context.Background()); err != nil {
		t.Fatalf("EnsureIndexes: %v", err)
	}
	for i := 1; i <= 5; i++ {
		insertLiveSystem(t, fmt.Sprintf("gs-%02d", i), fmt.Sprintf("System %02d", i))
	}

	cur, err := database.Db.Collection(versionCollection).Indexes().List(context.Background())
	if err != nil {
		t.Fatalf("list indexes: %v", err)
	}
	var specs []bson.M
	if err := cur.All(context.Background(), &specs); err != nil {
		t.Fatalf("decode indexes: %v", err)
	}
	found := false
	for _, s := range specs {
		if s["name"] == "state_1_name_1" {
			found = true
		}
	}
	if !found {
		t.Fatalf("state_1_name_1 index not created; have %v", specs)
	}

	// explain the list aggregation's leading $match/$sort: it must resolve via an index scan,
	// not a full collection scan.
	explain := database.Db.RunCommand(context.Background(), bson.D{
		{Key: "explain", Value: bson.D{
			{Key: "aggregate", Value: versionCollection},
			{Key: "pipeline", Value: bson.A{
				bson.D{{Key: "$match", Value: bson.D{{Key: "state", Value: string(VersionStateLive)}}}},
				bson.D{{Key: "$sort", Value: bson.D{{Key: "name", Value: 1}}}},
			}},
			{Key: "cursor", Value: bson.D{}},
		}},
		{Key: "verbosity", Value: "queryPlanner"},
	})
	var raw bson.Raw
	if err := explain.Decode(&raw); err != nil {
		t.Fatalf("explain: %v", err)
	}
	if got := raw.String(); !strings.Contains(got, "IXSCAN") || strings.Contains(got, "COLLSCAN") {
		t.Fatalf("aggregation $match/$sort did not use an index scan:\n%s", got)
	}
}

func TestListSortAllowlistAndClamp(t *testing.T) {
	setupCountTestDB(t)
	for i := 1; i <= 5; i++ {
		insertLiveSystem(t, fmt.Sprintf("gs-%02d", i), fmt.Sprintf("System %02d", i))
	}

	cases := []struct {
		name       string
		params     ListParams
		wantErr    error
		wantFirst  string
		wantPerPag int
	}{
		{name: "default sort name asc", params: ListParams{}, wantFirst: "System 01", wantPerPag: 24},
		{name: "explicit name asc", params: ListParams{Sort: "name"}, wantFirst: "System 01", wantPerPag: 24},
		{name: "name desc", params: ListParams{Sort: "-name"}, wantFirst: "System 05", wantPerPag: 24},
		{name: "off-allowlist rejected", params: ListParams{Sort: "name; drop"}, wantErr: ErrInvalidSort},
		{name: "oversized per_page clamped", params: ListParams{PerPage: 500}, wantFirst: "System 01", wantPerPag: 100},
		{name: "zero per_page defaulted", params: ListParams{PerPage: 0}, wantFirst: "System 01", wantPerPag: 24},
		{name: "negative page clamped to 1", params: ListParams{Page: -3}, wantFirst: "System 01", wantPerPag: 24},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := List(context.Background(), tc.params)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			if got.PerPage != tc.wantPerPag {
				t.Errorf("PerPage = %d, want %d", got.PerPage, tc.wantPerPag)
			}
			if got.Page < 1 {
				t.Errorf("Page = %d, want >= 1", got.Page)
			}
			if len(got.Systems) == 0 || got.Systems[0].Name != tc.wantFirst {
				t.Errorf("first = %v, want %q", names(got), tc.wantFirst)
			}
		})
	}
}
