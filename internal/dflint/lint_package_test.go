package dflint

import "testing"

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

func TestCheckNoPackageCleanup_ConditionalBlock(t *testing.T) {
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
