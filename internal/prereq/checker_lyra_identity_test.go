package prereq

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/testsupport"
)

func TestResolvedLyraProjectStillRequiresContent(t *testing.T) {
	root := testsupport.FakeEngineTree(t)
	project := filepath.Join(root, "Samples", "Games", "Lyra", "Lyra.uproject")
	if err := os.MkdirAll(filepath.Dir(project), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(project, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	game := &config.GameConfig{ProjectName: "Lyra"}
	game.ResolveProjectPath(root)
	if game.ProjectPath != project {
		t.Fatal("fixture did not auto-resolve Lyra")
	}
	checker := &Checker{EngineSourcePath: root, GameConfig: game}
	assertMissingLyraContent(t, checker.CheckGameReady())
}

func assertMissingLyraContent(t *testing.T, results []CheckResult) {
	t.Helper()
	for _, result := range results {
		if result.Name != "Lyra Content" {
			continue
		}
		if result.Passed || !strings.Contains(result.Message, "Lyra Content not found") {
			t.Fatalf("missing content accepted after path resolution: %+v", result)
		}
		return
	}
	t.Fatal("Lyra content prerequisite was skipped")
}

func TestLyraStillRequiresConfiguredProject(t *testing.T) {
	project := testsupport.FakeProject(t, "Lyra")
	addLyraContent(t, filepath.Dir(project))
	if err := os.Remove(project); err != nil {
		t.Fatal(err)
	}
	checker := &Checker{GameConfig: &config.GameConfig{ProjectName: "Lyra", ProjectPath: project}}
	result := checker.checkGameContent()
	if result.Passed || !strings.Contains(result.Message, ".uproject not found") {
		t.Fatalf("missing configured project was ignored: %+v", result)
	}
}

func TestStandaloneLyraContentUsesProjectDirectory(t *testing.T) {
	project := testsupport.FakeProject(t, "Lyra")
	checker := &Checker{GameConfig: &config.GameConfig{ProjectName: "Lyra", ProjectPath: project}, PrebuiltImage: true}
	missing := checker.checkGameContent()
	if missing.Passed || !strings.Contains(missing.Message, filepath.Dir(project)) {
		t.Fatalf("expected missing content at standalone project: %+v", missing)
	}
	addLyraContent(t, filepath.Dir(project))
	if result := checker.checkGameContent(); !result.Passed {
		t.Fatalf("complete standalone Lyra rejected: %+v", result)
	}
}

func addLyraContent(t *testing.T, root string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "Content", "DefaultGameData.uasset"), []byte("asset"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, plugin := range []string{"ShooterCore", "ShooterMaps", "TopDownArena"} {
		if err := os.MkdirAll(filepath.Join(root, "Plugins", "GameFeatures", plugin, "Content"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLyraContentValidationCanBeDisabled(t *testing.T) {
	game := &config.GameConfig{
		ProjectName: "Lyra", ProjectPath: testsupport.FakeProject(t, "Lyra"),
		ContentValidation: &config.ContentValidationConfig{Disabled: true},
	}
	result := (&Checker{GameConfig: game}).checkGameContent()
	if !result.Passed || !result.Warning {
		t.Fatalf("disabled validation did not skip content: %+v", result)
	}
}

func TestRobocopyOverlay_ExitCodes(t *testing.T) {
	for _, tt := range []struct {
		name    string
		exit    int
		wantErr bool
	}{
		{"success", 0, false},
		{"minor error", 7, false},
		{"fatal error", 9, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			testsupport.FakeTool(t, "robocopy", testsupport.ToolBehavior{ExitCode: tt.exit})
			err := (&Checker{}).robocopyOverlay(t.TempDir(), t.TempDir())
			if (err != nil) != tt.wantErr {
				t.Fatalf("robocopyOverlay() exit=%d err=%v, wantErr=%v", tt.exit, err, tt.wantErr)
			}
		})
	}
}
