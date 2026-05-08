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
