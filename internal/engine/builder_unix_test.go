//go:build !windows

package engine

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/internal/runner"
)

func unixBuilder(t *testing.T, skipSetup bool) (*Builder, *bytes.Buffer) {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{"Setup.sh", "GenerateProjectFiles.sh"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	var output bytes.Buffer
	r := &runner.Runner{DryRun: true, Stdout: &output, Stderr: &output}
	return NewBuilder(BuildOptions{SourcePath: dir, MaxJobs: 3, SkipSetup: skipSetup}, r), &output
}

func TestBuilderBuildDryRun(t *testing.T) {
	tests := []struct {
		name      string
		skipSetup bool
		wantSetup bool
	}{
		{name: "all steps", wantSetup: true},
		{name: "skip setup", skipSetup: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder, output := unixBuilder(t, tt.skipSetup)
			result, err := builder.Build(context.Background())
			if err != nil {
				t.Fatalf("Build() error = %v", err)
			}
			if !result.Success || result.EnginePath != builder.opts.SourcePath {
				t.Errorf("Build() result = %+v", result)
			}
			assertBuildTrace(t, output.String(), tt.wantSetup)
		})
	}
}

func assertBuildTrace(t *testing.T, output string, wantSetup bool) {
	t.Helper()
	for _, want := range []string{"GenerateProjectFiles.sh", "make -j3 ShaderCompileWorker", "make -j3 UnrealEditor"} {
		if !strings.Contains(output, want) {
			t.Errorf("dry-run output missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "Setup.sh") != wantSetup {
		t.Errorf("Setup.sh presence = %v, want %v:\n%s", strings.Contains(output, "Setup.sh"), wantSetup, output)
	}
}

func TestBuilderScriptsMustExist(t *testing.T) {
	tests := []struct {
		name string
		call func(*Builder) error
		want string
	}{
		{name: "setup", call: func(b *Builder) error { return b.Setup(context.Background()) }, want: "Setup.sh not found"},
		{name: "generate", call: func(b *Builder) error { return b.GenerateProjectFiles(context.Background()) }, want: "GenerateProjectFiles.sh not found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := NewBuilder(BuildOptions{SourcePath: t.TempDir()}, &runner.Runner{DryRun: true})
			err := tt.call(builder)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestBuilderBuildReportsSetupFailure(t *testing.T) {
	builder := NewBuilder(BuildOptions{SourcePath: t.TempDir()}, &runner.Runner{DryRun: true})
	result, err := builder.Build(context.Background())
	if err == nil || !strings.Contains(err.Error(), "setup failed") {
		t.Fatalf("Build() error = %v, want setup failure", err)
	}
	if result.Error == nil || !errors.Is(result.Error, err) {
		t.Errorf("Build() result error = %v, returned error = %v", result.Error, err)
	}
}
