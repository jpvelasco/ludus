package awsenv

import "testing"

func TestURIBuilders(t *testing.T) {
	ok := Env{AccountID: "123456789012", Region: "us-west-2"}

	t.Run("registry", func(t *testing.T) {
		got, err := RegistryURI(ok)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if want := "123456789012.dkr.ecr.us-west-2.amazonaws.com"; got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("repository", func(t *testing.T) {
		got, err := RepositoryURI(ok, "ludus-server")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if want := "123456789012.dkr.ecr.us-west-2.amazonaws.com/ludus-server"; got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("image", func(t *testing.T) {
		got, err := ImageURI(ok, "ludus-server", "latest")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if want := "123456789012.dkr.ecr.us-west-2.amazonaws.com/ludus-server:latest"; got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	// No builder may ever emit a blank segment.
	bad := []struct {
		name      string
		env       Env
		repo, tag string
	}{
		{"empty account", Env{Region: "us-west-2"}, "r", "t"},
		{"empty region", Env{AccountID: "123456789012"}, "r", "t"},
		{"whitespace account", Env{AccountID: "  ", Region: "us-west-2"}, "r", "t"},
		{"whitespace region", Env{AccountID: "123456789012", Region: " "}, "r", "t"},
		{"empty repo", ok, "", "t"},
		{"empty tag", ok, "r", ""},
	}
	for _, tt := range bad {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ImageURI(tt.env, tt.repo, tt.tag); err == nil {
				t.Errorf("expected error for %s, got nil", tt.name)
			}
		})
	}
}
