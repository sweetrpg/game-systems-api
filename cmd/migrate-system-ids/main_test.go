package main

import "testing"

func TestSlugify(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"Numenera", "numenera"},
		{"D&D 5th Edition", "d-d-5th-edition"},
		{"  Pathfinder 2E ", "pathfinder-2e"},
		{"---", ""},
	}
	for _, tc := range cases {
		if got := slugify(tc.in); got != tc.want {
			t.Errorf("slugify(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
