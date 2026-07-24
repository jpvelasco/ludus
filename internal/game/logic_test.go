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
	tests := []struct {
		name      string
		content   string
		wantCount int
		wantHint  string // substring expected in the first hint (when wantCount==1)
	}{
		{"no matches", "nothing interesting here", 0, ""},
		{"single match", "error: GameFeatureData is missing from disk", 1, "Missing game content"},
		{"multiple distinct matches", "C1076: compiler limit reached\nalso LINUX_MULTIARCH_ROOT not set", 2, ""},
		// repeated pattern → deduped by hint
		{"duplicate hint deduped", "GameFeatureData is missing ... GameFeatureData is missing again", 1, "Missing game content"},
		// #405: warning-promotion signature → BuildSettingsVersion hint
		{"build settings mismatch (#405)",
			"LyraEditor modifies the values of properties: [ UnreachableCodeWarningLevel: Off != Error ]. " +
				"This is not allowed, as LyraEditor has build products in common with UnrealEditor.",
			1, "BuildSettingsVersion"},
		// #408 guard: generic shared-build clause with a non-warning property must NOT match
		{"generic shared-build violation does not match",
			"FooEditor modifies the values of properties: [ GlobalDefinitions ]. " +
				"This is not allowed, as FooEditor has build products in common with UnrealEditor.",
			0, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertHints(t, matchBuildLogHints(tt.content), tt.wantCount, tt.wantHint)
		})
	}
}

// assertHints checks the hint slice length and (for single matches) a substring
// of the first hint, keeping the test loop body under the complexity limit.
func assertHints(t *testing.T, hints []string, wantCount int, wantHint string) {
	t.Helper()
	if len(hints) != wantCount {
		t.Fatalf("got %d hints, want %d: %v", len(hints), wantCount, hints)
	}
	if wantHint != "" && !strings.Contains(hints[0], wantHint) {
		t.Errorf("hint %q does not contain %q", hints[0], wantHint)
	}
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
	tests := []struct {
		name          string
		input         string
		wantOK        bool
		want          string
		wantUnchanged bool
	}{
		{name: "already present is idempotent", input: "<Configuration><BuildConfiguration>" + tag + "</BuildConfiguration></Configuration>", wantOK: true, wantUnchanged: true},
		{name: "inserts into existing BuildConfiguration", input: "<Configuration>\n  <BuildConfiguration>\n  </BuildConfiguration>\n</Configuration>", wantOK: true, want: tag},
		{name: "creates BuildConfiguration", input: "<Configuration>\n</Configuration>", wantOK: true, want: tag},
		{name: "unrecognized format", input: "<xml>nope</xml>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertDumpSymsContent(t, tt.input, tt.wantOK, tt.want, tt.wantUnchanged)
		})
	}
}

func assertDumpSymsContent(t *testing.T, input string, wantOK bool, want string, wantUnchanged bool) {
	t.Helper()
	got, ok := dumpSymsDisabledContent(input)
	if ok != wantOK {
		t.Fatalf("dumpSymsDisabledContent() ok = %v, want %v", ok, wantOK)
	}
	if want != "" && !strings.Contains(got, want) {
		t.Errorf("output %q does not contain %q", got, want)
	}
	if wantUnchanged && got != input {
		t.Errorf("output = %q, want unchanged %q", got, input)
	}
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

// mkServerTargetProject creates a project dir with an optional DefaultEngine.ini
// and returns the .uproject path.
func mkServerTargetProject(t *testing.T, iniContent string) string {
	t.Helper()
	dir := t.TempDir()
	uproject := filepath.Join(dir, "MyGame.uproject")
	writeTestFile(t, uproject, "{}")
	if iniContent != "" {
		writeTestFile(t, filepath.Join(dir, "Config", "DefaultEngine.ini"), iniContent)
	}
	return uproject
}

// assertIniContains reads the project's DefaultEngine.ini and checks for want.
func assertIniContains(t *testing.T, uproject, want string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(filepath.Dir(uproject), "Config", "DefaultEngine.ini"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), want) {
		t.Errorf("expected %q in ini, got:\n%s", want, data)
	}
}

func TestEnsureDefaultServerTarget(t *testing.T) {
	tests := []struct {
		name    string
		ini     string
		opts    BuildOptions
		want    string // substring expected in the ini after the call ("" = no write check)
		wantSec bool   // also expect the BuildSettings section header present
	}{
		{name: "ini missing is graceful", ini: "", opts: BuildOptions{ProjectName: "MyGame"}},
		{name: "already set is no-op", ini: "[/Script/Engine]\nDefaultServerTarget=MyGameServer\n", opts: BuildOptions{ProjectName: "MyGame"}, want: "DefaultServerTarget=MyGameServer"},
		{name: "inserts after section header", ini: "[/Script/BuildSettings.BuildSettings]\nDefaultGameTarget=MyGameGame\n", opts: BuildOptions{ProjectName: "MyGame"}, want: "DefaultServerTarget=MyGameServer"},
		// #404: game target need not match ProjectName+"Game" (Lyra: project
		// LyraStarterGame6, target LyraGame). Section-header anchor still works.
		{name: "game target differs from ProjectName (Lyra)", ini: "[/Script/BuildSettings.BuildSettings]\nDefaultGameTarget=LyraGame\n", opts: BuildOptions{ProjectName: "LyraStarterGame6", ServerTarget: "LyraServer"}, want: "DefaultServerTarget=LyraServer"},
		{name: "appends section when absent", ini: "[/Script/Engine]\nSomethingElse=1\n", opts: BuildOptions{ProjectName: "MyGame"}, want: "DefaultServerTarget=MyGameServer", wantSec: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uproject := mkServerTargetProject(t, tt.ini)
			if err := newTestBuilder(tt.opts).ensureDefaultServerTarget(uproject); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.want != "" {
				assertIniContains(t, uproject, tt.want)
			}
			if tt.wantSec {
				assertIniContains(t, uproject, "[/Script/BuildSettings.BuildSettings]")
			}
		})
	}
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
func TestDisableDumpSymsInConfig(t *testing.T) {
	tests := []struct {
		name     string
		original string
		wantTag  bool
	}{
		{name: "updates and restores recognized config", original: "<Configuration>\n</Configuration>\n", wantTag: true},
		{name: "already disabled remains unchanged", original: "<BuildConfiguration><bDisableDumpSyms>true</bDisableDumpSyms></BuildConfiguration>"},
		{name: "malformed config remains unchanged", original: "<xml>malformed</xml>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "BuildConfiguration.xml")
			writeTestFile(t, path, tt.original)
			restore := newTestBuilder(BuildOptions{}).disableDumpSymsInConfig(path, []byte(tt.original))
			if tt.wantTag {
				assertFileContains(t, path, "<bDisableDumpSyms>true</bDisableDumpSyms>")
			}
			restore()
			assertFileEquals(t, path, tt.original)
		})
	}
}

