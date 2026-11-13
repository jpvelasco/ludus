package dockerbuild

import (
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/state"
	"testing"
)

func TestResolveEngineImage(t *testing.T) {
	tests := []struct {
		name           string
		dockerImage    string
		imageName      string
		version        string
		requireVersion bool
		want           string
		wantErr        bool
	}{
		{"explicit docker image", "my-registry/engine:latest", "", "", false, "my-registry/engine:latest", false},
		{"custom image name with version", "", "custom-engine", "5.6.1", false, "custom-engine:5.6.1", false},
		{"default image name with version", "", "", "5.7.4", false, "ludus-engine:5.7.4", false},
		{"no version defaults to latest", "", "", "", false, "ludus-engine:latest", false},
		{"requireVersion with version succeeds", "", "", "5.7.4", true, "ludus-engine:5.7.4", false},
		{"requireVersion without version errors", "", "", "", true, "", true},
		{"requireVersion bypasses check with explicit image", "my-registry/engine:custom", "", "", true, "my-registry/engine:custom", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())

			cfg := &config.Config{}
			cfg.Engine.DockerImage = tt.dockerImage
			cfg.Engine.DockerImageName = tt.imageName
			cfg.Engine.Version = tt.version

			got, err := ResolveEngineImage(cfg, tt.requireVersion)
			if (err != nil) != tt.wantErr {
				t.Errorf("ResolveEngineImage() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ResolveEngineImage() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveEngineImage_StateImageTag(t *testing.T) {
	t.Chdir(t.TempDir())
	cfg := &config.Config{}
	cfg.Engine.DockerImageName = "configured-name"
	cfg.Engine.Version = "9.9.9"
	if err := state.UpdateEngineImage(&state.EngineImageState{ImageTag: "state-engine:5.7"}); err != nil {
		t.Fatalf("UpdateEngineImage: %v", err)
	}

	got, err := ResolveEngineImage(cfg, true)
	if err != nil {
		t.Fatalf("ResolveEngineImage() error = %v, want nil", err)
	}
	if got != "state-engine:5.7" {
		t.Errorf("ResolveEngineImage() = %q, want %q", got, "state-engine:5.7")
	}
}
