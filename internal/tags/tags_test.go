package tags

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/devrecon/ludus/internal/config"
)

func TestBuild_DefaultTags(t *testing.T) {
	cfg := config.Defaults()
	tags := Build(cfg)

	if tags["ManagedBy"] != "ludus" {
		t.Errorf("ManagedBy: got %q, want %q", tags["ManagedBy"], "ludus")
	}
	if tags["Project"] != "Lyra" {
		t.Errorf("Project: got %q, want %q", tags["Project"], "Lyra")
	}
}

func TestBuild_CustomTags(t *testing.T) {
	cfg := config.Defaults()
	cfg.AWS.Tags = map[string]string{
		"Environment": "prod",
		"Team":        "platform",
	}
	cfg.Game.ProjectName = "MyGame"

	tags := Build(cfg)

	if tags["Environment"] != "prod" {
		t.Errorf("Environment: got %q, want %q", tags["Environment"], "prod")
	}
	if tags["Team"] != "platform" {
		t.Errorf("Team: got %q, want %q", tags["Team"], "platform")
	}
	if tags["Project"] != "MyGame" {
		t.Errorf("Project: got %q, want %q", tags["Project"], "MyGame")
	}
	if tags["ManagedBy"] != "ludus" {
		t.Errorf("ManagedBy: got %q, want %q", tags["ManagedBy"], "ludus")
	}
}

func TestBuild_UserOverridesManagedBy(t *testing.T) {
	cfg := config.Defaults()
	cfg.AWS.Tags = map[string]string{
		"ManagedBy": "custom-tool",
	}

	tags := Build(cfg)
	if tags["ManagedBy"] != "custom-tool" {
		t.Errorf("ManagedBy should be overridable, got %q", tags["ManagedBy"])
	}
}

func TestBuild_UserOverridesProject(t *testing.T) {
	cfg := config.Defaults()
	cfg.AWS.Tags = map[string]string{
		"Project": "ExplicitProject",
	}
	cfg.Game.ProjectName = "IgnoredProject"

	tags := Build(cfg)
	if tags["Project"] != "ExplicitProject" {
		t.Errorf("user-set Project should not be overridden, got %q", tags["Project"])
	}
}

func TestBuild_EmptyProjectName(t *testing.T) {
	cfg := config.Defaults()
	cfg.Game.ProjectName = ""
	cfg.AWS.Tags = map[string]string{}

	tags := Build(cfg)
	if _, ok := tags["Project"]; ok {
		t.Error("Project tag should not be set when project name is empty")
	}
}

func TestMerge(t *testing.T) {
	base := map[string]string{"A": "1", "B": "2"}
	extra := map[string]string{"B": "override", "C": "3"}

	result := Merge(base, extra)

	if result["A"] != "1" {
		t.Errorf("A: got %q, want %q", result["A"], "1")
	}
	if result["B"] != "override" {
		t.Errorf("B: got %q, want %q", result["B"], "override")
	}
	if result["C"] != "3" {
		t.Errorf("C: got %q, want %q", result["C"], "3")
	}

	// Original should not be modified
	if base["B"] != "2" {
		t.Error("Merge should not modify base map")
	}
}

func TestMerge_MultipleExtras(t *testing.T) {
	base := map[string]string{"A": "1"}
	extra1 := map[string]string{"B": "2"}
	extra2 := map[string]string{"B": "3", "C": "4"}

	result := Merge(base, extra1, extra2)
	if result["B"] != "3" {
		t.Errorf("later extras should override earlier: got %q, want %q", result["B"], "3")
	}
	if result["C"] != "4" {
		t.Errorf("C: got %q, want %q", result["C"], "4")
	}
}

func TestWithResourceName(t *testing.T) {
	base := map[string]string{"ManagedBy": "ludus"}
	result := WithResourceName(base, "my-fleet")

	if result["Name"] != "my-fleet" {
		t.Errorf("Name: got %q, want %q", result["Name"], "my-fleet")
	}
	if result["ManagedBy"] != "ludus" {
		t.Errorf("ManagedBy should be preserved, got %q", result["ManagedBy"])
	}
	// Original should not be modified
	if _, ok := base["Name"]; ok {
		t.Error("WithResourceName should not modify original map")
	}
}

func TestToTemplateTags_Deterministic(t *testing.T) {
	tags := map[string]string{
		"Zebra":  "z",
		"Apple":  "a",
		"Mango":  "m",
		"Banana": "b",
	}

	result1 := ToTemplateTags(tags)
	result2 := ToTemplateTags(tags)

	if result1 != result2 {
		t.Error("ToTemplateTags should be deterministic")
	}

	// Verify sorted order
	appleIdx := strings.Index(result1, "Apple")
	bananaIdx := strings.Index(result1, "Banana")
	mangoIdx := strings.Index(result1, "Mango")
	zebraIdx := strings.Index(result1, "Zebra")

	if appleIdx > bananaIdx || bananaIdx > mangoIdx || mangoIdx > zebraIdx {
		t.Errorf("tags should be sorted alphabetically by key")
	}
}

func TestToTemplateTags_ValidJSON(t *testing.T) {
	tags := map[string]string{
		"Key1": "Value1",
		"Key2": "Value2",
	}

	result := ToTemplateTags(tags)

	var parsed []struct {
		Key   string `json:"Key"`
		Value string `json:"Value"`
	}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("result should be valid JSON: %v\ngot: %s", err, result)
	}
	if len(parsed) != 2 {
		t.Errorf("expected 2 tags, got %d", len(parsed))
	}
}

func TestToGameLiftTags(t *testing.T) {
	tags := map[string]string{"Key1": "Val1", "Key2": "Val2"}
	result := ToGameLiftTags(tags)

	if len(result) != 2 {
		t.Fatalf("expected 2 tags, got %d", len(result))
	}

	found := make(map[string]string)
	for _, tag := range result {
		found[*tag.Key] = *tag.Value
	}
	if found["Key1"] != "Val1" || found["Key2"] != "Val2" {
		t.Errorf("unexpected tag values: %v", found)
	}
}

func TestToIAMTags(t *testing.T) {
	tags := map[string]string{"Role": "server"}
	result := ToIAMTags(tags)

	if len(result) != 1 {
		t.Fatalf("expected 1 tag, got %d", len(result))
	}
	if *result[0].Key != "Role" || *result[0].Value != "server" {
		t.Errorf("unexpected tag: %v", result[0])
	}
}

func TestToCFNTags(t *testing.T) {
	tags := map[string]string{"Stack": "game"}
	result := ToCFNTags(tags)

	if len(result) != 1 {
		t.Fatalf("expected 1 tag, got %d", len(result))
	}
	if *result[0].Key != "Stack" || *result[0].Value != "game" {
		t.Errorf("unexpected tag: %v", result[0])
	}
}

func TestToS3Tags(t *testing.T) {
	tags := map[string]string{"Bucket": "builds"}
	result := ToS3Tags(tags)

	if len(result) != 1 {
		t.Fatalf("expected 1 tag, got %d", len(result))
	}
	if *result[0].Key != "Bucket" || *result[0].Value != "builds" {
		t.Errorf("unexpected tag: %v", result[0])
	}
}
