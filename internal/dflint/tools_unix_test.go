//go:build !windows

package dflint

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFakeTool(t *testing.T, name, body string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestRunHadolintParsesOutput(t *testing.T) {
	writeFakeTool(t, "hadolint", `printf '%s' '[{"line":7,"code":"DL3006","message":"pin base image","level":"warning"}]'`)
	findings, available := runHadolint("FROM example\n")
	if !available || len(findings) != 1 {
		t.Fatalf("runHadolint() = (%+v, %v), want one available finding", findings, available)
	}
	got := findings[0]
	if got.Source != "hadolint" || got.Rule != "DL3006" || got.Line != 7 || got.Level != SeverityWarning {
		t.Errorf("finding = %+v", got)
	}
}

func TestRunHadolintHandlesToolOutput(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "empty", body: "exit 1"},
		{name: "malformed", body: `printf 'not-json'`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writeFakeTool(t, "hadolint", tt.body)
			findings, available := runHadolint("FROM example\n")
			if !available || len(findings) != 0 {
				t.Errorf("runHadolint() = (%+v, %v), want empty available result", findings, available)
			}
		})
	}
}

func TestRunTrivyParsesLocalImage(t *testing.T) {
	dir := t.TempDir()
	writeToolAt(t, dir, "docker", "exit 0")
	writeToolAt(t, dir, "trivy", `printf '%s' '{"Results":[{"Vulnerabilities":[{"VulnerabilityID":"CVE-1","Severity":"CRITICAL","Title":"issue","PkgName":"pkg"}]}]}'`)
	t.Setenv("PATH", dir)

	findings, available := runTrivy("example:latest")
	if !available || len(findings) != 1 {
		t.Fatalf("runTrivy() = (%+v, %v), want one available finding", findings, available)
	}
	if findings[0].Rule != "CVE-1" || findings[0].Level != SeverityError {
		t.Errorf("finding = %+v", findings[0])
	}
}

func TestRunTrivySkipsMissingImage(t *testing.T) {
	dir := t.TempDir()
	writeToolAt(t, dir, "docker", "exit 1")
	writeToolAt(t, dir, "trivy", "exit 0")
	t.Setenv("PATH", dir)

	findings, available := runTrivy("missing:latest")
	if !available || findings != nil {
		t.Errorf("runTrivy() = (%+v, %v), want nil available result", findings, available)
	}
}

func TestExecTrivyScanOutputCases(t *testing.T) {
	tests := []struct {
		name string
		body string
		ok   bool
	}{
		{name: "valid despite exit", body: `printf '{"Results":[]}'; exit 1`, ok: true},
		{name: "empty failure", body: "exit 1"},
		{name: "malformed", body: `printf 'bad-json'`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := writeToolAt(t, dir, "trivy", tt.body)
			_, ok := execTrivyScan(path, "image:tag")
			if ok != tt.ok {
				t.Errorf("execTrivyScan() ok = %v, want %v", ok, tt.ok)
			}
		})
	}
}

func writeToolAt(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}
