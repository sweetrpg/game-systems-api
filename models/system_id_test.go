package models

import "testing"

func TestValidateSystemID(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"numenera", true},
		{"d-d-5e", true},
		{"a1-b2", true},
		{"", false},
		{"Numenera", false},
		{"-numenera", false},
		{"numenera-", false},
		{"num--enera", false},
		{"num enera", false},
		{"num_enera", false},
	}
	for _, tc := range cases {
		if got := ValidateSystemID(tc.in); got != tc.want {
			t.Errorf("ValidateSystemID(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}
