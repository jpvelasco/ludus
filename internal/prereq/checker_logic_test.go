package prereq

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/internal/config"
)

// --- checkServerMap ---------------------------------------------------------

func TestCheckServerMap_NoServerMapConfigured(t *testing.T) {
	// No GameConfig at all, and a config with empty ServerMap, both skip-warn.
	for _, c := range []*Checker{
		{},
		{GameConfig: &config.GameConfig{}},
	} {
		res := c.checkServerMap()
		if !res.Passed || !res.Warning {
			t.Errorf("expected skip-warning pass, got %+v", res)
		}
		if !strings.Contains(res.Message, "no serverMap configured") {
			t.Errorf("unexpected message: %q", res.Message)
		}
	}
}

func TestCheckServerMap_ContentDirUndetermined(t *testing.T) {
	// ServerMap set, but a non-Lyra project with no ProjectPath → resolveContentDir
	// returns "" → skip-warn.
	c := &Checker{GameConfig: &config.GameConfig{
		ProjectName: "MyGame",
		ServerMap:   "L_Main",
	}}
	res := c.checkServerMap()
	if !res.Passed || !res.Warning {
		t.Errorf("expected skip-warning pass, got %+v", res)
	}
	if !strings.Contains(res.Message, "could not determine project content directory") {
		t.Errorf("unexpected message: %q", res.Message)
	}
}

