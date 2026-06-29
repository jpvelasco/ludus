package game

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/internal/runner"
)

func newTestBuilder(opts BuildOptions) *Builder {
	return NewBuilder(opts, runner.NewRunner(false, true)) // dry-run runner
}

// --- diagnostics ------------------------------------------------------------

func TestDiagnoseBuildError_Nil(t *testing.T) {
	if err := diagnoseBuildError(nil, "build", ""); err != nil {
		t.Errorf("expected nil for nil input, got %v", err)
	}
}

func TestIsSmartScreenExit(t *testing.T) {
	t.Run("non-exit error", func(t *testing.T) {
		if isSmartScreenExit(errors.New("plain")) {
			t.Error("plain error should not be a SmartScreen exit")
		}
	})
	// Note: the magic 0xC0E90002 exit code is hard to synthesize portably via
	// exec.ExitError; the non-exit path is the testable branch here.
}

func TestSmartScreenError_Message(t *testing.T) {
	inner := errors.New("boom")
	err := smartScreenError(inner, "game build")
	if !strings.Contains(err.Error(), "SmartScreen") || !strings.Contains(err.Error(), "game build") {
		t.Errorf("unexpected message: %v", err)
	}
	if !errors.Is(err, inner) {
		t.Error("smartScreenError should wrap the original error")
	}
}

func TestBuildLogError_WrapsAndMentionsLog(t *testing.T) {
	inner := errors.New("inner failure")
	err := buildLogError(inner, "game build", t.TempDir())
	if !errors.Is(err, inner) {
		t.Error("buildLogError should wrap the original error")
	}
	if !strings.Contains(err.Error(), "game build failed") {
		t.Errorf("unexpected message: %v", err)
	}
	if !strings.Contains(err.Error(), "Full build log:") {
		t.Errorf("expected log path in message: %v", err)
	}
}

func TestMatchBuildLogHints(t *testing.T) {
	t.Run("no matches", func(t *testing.T) {
		if hints := matchBuildLogHints("nothing interesting here"); hints != nil {
			t.Errorf("expected no hints, got %v", hints)
		}
	})

	t.Run("single match", func(t *testing.T) {
		hints := matchBuildLogHints("error: GameFeatureData is missing from disk")
		if len(hints) != 1 || !strings.Contains(hints[0], "Missing game content") {
			t.Errorf("unexpected hints: %v", hints)
		}
	})

	t.Run("multiple distinct matches", func(t *testing.T) {
		content := "C1076: compiler limit reached\nalso LINUX_MULTIARCH_ROOT not set"
		hints := matchBuildLogHints(content)
		if len(hints) != 2 {
			t.Errorf("expected 2 hints, got %d: %v", len(hints), hints)
		}
	})

	t.Run("duplicate hint deduped", func(t *testing.T) {
		// Both SAC patterns... use two patterns that map to distinct hints, then
		// repeat one pattern to confirm dedup by hint.
		content := "GameFeatureData is missing ... GameFeatureData is missing again"
		hints := matchBuildLogHints(content)
		if len(hints) != 1 {
			t.Errorf("expected deduped single hint, got %v", hints)
		}
	})

	t.Run("build settings mismatch hint (#405)", func(t *testing.T) {
		// The UBT failure when an older project target conflicts with newer engine
		// warning-level defaults (e.g. Lyra V6 vs UE 5.8).
		content := "LyraEditor modifies the values of properties: [ ... ]. " +
			"This is not allowed, as LyraEditor has build products in common with UnrealEditor."
		hints := matchBuildLogHints(content)
		if len(hints) != 1 || !strings.Contains(hints[0], "BuildSettingsVersion") {
			t.Errorf("expected BuildSettingsVersion hint, got %v", hints)
		}
	})
}

func TestDiagnoseBuildError_AppendsLogHints(t *testing.T) {
	engine := t.TempDir()
	writeTestFile(t, buildLogPath(engine), "fatal: GameFeatureData is missing\n")
	err := diagnoseBuildError(errors.New("uat failed"), "game build", engine)
	if !strings.Contains(err.Error(), "Diagnostics from build logs:") {
		t.Errorf("expected diagnostics section, got: %v", err)
	}
	if !strings.Contains(err.Error(), "Missing game content") {
		t.Errorf("expected matched hint, got: %v", err)
	}
}

