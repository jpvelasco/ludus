package ci

import (
	"testing"
)

func TestParseRepoFromRemote(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		want    string
		wantErr bool
	}{
		{
			name: "SSH URL",
			url:  "git@github.com:jpvelasco/ludus.git",
			want: "jpvelasco/ludus",
		},
		{
			name: "SSH URL without .git",
			url:  "git@github.com:jpvelasco/ludus",
			want: "jpvelasco/ludus",
		},
		{
			name: "HTTPS URL",
			url:  "https://github.com/jpvelasco/ludus.git",
			want: "jpvelasco/ludus",
		},
		{
			name: "HTTPS URL without .git",
			url:  "https://github.com/jpvelasco/ludus",
			want: "jpvelasco/ludus",
		},
		{
			name: "URL with trailing whitespace",
			url:  "git@github.com:owner/repo.git\n",
			want: "owner/repo",
		},
		{
			name:    "non-GitHub URL",
			url:     "https://gitlab.com/owner/repo.git",
			wantErr: true,
		},
		{
			name:    "empty string",
			url:     "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
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
	t.Run("tilde prefix", func(t *testing.T) {
		result := expandHome("~/actions-runner")
		if result == "~/actions-runner" {
			t.Error("expected ~ to be expanded")
		}
		if result == "" {
			t.Error("expected non-empty result")
		}
	})

	t.Run("absolute path unchanged", func(t *testing.T) {
		result := expandHome("/opt/runner")
		if result != "/opt/runner" {
			t.Errorf("expected /opt/runner, got %q", result)
		}
	})

	t.Run("relative path unchanged", func(t *testing.T) {
		result := expandHome("runner")
		if result != "runner" {
			t.Errorf("expected runner, got %q", result)
		}
	})
}
