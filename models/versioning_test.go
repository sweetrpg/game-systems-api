package models

import "testing"

func TestChangedFields(t *testing.T) {
	base := &GameSystemVersion{Name: "D&D", Edition: "5e", Notes: "core rulebook"}
	submitted := &GameSystemVersion{Name: "D&D", Edition: "5.5e", Notes: "core rulebook"}

	changed := changedFields(submitted, base)

	if len(changed) != 1 || changed[0] != "edition" {
		t.Fatalf("changedFields() = %v, want [edition]", changed)
	}
}

func TestChangedFieldsNoDiff(t *testing.T) {
	base := &GameSystemVersion{Name: "D&D", Edition: "5e"}
	submitted := &GameSystemVersion{Name: "D&D", Edition: "5e"}

	if changed := changedFields(submitted, base); len(changed) != 0 {
		t.Fatalf("changedFields() = %v, want none", changed)
	}
}

func TestSetFieldValue(t *testing.T) {
	v := &GameSystemVersion{}
	setFieldValue(v, "name", "Pathfinder")
	setFieldValue(v, "edition", "2e")

	if v.Name != "Pathfinder" || v.Edition != "2e" {
		t.Fatalf("setFieldValue() got Name=%q Edition=%q", v.Name, v.Edition)
	}
}