// --- workarounds: dumpSymsDisabledContent -----------------------------------

func TestDumpSymsDisabledContent(t *testing.T) {
	tag := "<bDisableDumpSyms>true</bDisableDumpSyms>"

	t.Run("already present is idempotent", func(t *testing.T) {
		in := "<Configuration><BuildConfiguration>" + tag + "</BuildConfiguration></Configuration>"
		out, ok := dumpSymsDisabledContent(in)
		if !ok || out != in {
			t.Errorf("expected unchanged content, ok=%v", ok)
		}
	})

	t.Run("inserts into existing BuildConfiguration", func(t *testing.T) {
		in := "<Configuration>\n  <BuildConfiguration>\n  </BuildConfiguration>\n</Configuration>"
		out, ok := dumpSymsDisabledContent(in)
		if !ok || !strings.Contains(out, tag) {
			t.Errorf("expected tag inserted, ok=%v out=%q", ok, out)
		}
	})

	t.Run("creates BuildConfiguration when only Configuration present", func(t *testing.T) {
		in := "<Configuration>\n</Configuration>"
		out, ok := dumpSymsDisabledContent(in)
		if !ok || !strings.Contains(out, tag) || !strings.Contains(out, "<BuildConfiguration>") {
			t.Errorf("expected new BuildConfiguration block, ok=%v out=%q", ok, out)
		}
	})

	t.Run("unrecognized format", func(t *testing.T) {
		if _, ok := dumpSymsDisabledContent("<xml>nope</xml>"); ok {
			t.Error("expected ok=false for unrecognized format")
		}
	})
}

// --- workarounds: gameTargetName --------------------------------------------

func TestGameTargetName(t *testing.T) {
	if got := newTestBuilder(BuildOptions{GameTarget: "CustomGame"}).gameTargetName(); got != "CustomGame" {
		t.Errorf("explicit GameTarget: got %q", got)
	}
	if got := newTestBuilder(BuildOptions{ProjectName: "Lyra"}).gameTargetName(); got != "LyraGame" {
		t.Errorf("derived: got %q, want LyraGame", got)
	}
}

// --- workarounds: ensureDefaultServerTarget ---------------------------------

