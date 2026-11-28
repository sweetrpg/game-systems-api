package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

const (
	uuidID   = "b3384f5d-78c1-4965-8112-37395c2b8ef3"
	legacy   = "auth0|known"
	orphaned = "auth0|missing"
)

type fakeStore struct {
	values   []string
	failOn   string
	rewrites map[string]string
}

func (f *fakeStore) distinct(context.Context, target) ([]string, error) {
	if f.failOn == "distinct" {
		return nil, errors.New("boom")
	}
	return f.values, nil
}

func (f *fakeStore) count(context.Context, target, string) (int64, error) {
	if f.failOn == "count" {
		return 0, errors.New("boom")
	}
	return 1, nil
}

func (f *fakeStore) rewrite(_ context.Context, _ target, from, to string) (int64, error) {
	if f.rewrites == nil {
		f.rewrites = map[string]string{}
	}
	f.rewrites[from] = to
	return 1, nil
}

func resolverReturning(out map[string]string, err error) resolver {
	return func(context.Context, []string) (map[string]string, error) { return out, err }
}

func allValues() []string {
	return []string{"", "system", uuidID, "0123456789abcdef01234567", "unrecognized", legacy, orphaned}
}

func TestIsSubjectShaped(t *testing.T) {
	cases := map[string]bool{
		"": false, "system": false, uuidID: false, "0123456789abcdef01234567": false,
		"unrecognized": false, "auth0|abc": true, "github|1": true, "abc@clients": true,
	}
	for in, want := range cases {
		if got := isSubjectShaped(in); got != want {
			t.Errorf("isSubjectShaped(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestRunApplyRewritesOnlyResolvedSubjects(t *testing.T) {
	s := &fakeStore{values: allValues()}
	_, err := run(context.Background(), s, resolverReturning(map[string]string{legacy: uuidID}, nil), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.rewrites) != 1 || s.rewrites[legacy] != uuidID {
		t.Fatalf("want only the resolved subject rewritten, got %v", s.rewrites)
	}
}

func TestRunReportsUnresolved(t *testing.T) {
	s := &fakeStore{values: allValues()}
	got, err := run(context.Background(), s, resolverReturning(nil, nil), true)
	if err != nil {
		t.Fatal(err)
	}
	if got.subjects != 2 || got.unresolved != 2 || len(s.rewrites) != 0 {
		t.Fatalf("summary %+v, rewrites %v", got, s.rewrites)
	}
}

func TestRunDryRunWritesNothing(t *testing.T) {
	s := &fakeStore{values: allValues()}
	got, err := run(context.Background(), s, resolverReturning(map[string]string{legacy: uuidID}, nil), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.rewrites) != 0 || got.documents != int64(len(targets)) {
		t.Fatalf("summary %+v, rewrites %v", got, s.rewrites)
	}
}

func TestRunFailuresWriteNothing(t *testing.T) {
	ok := resolverReturning(map[string]string{legacy: uuidID}, nil)
	cases := map[string]struct {
		store   *fakeStore
		resolve resolver
		apply   bool
	}{
		"scan error":     {&fakeStore{values: allValues(), failOn: "distinct"}, ok, true},
		"count error":    {&fakeStore{values: allValues(), failOn: "count"}, ok, false},
		"lookup error":   {&fakeStore{values: allValues()}, resolverReturning(nil, errors.New("401")), true},
		"non-canonical":  {&fakeStore{values: allValues()}, resolverReturning(map[string]string{legacy: uuidID, orphaned: orphaned}, nil), true},
		"system mapping": {&fakeStore{values: allValues()}, resolverReturning(map[string]string{legacy: "system"}, nil), true},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := run(context.Background(), c.store, c.resolve, c.apply); err == nil {
				t.Fatal("want error")
			}
			if len(c.store.rewrites) != 0 {
				t.Fatalf("wrote %v", c.store.rewrites)
			}
		})
	}
}

func TestHTTPResolverRequiresTokenAndURL(t *testing.T) {
	for name, c := range map[string][2]string{"no token": {"http://users.invalid", ""}, "no url": {"", "t"}} {
		t.Run(name, func(t *testing.T) {
			if _, err := httpResolver(c[0], c[1])(context.Background(), []string{legacy}); err == nil {
				t.Fatal("want error")
			}
		})
	}
}

func TestHTTPResolverStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer good" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"auth0|known":"` + uuidID + `"}`))
	}))
	defer srv.Close()

	got, err := httpResolver(srv.URL, "good")(context.Background(), []string{legacy})
	if err != nil || got[legacy] != uuidID {
		t.Fatalf("got %v, %v", got, err)
	}
	if _, err := httpResolver(srv.URL, "expired")(context.Background(), []string{legacy}); err == nil {
		t.Fatal("want error for a rejected token")
	}
}
