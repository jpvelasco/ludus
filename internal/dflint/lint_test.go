package dflint

import (
	"strings"
	"testing"
)

func TestCheckNoRootUser_NonRoot(t *testing.T) {
	dockerfile := `FROM ubuntu:22.04
RUN apt-get update
USER appuser
CMD ["./app"]
`
	findings := checkNoRootUser(dockerfile)
	if len(findings) != 0 {
		t.Errorf("expected 0 findings, got %d: %v", len(findings), findings)
	}
}

func TestCheckNoRootUser_Missing(t *testing.T) {
	dockerfile := `FROM ubuntu:22.04
RUN apt-get update
CMD ["./app"]
`
	findings := checkNoRootUser(dockerfile)
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Rule != "no-root-user" {
		t.Errorf("expected rule no-root-user, got %s", findings[0].Rule)
	}
	if findings[0].Level != SeverityWarning {
		t.Errorf("expected warning, got %s", findings[0].Level)
	}
}

func TestCheckNoRootUser_Root(t *testing.T) {
	dockerfile := `FROM ubuntu:22.04
USER root
CMD ["./app"]
`
	findings := checkNoRootUser(dockerfile)
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if !strings.Contains(findings[0].Message, "root") {
		t.Errorf("expected message about root, got %s", findings[0].Message)
	}
}

func TestCheckNoRootUser_NumericRoot(t *testing.T) {
	findings := checkNoRootUser("FROM alpine\nUSER 0\n")
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding for UID 0, got %d", len(findings))
	}
}

func TestCheckUnpinnedBaseImage_Pinned(t *testing.T) {
	dockerfile := `FROM public.ecr.aws/amazonlinux/amazonlinux:2023
RUN echo hello
`
	findings := checkUnpinnedBaseImage(dockerfile)
	if len(findings) != 0 {
		t.Errorf("expected 0 findings for pinned image, got %d: %v", len(findings), findings)
	}
}

func TestCheckUnpinnedBaseImage_Latest(t *testing.T) {
	dockerfile := `FROM ubuntu:latest
RUN echo hello
`
	findings := checkUnpinnedBaseImage(dockerfile)
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Rule != "unpinned-base-image" {
		t.Errorf("expected rule unpinned-base-image, got %s", findings[0].Rule)
	}
}

func TestCheckUnpinnedBaseImage_NoTag(t *testing.T) {
	dockerfile := `FROM ubuntu
RUN echo hello
`
	findings := checkUnpinnedBaseImage(dockerfile)
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding for untagged image, got %d", len(findings))
	}
}

func TestCheckUnpinnedBaseImage_DigestPinned(t *testing.T) {
	dockerfile := `FROM ubuntu@sha256:abc123
RUN echo hello
`
	findings := checkUnpinnedBaseImage(dockerfile)
	if len(findings) != 0 {
		t.Errorf("expected 0 findings for digest-pinned image, got %d", len(findings))
	}
}

func TestCheckNoPackageCleanup_WithCleanup(t *testing.T) {
	tests := []struct {
		name       string
		dockerfile string
	}{
		{
			name: "apt-get with cleanup",
			dockerfile: `FROM ubuntu:22.04
RUN apt-get update && apt-get install -y curl \
    && rm -rf /var/lib/apt/lists/*
`,
		},
		{
			name: "dnf with cleanup",
			dockerfile: `FROM amazonlinux:2023
RUN dnf install -y curl \
    && dnf clean all
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			findings := checkNoPackageCleanup(tt.dockerfile)
			if len(findings) != 0 {
				t.Errorf("expected 0 findings, got %d: %v", len(findings), findings)
			}
		})
	}
}

func TestCheckNoPackageCleanup_WithoutCleanup(t *testing.T) {
	tests := []struct {
		name       string
		dockerfile string
	}{
		{
			name: "apt-get without cleanup",
			dockerfile: `FROM ubuntu:22.04
RUN apt-get update && apt-get install -y curl
`,
		},
		{
			name: "dnf without cleanup",
			dockerfile: `FROM amazonlinux:2023
RUN dnf install -y curl
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			findings := checkNoPackageCleanup(tt.dockerfile)
			if len(findings) != 1 {
				t.Fatalf("expected 1 finding, got %d", len(findings))
			}
			if findings[0].Rule != "no-package-cleanup" {
				t.Errorf("expected rule no-package-cleanup, got %s", findings[0].Rule)
			}
		})
	}
}