func TestCheckServerMap_ContentDirMissing(t *testing.T) {
	// ProjectPath points into a tree where Content/ does not exist.
	dir := t.TempDir()
	uproject := filepath.Join(dir, "MyGame.uproject")
	if err := os.WriteFile(uproject, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := &Checker{GameConfig: &config.GameConfig{
		ProjectName: "MyGame",
		ProjectPath: uproject,
		ServerMap:   "L_Main",
	}}
	res := c.checkServerMap()
	if !res.Passed || !res.Warning {
		t.Errorf("expected skip-warning pass, got %+v", res)
	}
	if !strings.Contains(res.Message, "content directory does not exist") {
		t.Errorf("unexpected message: %q", res.Message)
	}
}

func TestCheckServerMap_FoundInContent(t *testing.T) {
	dir := t.TempDir()
	uproject := filepath.Join(dir, "MyGame.uproject")
	if err := os.WriteFile(uproject, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	contentMaps := filepath.Join(dir, "Content", "Maps")
	if err := os.MkdirAll(contentMaps, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(contentMaps, "L_Main.umap"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := &Checker{GameConfig: &config.GameConfig{
		ProjectName: "MyGame",
		ProjectPath: uproject,
		ServerMap:   "L_Main",
	}}
	res := c.checkServerMap()
	if !res.Passed || res.Warning {
		t.Errorf("expected clean pass, got %+v", res)
	}
	if !strings.Contains(res.Message, "found") {
		t.Errorf("unexpected message: %q", res.Message)
	}
}

func TestCheckServerMap_FoundInPlugins(t *testing.T) {
	// UE GameFeature plugins store maps under Plugins/.../Content/Maps.
	dir := t.TempDir()
	uproject := filepath.Join(dir, "MyGame.uproject")
	if err := os.WriteFile(uproject, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Content/ must exist (else the check skips), but the map lives under Plugins/.
	if err := os.MkdirAll(filepath.Join(dir, "Content"), 0o755); err != nil {
		t.Fatal(err)
	}
	pluginMaps := filepath.Join(dir, "Plugins", "GameFeatures", "Shooter", "Content", "Maps")
	if err := os.MkdirAll(pluginMaps, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginMaps, "L_Plugin.umap"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := &Checker{GameConfig: &config.GameConfig{
		ProjectName: "MyGame",
		ProjectPath: uproject,
		ServerMap:   "L_Plugin",
	}}
	res := c.checkServerMap()
	if !res.Passed || res.Warning {
		t.Errorf("expected clean pass, got %+v", res)
	}
}

func TestCheckServerMap_NotFound(t *testing.T) {
	dir := t.TempDir()
	uproject := filepath.Join(dir, "MyGame.uproject")
	if err := os.WriteFile(uproject, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "Content"), 0o755); err != nil {
		t.Fatal(err)
	}
	c := &Checker{GameConfig: &config.GameConfig{
		ProjectName: "MyGame",
		ProjectPath: uproject,
		ServerMap:   "L_DoesNotExist",
	}}
	res := c.checkServerMap()
	if !res.Passed || !res.Warning {
		t.Errorf("expected not-found warning pass, got %+v", res)
	}
	if !strings.Contains(res.Message, "not found") {
		t.Errorf("unexpected message: %q", res.Message)
	}
}

// --- checkToolchain ---------------------------------------------------------

func TestCheckToolchain_NoEngineSourceSkips(t *testing.T) {
	c := &Checker{}
	res := c.checkToolchain()
	if !res.Passed || !res.Warning {
		t.Errorf("expected skip-warning pass, got %+v", res)
	}
	if !strings.Contains(res.Message, "no engine source path") {
		t.Errorf("unexpected message: %q", res.Message)
	}
}

func TestCheckToolchain_UnknownVersionWarns(t *testing.T) {
	// An engine source path with no detectable version yields no toolchain
	// requirement → warning pass (tc.Required == nil).
	c := &Checker{EngineSourcePath: t.TempDir()}
	res := c.checkToolchain()
	if !res.Passed {
		t.Errorf("expected pass, got %+v", res)
	}
	if res.Name != "Toolchain" {
		t.Errorf("unexpected name %q", res.Name)
	}
}

// --- checkEngineSource ------------------------------------------------------

func TestCheckEngineSource(t *testing.T) {
	t.Run("not configured", func(t *testing.T) {
		res := (&Checker{}).checkEngineSource()
		if res.Passed {
			t.Errorf("expected fail, got %+v", res)
		}
	})

	t.Run("setup file missing", func(t *testing.T) {
		res := (&Checker{EngineSourcePath: t.TempDir()}).checkEngineSource()
		if res.Passed {
			t.Errorf("expected fail when setup file absent, got %+v", res)
		}
	})

	t.Run("setup file present", func(t *testing.T) {
		dir := t.TempDir()
		setup := "Setup.sh"
		if runtime.GOOS == "windows" {
			setup = "Setup.bat"
		}
		if err := os.WriteFile(filepath.Join(dir, setup), []byte("#!/bin/sh\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		res := (&Checker{EngineSourcePath: dir}).checkEngineSource()
		if !res.Passed {
			t.Errorf("expected pass when %s present, got %+v", setup, res)
		}
	})
}

// --- checkGameContent / checkCustomProjectContent ---------------------------

func TestCheckGameContent_ValidationDisabled(t *testing.T) {
	c := &Checker{GameConfig: &config.GameConfig{
		ProjectName:       "MyGame",
		ContentValidation: &config.ContentValidationConfig{Disabled: true},
	}}
	res := c.checkGameContent()
	if !res.Passed || !res.Warning {
		t.Errorf("expected disabled skip-warn, got %+v", res)
	}
	if !strings.Contains(res.Message, "disabled") {
		t.Errorf("unexpected message: %q", res.Message)
	}
}

func TestCheckCustomProjectContent_NoProjectPath(t *testing.T) {
	c := &Checker{GameConfig: &config.GameConfig{ProjectName: "MyGame"}}
	res := c.checkGameContent() // routes to checkCustomProjectContent
	if res.Passed {
		t.Errorf("expected fail with no projectPath, got %+v", res)
	}
	if !strings.Contains(res.Message, "projectPath not configured") {
		t.Errorf("unexpected message: %q", res.Message)
	}
}

func TestCheckCustomProjectContent_UProjectMissing(t *testing.T) {
	c := &Checker{GameConfig: &config.GameConfig{
		ProjectName: "MyGame",
		ProjectPath: filepath.Join(t.TempDir(), "Absent.uproject"),
	}}
	res := c.checkGameContent()
	if res.Passed {
		t.Errorf("expected fail when .uproject missing, got %+v", res)
	}
}

func TestCheckCustomProjectContent_MarkerFileMissing(t *testing.T) {
	dir := t.TempDir()
	uproject := filepath.Join(dir, "MyGame.uproject")
	if err := os.WriteFile(uproject, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := &Checker{GameConfig: &config.GameConfig{
		ProjectName:       "MyGame",
		ProjectPath:       uproject,
		ContentValidation: &config.ContentValidationConfig{ContentMarkerFile: "Content/Marker.uasset"},
	}}
	res := c.checkGameContent()
	if res.Passed {
		t.Errorf("expected fail when marker file missing, got %+v", res)
	}
	if !strings.Contains(res.Message, "content marker file not found") {
		t.Errorf("unexpected message: %q", res.Message)
	}
}

func TestCheckCustomProjectContent_OK(t *testing.T) {
	dir := t.TempDir()
	uproject := filepath.Join(dir, "MyGame.uproject")
	if err := os.WriteFile(uproject, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(dir, "Marker.uasset")
	if err := os.WriteFile(marker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := &Checker{GameConfig: &config.GameConfig{
		ProjectName:       "MyGame",
		ProjectPath:       uproject,
		ContentValidation: &config.ContentValidationConfig{ContentMarkerFile: "Marker.uasset"},
	}}
	res := c.checkGameContent()
	if !res.Passed || res.Warning {
		t.Errorf("expected clean pass, got %+v", res)
	}
}

// --- detectLyraContentState -------------------------------------------------

func TestDetectLyraContentState(t *testing.T) {
	t.Run("top content missing", func(t *testing.T) {
		c := &Checker{EngineSourcePath: t.TempDir()}
		s := c.detectLyraContentState()
		if !s.topMissing {
			t.Errorf("expected topMissing when DefaultGameData.uasset absent")
		}
	})

	t.Run("top present, plugins missing", func(t *testing.T) {
		root := t.TempDir()
		content := filepath.Join(root, "Samples", "Games", "Lyra", "Content")
		if err := os.MkdirAll(content, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(content, "DefaultGameData.uasset"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		c := &Checker{EngineSourcePath: root}
		s := c.detectLyraContentState()
		if s.topMissing {
			t.Errorf("did not expect topMissing")
		}
		if len(s.missingPlugins) != 3 {
			t.Errorf("expected 3 missing plugins, got %v", s.missingPlugins)
		}
	})
}

// --- goVersionTooOld --------------------------------------------------------

func TestGoVersionTooOld(t *testing.T) {
	tests := []struct {
		major, minor int
		want         bool
	}{
		{1, 19, true},  // below minor
		{1, 20, false}, // exactly minimum
		{1, 25, false}, // above
		{0, 99, true},  // below major
		{2, 0, false},  // above major
	}
	for _, tt := range tests {
		if got := goVersionTooOld(tt.major, tt.minor); got != tt.want {
			t.Errorf("goVersionTooOld(%d,%d) = %v, want %v", tt.major, tt.minor, got, tt.want)
		}
	}
}

// --- appleSiliconEmulationResult --------------------------------------------

func TestAppleSiliconEmulationResult(t *testing.T) {
	res := appleSiliconEmulationResult("Cross-Arch Emulation")
	if res.Name != "Cross-Arch Emulation" {
		t.Errorf("unexpected name %q", res.Name)
	}
	if !res.Passed || !res.Warning {
		t.Errorf("expected warning pass, got %+v", res)
	}
	if !strings.Contains(res.Message, "QEMU x86_64 emulation") {
		t.Errorf("unexpected message: %q", res.Message)
	}
}

// --- NewChecker / orchestration ---------------------------------------------

func TestNewChecker(t *testing.T) {
	cfg := &config.GameConfig{ProjectName: "MyGame"}
	c := NewChecker("/engine/src", "5.7", true, cfg)
	if c.EngineSourcePath != "/engine/src" || c.EngineVersion != "5.7" || !c.Fix || c.GameConfig != cfg {
		t.Errorf("NewChecker fields not wired: %+v", c)
	}
}

func TestRunAll_ReturnsResults(t *testing.T) {
	// Smoke test: RunAll wires many sub-checks; assert it returns a non-empty,
	// well-formed slice (each result has a name) without panicking.
	c := NewChecker("", "", false, &config.GameConfig{})
	results := c.RunAll()
	if len(results) == 0 {
		t.Fatal("expected RunAll to return results")
	}
	for _, r := range results {
		if r.Name == "" {
			t.Errorf("result with empty name: %+v", r)
		}
	}
}

func TestCheckGameContainerReady_ReturnsTwoChecks(t *testing.T) {
	c := NewChecker("", "", false, &config.GameConfig{})
	results := c.CheckGameContainerReady()
	if len(results) != 2 {
		t.Fatalf("expected 2 checks (runtime + cross-arch), got %d", len(results))
	}
}
