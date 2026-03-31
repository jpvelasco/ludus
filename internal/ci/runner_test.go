package ci

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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

func TestExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("cannot get home dir: %v", err)
	}

	tests := []struct {
		name       string
		path       string
		wantExact  string
		wantPrefix string
	}{
		{
			name:       "tilde with subpath expands to home",
			path:       "~/actions-runner",
			wantPrefix: home,
		},
		{
			name:      "tilde with nested subpath",
			path:      "~/some/deep/path",
			wantExact: filepath.Join(home, "some", "deep", "path"),
		},
		{
			name:      "absolute path unchanged",
			path:      "/opt/runner",
			wantExact: "/opt/runner",
		},
		{
			name:      "relative path unchanged",
			path:      "runner",
			wantExact: "runner",
		},
		{
			name:      "empty string unchanged",
			path:      "",
			wantExact: "",
		},
		{
			name:      "tilde alone without slash unchanged",
			path:      "~nope",
			wantExact: "~nope",
		},
		{
			name:      "dot path unchanged",
			path:      "./local/runner",
			wantExact: "./local/runner",
		},
		{
			name:      "tilde with single file",
			path:      "~/file.txt",
			wantExact: filepath.Join(home, "file.txt"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := expandHome(tt.path)

			if tt.wantExact != "" {
				if got != tt.wantExact {
					t.Errorf("got %q, want %q", got, tt.wantExact)
				}
			}

			if tt.wantPrefix != "" {
				if !strings.HasPrefix(got, tt.wantPrefix) {
					t.Errorf("got %q, want prefix %q", got, tt.wantPrefix)
				}
			}

			// Tilde paths should never remain unexpanded
			if strings.HasPrefix(tt.path, "~/") && got == tt.path {
				t.Errorf("tilde path was not expanded: %q", got)
			}
		})
	}
}