func TestCheckSensitiveEnv_NoSecrets(t *testing.T) {
	dockerfile := `FROM ubuntu:22.04
ENV APP_NAME=myapp
ENV PORT=8080
`
	findings := checkSensitiveEnv(dockerfile)
	if len(findings) != 0 {
		t.Errorf("expected 0 findings, got %d: %v", len(findings), findings)
	}
}

func TestCheckSensitiveEnv_WithSecrets(t *testing.T) {
	tests := []struct {
		name string
		line string
	}{
		{"password", "ENV DB_PASSWORD=secret123"},
		{"secret", "ENV API_SECRET=abc"},
		{"token", "ENV AUTH_TOKEN=xyz"},
		{"key", "ENV AWS_SECRET_KEY=foo"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dockerfile := "FROM ubuntu:22.04\n" + tt.line + "\n"
			findings := checkSensitiveEnv(dockerfile)
			if len(findings) != 1 {
				t.Fatalf("expected 1 finding for %s, got %d", tt.name, len(findings))
			}
			if findings[0].Level != SeverityError {
				t.Errorf("expected error severity, got %s", findings[0].Level)
			}
		})
	}
}

func TestLintDockerfile_GoodDockerfile(t *testing.T) {
	// Mirrors the actual game server Dockerfile pattern
	dockerfile := `FROM public.ecr.aws/amazonlinux/amazonlinux:2023

RUN dnf install -y \
    libicu \
    libnsl \
    libstdc++ \
    shadow-utils \
    && dnf clean all

RUN useradd -m -s /bin/bash ueserver

RUN mkdir -p /opt/server && chown ueserver:ueserver /opt/server

COPY --chown=ueserver:ueserver . /opt/server/

RUN chmod +x /opt/server/amazon-gamelift-servers-game-server-wrapper \
    && chmod +x /opt/server/Lyra/Binaries/Linux/LyraServer

EXPOSE 7777/udp

USER ueserver
WORKDIR /opt/server

ENTRYPOINT ["./amazon-gamelift-servers-game-server-wrapper"]
`
	result := LintDockerfile(dockerfile)

	// Filter to only built-in findings (skip hadolint which may or may not be available)
	var builtinFindings []Finding
	for _, f := range result.Findings {
		if f.Source == "builtin" {
			builtinFindings = append(builtinFindings, f)
		}
	}

	if len(builtinFindings) != 0 {
		t.Errorf("expected 0 built-in findings for good Dockerfile, got %d:", len(builtinFindings))
		for _, f := range builtinFindings {
			t.Errorf("  [%s] %s: %s", f.Level, f.Rule, f.Message)
		}
	}
}

func TestLintDockerfile_EngineDockerfile(t *testing.T) {
	// Engine build Dockerfile has no USER — expected warning
	dockerfile := `FROM ubuntu:22.04

RUN set -e; \
    if command -v apt-get >/dev/null 2>&1; then \
        export DEBIAN_FRONTEND=noninteractive; \
        apt-get update && apt-get install -y \
            build-essential \
            git \
        && rm -rf /var/lib/apt/lists/*; \
    fi

ARG MAX_JOBS=4

COPY . /engine

WORKDIR /engine

RUN bash Setup.sh \
    && bash GenerateProjectFiles.sh \
    && make -j${MAX_JOBS} ShaderCompileWorker
`
	result := LintDockerfile(dockerfile)

	var builtinFindings []Finding
	for _, f := range result.Findings {
		if f.Source == "builtin" {
			builtinFindings = append(builtinFindings, f)
		}
	}

	// Should have exactly 1 finding: no-root-user
	foundNoRoot := false
	for _, f := range builtinFindings {
		if f.Rule == "no-root-user" {
			foundNoRoot = true
		}
	}
	if !foundNoRoot {
		t.Error("expected no-root-user warning for engine Dockerfile")
	}
}

