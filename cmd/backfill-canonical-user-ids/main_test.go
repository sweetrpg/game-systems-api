package main

import "testing"

func TestIsSubjectShaped(t *testing.T) {
	cases := map[string]bool{
		"":                                     false,
		"system":                               false,
		"b3384f5d-78c1-4965-8112-37395c2b8ef3": false, // canonical users._id (UUID)
		"auth0|abc123":                         true,
		"github|419457":                        true,
		"google-oauth2|108...":                 true,
		"p3NcXqrwnl2R79tAA4ntqlu9WQgXxiYg@clients": true,
	}
	for in, want := range cases {
		if got := isSubjectShaped(in); got != want {
			t.Errorf("isSubjectShaped(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestPlanRewrites(t *testing.T) {
	subjects := []string{"auth0|known", "auth0|missing"}
	resolved := map[string]string{"auth0|known": "b3384f5d-78c1-4965-8112-37395c2b8ef3"}

	got := planRewrites(subjects, resolved)

	if got["auth0|known"] != "b3384f5d-78c1-4965-8112-37395c2b8ef3" {
		t.Errorf("resolved subject: got %q", got["auth0|known"])
	}
	if got["auth0|missing"] != "system" {
		t.Errorf("unmappable subject should fall back to system: got %q", got["auth0|missing"])
	}
}
