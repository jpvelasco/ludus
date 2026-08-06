package ci

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/internal/runner"
)

var parseRepoFromRemoteTests = []struct {
	name    string
	url     string
	want    string
	wantErr bool
}{
	{name: "SSH URL with .git", url: "git@github.com:jpvelasco/ludus.git", want: "jpvelasco/ludus"},
	{name: "SSH URL without .git", url: "git@github.com:jpvelasco/ludus", want: "jpvelasco/ludus"},
	{name: "HTTPS URL with .git", url: "https://github.com/jpvelasco/ludus.git", want: "jpvelasco/ludus"},
	{name: "HTTPS URL without .git", url: "https://github.com/jpvelasco/ludus", want: "jpvelasco/ludus"},
	{name: "SSH URL with trailing newline", url: "git@github.com:owner/repo.git\n", want: "owner/repo"},
	{name: "HTTPS URL with trailing whitespace", url: "https://github.com/owner/repo.git  \n", want: "owner/repo"},
	{name: "SSH URL different owner and repo", url: "git@github.com:my-org/my-project.git", want: "my-org/my-project"},
	{name: "HTTPS URL with hyphens in names", url: "https://github.com/my-org/my-project.git", want: "my-org/my-project"},
	{name: "SSH URL with underscores", url: "git@github.com:some_user/some_repo", want: "some_user/some_repo"},
	{name: "HTTPS URL with underscores", url: "https://github.com/some_user/some_repo", want: "some_user/some_repo"},
	{name: "non-GitHub SSH URL", url: "git@gitlab.com:owner/repo.git", wantErr: true},
	{name: "non-GitHub HTTPS URL", url: "https://gitlab.com/owner/repo.git", wantErr: true},
	{name: "empty string", url: "", wantErr: true},
	{name: "whitespace only", url: "   \n\t  ", wantErr: true},
	{name: "random text", url: "not-a-url-at-all", wantErr: true},
	{name: "GitHub URL but missing repo path", url: "https://github.com/owner", wantErr: true},
	{name: "bare github.com", url: "https://github.com", wantErr: true},
}

func TestParseRepoFromRemote(t *testing.T) {
	for _, tt := range parseRepoFromRemoteTests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseRepoFromRemote(tt.url)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

var expandHomeTests = []struct {
	name         string
	path         string
	wantExact    string // expected result for non-home paths
	wantRelative string // expected path relative to home (joined with home at runtime)
}{
	{name: "tilde with subpath expands to home", path: "~/actions-runner", wantRelative: "actions-runner"},
	{name: "tilde with nested subpath", path: "~/some/deep/path", wantRelative: filepath.Join("some", "deep", "path")},
	{name: "absolute path unchanged", path: "/opt/runner", wantExact: "/opt/runner"},
	{name: "relative path unchanged", path: "runner", wantExact: "runner"},
	{name: "empty string unchanged", path: ""},
	{name: "tilde alone without slash unchanged", path: "~nope", wantExact: "~nope"},
	{name: "dot path unchanged", path: "./local/runner", wantExact: "./local/runner"},
	{name: "tilde with single file", path: "~/file.txt", wantRelative: "file.txt"},
}

func TestExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("cannot get home dir: %v", err)
	}

	for _, tt := range expandHomeTests {
		t.Run(tt.name, func(t *testing.T) {
			got := expandHome(tt.path)

			var want string
			if tt.wantRelative != "" {
				want = filepath.Join(home, tt.wantRelative)
			} else {
				want = tt.wantExact
			}
			if got != want {
				t.Errorf("got %q, want %q", got, want)
			}
		})
	}
}

func TestDownloadRunnerName(t *testing.T) {
	dir := t.TempDir()
	ri := &RunnerInstaller{Runner: runner.NewRunner(false, true)}
	got, err := ri.downloadRunner(context.Background(), dir, "v2.311.0")
	if err != nil {
		t.Fatalf("dry-run download must not error: %v", err)
	}
	if want := "actions-runner-linux-x64-2.311.0.tar.gz"; got != want {
		t.Errorf("tarball = %q, want %q", got, want)
	}
}

func TestRunnerInstaller_NonLinuxPlatforms(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("non-linux error paths not exercised on linux")
	}
	ri := &RunnerInstaller{Runner: runner.NewRunner(false, false)}
	ctx := context.Background()

	if err := ri.Install(ctx); err == nil || !strings.Contains(err.Error(), "only supported on Linux") {
		t.Errorf("Install error = %v, want linux-only message", err)
	}
	if _, err := ri.Status(ctx); err == nil || !strings.Contains(err.Error(), "only supported on Linux") {
		t.Errorf("Status error = %v, want linux-only message", err)
	}
	if err := ri.Uninstall(ctx, false); err == nil || !strings.Contains(err.Error(), "only supported on Linux") {
		t.Errorf("Uninstall error = %v, want linux-only message", err)
	}
}