func TestLintResult_Summary(t *testing.T) {
	tests := []struct {
		name     string
		result   LintResult
		contains string
	}{
		{
			name:     "no findings",
			result:   LintResult{},
			contains: "no issues",
		},
		{
			name: "errors only",
			result: LintResult{
				Findings: []Finding{
					{Level: SeverityError, Rule: "test"},
				},
			},
			contains: "1 error(s)",
		},
		{
			name: "warnings only",
			result: LintResult{
				Findings: []Finding{
					{Level: SeverityWarning, Rule: "test1"},
					{Level: SeverityWarning, Rule: "test2"},
				},
			},
			contains: "2 warning(s)",
		},
		{
			name: "mixed",
			result: LintResult{
				Findings: []Finding{
					{Level: SeverityError, Rule: "err"},
					{Level: SeverityWarning, Rule: "warn"},
					{Level: SeverityInfo, Rule: "info"},
				},
			},
			contains: "1 error(s), 1 warning(s), 1 info",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary := tt.result.Summary()
			if !strings.Contains(summary, tt.contains) {
				t.Errorf("expected summary to contain %q, got %q", tt.contains, summary)
			}
		})
	}
}

func TestLintResult_HasErrors(t *testing.T) {
	r := &LintResult{}
	if r.HasErrors() {
		t.Error("empty result should not have errors")
	}

	r.Findings = []Finding{{Level: SeverityWarning}}
	if r.HasErrors() {
		t.Error("warning-only result should not have errors")
	}

	r.Findings = append(r.Findings, Finding{Level: SeverityError})
	if !r.HasErrors() {
		t.Error("result with error should have errors")
	}
}

func TestLintResult_HasWarnings(t *testing.T) {
	r := &LintResult{}
	if r.HasWarnings() {
		t.Error("empty result should not have warnings")
	}

	r.Findings = []Finding{{Level: SeverityInfo}}
	if r.HasWarnings() {
		t.Error("info-only result should not have warnings")
	}

	r.Findings = []Finding{{Level: SeverityWarning}}
	if !r.HasWarnings() {
		t.Error("result with warning should have warnings")
	}

	r.Findings = []Finding{{Level: SeverityError}}
	if !r.HasWarnings() {
		t.Error("result with error should also count as having warnings")
	}
}

func TestParseRunBlocks(t *testing.T) {
	dockerfile := `FROM ubuntu:22.04
RUN echo hello
RUN apt-get update && \
    apt-get install -y curl && \
    rm -rf /var/lib/apt/lists/*
COPY . /app
RUN echo done
`
	blocks := parseRunBlocks(dockerfile)
	if len(blocks) != 3 {
		t.Fatalf("expected 3 RUN blocks, got %d", len(blocks))
	}

	if !strings.Contains(blocks[0].text, "echo hello") {
		t.Errorf("first block should contain 'echo hello', got %q", blocks[0].text)
	}

	if !strings.Contains(blocks[1].text, "apt-get install") || !strings.Contains(blocks[1].text, "rm -rf") {
		t.Errorf("second block should contain full multi-line RUN, got %q", blocks[1].text)
	}

	if !strings.Contains(blocks[2].text, "echo done") {
		t.Errorf("third block should contain 'echo done', got %q", blocks[2].text)
	}
}

