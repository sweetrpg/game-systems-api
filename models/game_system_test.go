package models

import (
	"encoding/json"
	"testing"

	modelcore "github.com/sweetrpg/model-core.go/models"
)

func TestEnsureTagsNormalizesNilToEmpty(t *testing.T) {
	v := &GameSystemVersion{Name: "Kromore"}
	v.EnsureTags()
	if v.Tags == nil {
		t.Fatal("EnsureTags left Tags nil")
	}

	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if string(raw["tags"]) != "[]" {
		t.Fatalf("tags encoded as %s, want []", raw["tags"])
	}
}

func TestEnsureTagsKeepsExisting(t *testing.T) {
	tags := []modelcore.Tag{{Name: "genre", Value: "scifi"}}
	v := &GameSystemVersion{Tags: tags}
	v.EnsureTags()
	if len(v.Tags) != 1 || v.Tags[0].Value != "scifi" {
		t.Fatalf("EnsureTags mutated existing tags: %+v", v.Tags)
	}
}