func TestEnsureDefaultServerTarget(t *testing.T) {
	mkProject := func(t *testing.T, iniContent string) string {
		t.Helper()
		dir := t.TempDir()
		uproject := filepath.Join(dir, "MyGame.uproject")
		writeTestFile(t, uproject, "{}")
		if iniContent != "" {
			writeTestFile(t, filepath.Join(dir, "Config", "DefaultEngine.ini"), iniContent)
		}
		return uproject
	}

	t.Run("ini missing is graceful", func(t *testing.T) {
		uproject := mkProject(t, "")
		b := newTestBuilder(BuildOptions{ProjectName: "MyGame"})
		if err := b.ensureDefaultServerTarget(uproject); err != nil {
			t.Errorf("expected nil when ini missing, got %v", err)
		}
	})

	t.Run("already set is no-op", func(t *testing.T) {
		uproject := mkProject(t, "[/Script/Engine]\nDefaultServerTarget=MyGameServer\n")
		b := newTestBuilder(BuildOptions{ProjectName: "MyGame"})
		if err := b.ensureDefaultServerTarget(uproject); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("no DefaultGameTarget anchor skips", func(t *testing.T) {
		uproject := mkProject(t, "[/Script/Engine]\nSomethingElse=1\n")
		b := newTestBuilder(BuildOptions{ProjectName: "MyGame"})
		if err := b.ensureDefaultServerTarget(uproject); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("inserts after DefaultGameTarget", func(t *testing.T) {
		uproject := mkProject(t, "DefaultGameTarget=MyGameGame\n")
		b := newTestBuilder(BuildOptions{ProjectName: "MyGame"})
		if err := b.ensureDefaultServerTarget(uproject); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		data, err := os.ReadFile(filepath.Join(filepath.Dir(uproject), "Config", "DefaultEngine.ini"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), "DefaultServerTarget=MyGameServer") {
			t.Errorf("expected DefaultServerTarget inserted, got:\n%s", data)
		}
	})
}

// --- builder_client: resolveClientPlatform / clientBuildArgs / binaryPath ---

func TestResolveClientPlatform(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"", "Linux", false},
		{"Linux", "Linux", false},
		{"Win64", "Win64", false},
		{"Android", "", true},
	}
	for _, tt := range tests {
		got, err := newTestBuilder(BuildOptions{ClientPlatform: tt.in}).resolveClientPlatform()
		if (err != nil) != tt.wantErr {
			t.Errorf("platform %q: err=%v wantErr=%v", tt.in, err, tt.wantErr)
		}
		if !tt.wantErr && got != tt.want {
			t.Errorf("platform %q: got %q want %q", tt.in, got, tt.want)
		}
	}
}

func TestClientBuildArgs(t *testing.T) {
	t.Run("cook by default", func(t *testing.T) {
		args := newTestBuilder(BuildOptions{}).clientBuildArgs("/p/MyGame.uproject", "Linux", "/out")
		joined := strings.Join(args, " ")
		if !strings.Contains(joined, "BuildCookRun") || !strings.Contains(joined, "-cook") {
			t.Errorf("expected BuildCookRun with -cook, got: %v", args)
		}
		if !strings.Contains(joined, "-platform=Linux") {
			t.Errorf("expected platform flag, got: %v", args)
		}
	})
	t.Run("skip-cook", func(t *testing.T) {
		args := newTestBuilder(BuildOptions{SkipCook: true}).clientBuildArgs("/p/MyGame.uproject", "Linux", "/out")
		joined := strings.Join(args, " ")
		if !strings.Contains(joined, "-skipcook") || strings.Contains(joined, " -cook") {
			t.Errorf("expected -skipcook, got: %v", args)
		}
	})
}

// --- server_build_args helpers ----------------------------------------------

func TestServerTargetName(t *testing.T) {
	if got := newTestBuilder(BuildOptions{ServerTarget: "X"}).serverTargetName(); got != "X" {
		t.Errorf("explicit: got %q", got)
	}
	if got := newTestBuilder(BuildOptions{ProjectName: "Lyra"}).serverTargetName(); got != "LyraServer" {
		t.Errorf("derived: got %q want LyraServer", got)
	}
}

func TestServerOutputDir(t *testing.T) {
	if got := newTestBuilder(BuildOptions{OutputDir: "/custom"}).serverOutputDir("/proj"); got != "/custom" {
		t.Errorf("explicit OutputDir: got %q", got)
	}
	got := newTestBuilder(BuildOptions{}).serverOutputDir("/proj")
	if got != filepath.Join("/proj", "PackagedServer") {
		t.Errorf("default: got %q", got)
	}
}

// --- runuat -----------------------------------------------------------------

func TestResolveRunUAT(t *testing.T) {
	t.Run("missing script errors", func(t *testing.T) {
		_, _, err := newTestBuilder(BuildOptions{EnginePath: t.TempDir()}).resolveRunUAT()
		if err == nil {
			t.Error("expected error when RunUAT script absent")
		}
	})

	t.Run("present resolves shell + path", func(t *testing.T) {
		engine := t.TempDir()
		script := "RunUAT.sh"
		wantShell := "bash"
		if runtime.GOOS == "windows" {
			script = "RunUAT.bat"
			wantShell = "cmd"
		}
		writeTestFile(t, filepath.Join(engine, "Engine", "Build", "BatchFiles", script), "@echo off\n")
		shell, scriptPath, err := newTestBuilder(BuildOptions{EnginePath: engine}).resolveRunUAT()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if shell != wantShell {
			t.Errorf("shell = %q, want %q", shell, wantShell)
		}
		if !strings.HasSuffix(scriptPath, script) {
			t.Errorf("scriptPath = %q, want suffix %q", scriptPath, script)
		}
	})
}

// --- project: PartialBuildHint ----------------------------------------------

func TestPartialBuildHint_SkipCook(t *testing.T) {
	if hint := newTestBuilder(BuildOptions{SkipCook: true}).PartialBuildHint(); hint != "" {
		t.Errorf("expected empty hint when SkipCook set, got %q", hint)
	}
}

func TestPartialBuildHint_NoProject(t *testing.T) {
	// LocateProject fails for a custom project with no path → empty hint.
	if hint := newTestBuilder(BuildOptions{ProjectName: "MyGame"}).PartialBuildHint(); hint != "" {
		t.Errorf("expected empty hint when project not locatable, got %q", hint)
	}
}
