package dflint

import (
	"strings"
	"testing"
)

func TestLintDockerfile_GoodDockerfile(t *testing.T) {
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

	builtinFindings := builtinFindings(result.Findings)
	if len(builtinFindings) != 0 {
		t.Errorf("expected 0 built-in findings for good Dockerfile, got %d:", len(builtinFindings))
		for _, f := range builtinFindings {
			t.Errorf("  [%s] %s: %s", f.Level, f.Rule, f.Message)
		}
	}
}

func TestLintDockerfile_EngineDockerfile(t *testing.T) {
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

	foundNoRoot := false
	for _, f := range builtinFindings(result.Findings) {
		if f.Rule == "no-root-user" {
			foundNoRoot = true
		}
	}
	if !foundNoRoot {
		t.Error("expected no-root-user warning for engine Dockerfile")
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
