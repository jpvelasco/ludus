package game

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/internal/runner"
)

func TestResolveServerBuildArgs(t *testing.T) {
	projectPath := filepath.Join(t.TempDir(), "Lyra.uproject")
	b := NewBuilder(BuildOptions{
		ProjectName:  "Lyra",
		ServerConfig: "Shipping",
		ServerMap:    "L_Expanse",
		MaxJobs:      4,
		Arch:         "arm64",
		ServerTarget: "LyraDedicated",
		OutputDir:    filepath.Join(t.TempDir(), "Out"),
	}, runner.NewRunner(false, true))

	args, outputDir, serverTarget, err := b.resolveServerBuildArgs(projectPath)
	if err != nil {
		t.Fatalf("resolveServerBuildArgs() error: %v", err)
	}

	if outputDir != b.opts.OutputDir {
		t.Errorf("outputDir = %q, want %q", outputDir, b.opts.OutputDir)
	}
	if serverTarget != "LyraDedicated" {
		t.Errorf("serverTarget = %q, want LyraDedicated", serverTarget)
	}
	for _, want := range []string{
		"BuildCookRun",
		"-server",
		"-noclient",
		"-servertargetname=LyraDedicated",
		"-serverconfig=Shipping",
		"-cook",
		`-map="L_Expanse"`,
		"-MaxParallelActions=4",
	} {
		if !slices.Contains(args, want) {
			t.Errorf("args missing %q: %v", want, args)
		}
	}
	if !hasArgPrefix(args, "-platform=") {
		t.Errorf("args missing platform: %v", args)
	}
}

func TestResolveServerBuildArgsDefaults(t *testing.T) {
	projectDir := t.TempDir()
	projectPath := filepath.Join(projectDir, "Lyra.uproject")
	b := NewBuilder(BuildOptions{
		ProjectName: "Lyra",
		SkipCook:    true,
	}, runner.NewRunner(false, true))

	args, outputDir, serverTarget, err := b.resolveServerBuildArgs(projectPath)
	if err != nil {
		t.Fatalf("resolveServerBuildArgs() error: %v", err)
	}

	if outputDir != filepath.Join(projectDir, "PackagedServer") {
		t.Errorf("outputDir = %q, want PackagedServer under project dir", outputDir)
	}
	if serverTarget != "LyraServer" {
		t.Errorf("serverTarget = %q, want LyraServer", serverTarget)
	}
	if !slices.Contains(args, "-skipcook") {
		t.Errorf("args missing -skipcook: %v", args)
	}
	if containsArgPrefix(args, "-serverconfig=") {
		t.Errorf("args should not include server config: %v", args)
	}
	if containsArgPrefix(args, "-map=") {
		t.Errorf("args should not include map: %v", args)
	}
}

func hasArgPrefix(args []string, prefix string) bool {
	for _, arg := range args {
		if strings.HasPrefix(arg, prefix) {
			return true
		}
	}
	return false
}

func containsArgPrefix(args []string, prefix string) bool {
	return hasArgPrefix(args, prefix)
}