func TestCountSeverities(t *testing.T) {
	tests := []struct {
		name         string
		findings     []Finding
		wantErrors   int
		wantWarnings int
		wantInfos    int
	}{
		{
			name:     "empty findings",
			findings: nil,
		},
		{
			name: "all errors",
			findings: []Finding{
				{Level: SeverityError, Rule: "a"},
				{Level: SeverityError, Rule: "b"},
			},
			wantErrors: 2,
		},
		{
			name: "mixed severities",
			findings: []Finding{
				{Level: SeverityError, Rule: "e1"},
				{Level: SeverityWarning, Rule: "w1"},
				{Level: SeverityWarning, Rule: "w2"},
				{Level: SeverityInfo, Rule: "i1"},
			},
			wantErrors:   1,
			wantWarnings: 2,
			wantInfos:    1,
		},
		{
			name: "all info",
			findings: []Finding{
				{Level: SeverityInfo, Rule: "i1"},
				{Level: SeverityInfo, Rule: "i2"},
				{Level: SeverityInfo, Rule: "i3"},
			},
			wantInfos: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := countSeverities(tt.findings)
			if got.Errors != tt.wantErrors {
				t.Errorf("Errors = %d, want %d", got.Errors, tt.wantErrors)
			}
			if got.Warnings != tt.wantWarnings {
				t.Errorf("Warnings = %d, want %d", got.Warnings, tt.wantWarnings)
			}
			if got.Infos != tt.wantInfos {
				t.Errorf("Infos = %d, want %d", got.Infos, tt.wantInfos)
			}
		})
	}
}

func TestSeverityCountsFormatCounts(t *testing.T) {
	tests := []struct {
		name   string
		counts severityCounts
		want   string
	}{
		{
			name: "all zeros",
			want: "",
		},
		{
			name:   "errors only",
			counts: severityCounts{Errors: 3},
			want:   "3 error(s)",
		},
		{
			name:   "warnings only",
			counts: severityCounts{Warnings: 2},
			want:   "2 warning(s)",
		},
		{
			name:   "infos only",
			counts: severityCounts{Infos: 5},
			want:   "5 info",
		},
		{
			name:   "all three",
			counts: severityCounts{Errors: 1, Warnings: 2, Infos: 3},
			want:   "1 error(s), 2 warning(s), 3 info",
		},
		{
			name:   "errors and infos",
			counts: severityCounts{Errors: 4, Infos: 1},
			want:   "4 error(s), 1 info",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.counts.formatCounts()
			if got != tt.want {
				t.Errorf("formatCounts() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestToolsSummary(t *testing.T) {
	tests := []struct {
		name              string
		hadolintAvailable bool
		trivyAvailable    bool
		wantContains      []string
		wantNotContains   []string
	}{
		{
			name:            "no external tools",
			wantContains:    []string{"4 built-in rules"},
			wantNotContains: []string{"hadolint", "trivy"},
		},
		{
			name:              "hadolint only",
			hadolintAvailable: true,
			wantContains:      []string{"4 built-in rules", "hadolint"},
			wantNotContains:   []string{"trivy"},
		},
		{
			name:            "trivy only",
			trivyAvailable:  true,
			wantContains:    []string{"4 built-in rules", "trivy"},
			wantNotContains: []string{"hadolint"},
		},
		{
			name:              "both tools",
			hadolintAvailable: true,
			trivyAvailable:    true,
			wantContains:      []string{"4 built-in rules", "hadolint", "trivy"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &LintResult{
				HadolintAvailable: tt.hadolintAvailable,
				TrivyAvailable:    tt.trivyAvailable,
			}
			got := r.toolsSummary()
			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("toolsSummary() = %q, want it to contain %q", got, want)
				}
			}
			for _, notWant := range tt.wantNotContains {
				if strings.Contains(got, notWant) {
					t.Errorf("toolsSummary() = %q, want it NOT to contain %q", got, notWant)
				}
			}
		})
	}
}

func TestCheckNoPackageCleanup_ConditionalBlock(t *testing.T) {
	// The engine Dockerfile uses conditional apt-get/dnf in a single RUN block
	// Both branches have cleanup, so no findings expected
	dockerfile := `FROM ubuntu:22.04
RUN set -e; \
    if command -v apt-get >/dev/null 2>&1; then \
        apt-get update && apt-get install -y build-essential \
        && rm -rf /var/lib/apt/lists/*; \
    elif command -v dnf >/dev/null 2>&1; then \
        dnf install -y gcc gcc-c++ make \
        && dnf clean all; \
    fi
`
	findings := checkNoPackageCleanup(dockerfile)
	if len(findings) != 0 {
		t.Errorf("expected 0 findings for conditional block with cleanup, got %d: %v", len(findings), findings)
	}
}
