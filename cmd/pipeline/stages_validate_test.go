package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/deploy"
	"github.com/jpvelasco/ludus/internal/testsupport"
)

// TestStageValidateDryRunDoesNotFix is the issue-732 regression: a dry-run
// preview must not apply prerequisite auto-fixes. With Lyra content missing
// and a content source configured, validation used to overlay real files into
// the engine tree even under --dry-run.
func TestStageValidateDryRunDoesNotFix(t *testing.T) {
	engineRoot := testsupport.FakeEngineTree(t,
		testsupport.WithVersion("5.7.3"),
		testsupport.WithLinuxToolchain("v26_clang-20.1.8-rockylinux8"))
	stubCrossCompileToolchain(t)

	contentSrc := t.TempDir()
	if err := os.MkdirAll(filepath.Join(contentSrc, "Content"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(contentSrc, "Content", "DefaultGameData.uasset"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Engine: config.EngineConfig{
			SourcePath: engineRoot,
			Version:    "5.7.3",
		},
		Game: config.GameConfig{
			// No ProjectPath: routes the content check to the Lyra overlay
			// fix path with ContentSourcePath as the overlay source.
			ProjectName:       "Lyra",
			ContentSourcePath: contentSrc,
		},
	}

	// The Lyra sample directory is absent from the fake engine tree, but its
	// parent chain exists so the (pre-fix) overlay copy can complete — this
	// makes the test prove a real completed mutation, not just an attempt.
	lyraDir := filepath.Join(engineRoot, "Samples", "Games", "Lyra")
	if err := os.MkdirAll(lyraDir, 0o755); err != nil {
		t.Fatal(err)
	}

	globals.SetGlobals(t, cfg, globals.WithDryRun(true))
	testsupport.FakeTools(t, map[string]testsupport.ToolBehavior{
		"aws":    {Stdout: `{"Account":"123456789012","Arn":"arn:aws:iam::123456789012:user/test"}`},
		"docker": {},
	})

	p := &pipelineCtx{
		cfg:              cfg,
		containerBackend: "docker",
		target:           &stubTarget{name: "binary"},
	}

	out := captureStdout(func() {
		// Host disk/memory checks fail on CI runners regardless; the printed
		// check lines and the filesystem carry the signal.
		_ = p.stageValidate(context.Background())
	})

	// The preview must report the missing prerequisite as a failure instead
	// of silently auto-fixing it behind a success line.
	if !strings.Contains(out, "[FAIL] Lyra Content") {
		t.Errorf("dry run must report the missing prerequisite instead of fixing it, got:\n%s", out)
	}
	if strings.Contains(out, "overlaid from") {
		t.Errorf("dry run must not claim it applied a fix, got:\n%s", out)
	}
	overlaid := filepath.Join(engineRoot, "Samples", "Games", "Lyra", "Content", "DefaultGameData.uasset")
	if _, err := os.Stat(overlaid); !os.IsNotExist(err) {
		t.Errorf("dry run overlaid %s — a preview must not mutate the engine tree", overlaid)
	}
}

// TestNewValidateChecker pins the checker wiring for the validate stage: fixes
// are disabled under dry-run (issue 732), AWS checks are skipped for local
// targets (issue 693), and the backend/prebuilt-image flags pass through.
func TestNewValidateChecker(t *testing.T) {
	tests := []struct {
		name        string
		dryRun      bool
		caps        deploy.Capabilities
		wantFix     bool
		wantSkipAWS bool
	}{
		{
			name:        "normal run fixes and checks aws",
			caps:        deploy.Capabilities{NeedsAWS: true},
			wantFix:     true,
			wantSkipAWS: false,
		},
		{
			name:        "dry run disables fixes",
			dryRun:      true,
			caps:        deploy.Capabilities{NeedsAWS: true},
			wantFix:     false,
			wantSkipAWS: false,
		},
		{
			name:        "local target skips aws checks",
			caps:        deploy.Capabilities{},
			wantFix:     true,
			wantSkipAWS: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Engine: config.EngineConfig{SourcePath: "/engine", Version: "5.7.3"},
				Game:   config.GameConfig{ProjectName: "TestGame"},
			}
			globals.SetGlobals(t, cfg, globals.WithDryRun(tt.dryRun))

			p := &pipelineCtx{
				cfg:              cfg,
				containerBackend: "podman",
				target:           &stubTarget{caps: tt.caps},
			}

			c := p.newValidateChecker()
			if c.Fix != tt.wantFix {
				t.Errorf("Fix = %v, want %v", c.Fix, tt.wantFix)
			}
			if c.SkipAWS != tt.wantSkipAWS {
				t.Errorf("SkipAWS = %v, want %v", c.SkipAWS, tt.wantSkipAWS)
			}
			if c.Backend != "podman" {
				t.Errorf("Backend = %q, want podman", c.Backend)
			}
			if c.PrebuiltImage {
				t.Error("PrebuiltImage = true, want false without engine.dockerImage")
			}
		})
	}
}

// TestStageValidateAWSChecksFollowTarget is the issue-693 regression: a
// pipeline whose target never talks to AWS (e.g. binary export) must not run
// the AWS CLI/credentials prerequisite checks, while AWS targets keep them.
func TestStageValidateAWSChecksFollowTarget(t *testing.T) {
	tests := []struct {
		name    string
		caps    deploy.Capabilities
		wantAWS bool
	}{
		{name: "binary target skips aws checks", wantAWS: false},
		{name: "aws target keeps aws checks", caps: deploy.Capabilities{NeedsAWS: true}, wantAWS: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engineRoot := testsupport.FakeEngineTree(t,
				testsupport.WithVersion("5.7.3"),
				testsupport.WithLinuxToolchain("v26_clang-20.1.8-rockylinux8"))
			projectPath := testsupport.FakeProject(t, "TestGame")
			stubCrossCompileToolchain(t)

			cfg := &config.Config{
				Engine: config.EngineConfig{
					SourcePath: engineRoot,
					Version:    "5.7.3",
				},
				Game: config.GameConfig{
					ProjectName: "TestGame",
					ProjectPath: projectPath,
				},
			}

			globals.SetGlobals(t, cfg)
			// aws is stubbed so that, when the checks do run, they resolve
			// offline instead of hitting real credentials.
			testsupport.FakeTools(t, map[string]testsupport.ToolBehavior{
				"aws":    {Stdout: `{"Account":"123456789012","Arn":"arn:aws:iam::123456789012:user/test"}`},
				"docker": {},
			})

			p := &pipelineCtx{
				cfg:              cfg,
				containerBackend: "docker",
				target:           &stubTarget{name: "deploy", caps: tt.caps},
			}

			out := captureStdout(func() {
				_ = p.stageValidate(context.Background())
			})

			for _, awsCheck := range []string{"AWS CLI", "AWS Credentials"} {
				if got := strings.Contains(out, awsCheck); got != tt.wantAWS {
					t.Errorf("%s check present = %v, want %v, got:\n%s", awsCheck, got, tt.wantAWS, out)
				}
			}
		})
	}
}