func assertFileContains(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), want) {
		t.Errorf("%s does not contain %q: %s", path, want, data)
	}
}

func assertFileEquals(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Errorf("%s = %q, want %q", path, got, want)
	}
}

func TestPartialBuildHint(t *testing.T) {
	tests := []struct {
		name        string
		createCook  bool
		createBuild bool
		wantHint    bool
	}{
		{name: "no cooked content"},
		{name: "cooked content suggests skip cook", createCook: true, wantHint: true},
		{name: "completed server build suppresses hint", createCook: true, createBuild: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			projectPath := filepath.Join(t.TempDir(), "MyGame.uproject")
			writeTestFile(t, projectPath, "{}")
			b := newTestBuilder(BuildOptions{ProjectPath: projectPath, ProjectName: "MyGame"})
			preparePartialServerBuild(t, b, projectPath, tt.createCook, tt.createBuild)
			hint := b.PartialBuildHint()
			if (hint != "") != tt.wantHint {
				t.Errorf("PartialBuildHint() = %q, wantHint=%v", hint, tt.wantHint)
			}
		})
	}
}

func preparePartialServerBuild(t *testing.T, b *Builder, projectPath string, cook, build bool) {
	t.Helper()
	projectDir := filepath.Dir(projectPath)
	platformDir := "LinuxServer"
	if cook {
		writeTestFile(t, filepath.Join(projectDir, "Saved", "Cooked", platformDir, "asset"), "x")
	}
	if build {
		writeTestFile(t, filepath.Join(b.serverOutputDir(projectDir), platformDir, b.serverTargetName()), "x")
	}
}

func TestPartialClientBuildHint(t *testing.T) {
	tests := []struct {
		name       string
		opts       BuildOptions
		createCook bool
		createBin  bool
		wantHint   bool
	}{
		{name: "skip cook suppresses hint", opts: BuildOptions{SkipCook: true}},
		{name: "missing project suppresses hint", opts: BuildOptions{ProjectName: "Custom"}},
		{name: "no cooked content"},
		{name: "Linux cooked content suggests skip cook", createCook: true, wantHint: true},
		{name: "Win64 completed build suppresses hint", opts: BuildOptions{ClientPlatform: "Win64"}, createCook: true, createBin: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertPartialClientHint(t, tt.opts, tt.createCook, tt.createBin, tt.wantHint)
		})
	}
}

func assertPartialClientHint(t *testing.T, opts BuildOptions, cook, binary, wantHint bool) {
	t.Helper()
	if opts.ProjectName == "Custom" {
		if hint := newTestBuilder(opts).PartialClientBuildHint(); hint != "" {
			t.Errorf("PartialClientBuildHint() = %q", hint)
		}
		return
	}
	projectPath := filepath.Join(t.TempDir(), "MyGame.uproject")
	writeTestFile(t, projectPath, "{}")
	opts.ProjectPath, opts.ProjectName = projectPath, "MyGame"
	b := newTestBuilder(opts)
	preparePartialClientBuild(t, b, projectPath, cook, binary)
	hint := b.PartialClientBuildHint()
	if (hint != "") != wantHint {
		t.Errorf("PartialClientBuildHint() = %q, wantHint=%v", hint, wantHint)
	}
}

func preparePartialClientBuild(t *testing.T, b *Builder, projectPath string, cook, binary bool) {
	t.Helper()
	platform, cookedPlatform := b.opts.ClientPlatform, b.opts.ClientPlatform
	if platform == "" {
		platform, cookedPlatform = "Linux", "Linux"
	}
	if platform == "Win64" {
		cookedPlatform = "Windows"
	}
	projectDir := filepath.Dir(projectPath)
	if cook {
		writeTestFile(t, filepath.Join(projectDir, "Saved", "Cooked", cookedPlatform, "asset"), "x")
	}
	if binary {
		writeTestFile(t, b.clientBinaryPath(filepath.Join(projectDir, "PackagedClient"), platform), "x")
	}
}
func TestEnsureLinuxMultiarchRoot(t *testing.T) {
	t.Setenv("LINUX_MULTIARCH_ROOT", filepath.Join(t.TempDir(), "toolchain"))
	b := newTestBuilder(BuildOptions{})
	b.ensureLinuxMultiarchRoot()
	if len(b.Runner.Env) != 1 || !strings.HasPrefix(b.Runner.Env[0], "LINUX_MULTIARCH_ROOT=") {
		t.Fatalf("runner env = %v", b.Runner.Env)
	}
}
