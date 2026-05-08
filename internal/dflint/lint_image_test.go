package dflint

import "testing"

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
