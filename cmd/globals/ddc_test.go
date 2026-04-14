package globals

import (
	"runtime"
	"testing"

	"github.com/devrecon/ludus/internal/config"
)

func TestResolveDDCMode(t *testing.T) {
	tests := []struct {
		name    string
		flag    string
		cfgMode string
		want    string
		wantErr bool
	}{
		{"default", "", "", "local", false},
		{"flag local", "local", "", "local", false},
		{"flag none", "none", "", "none", false},
		{"config local", "", "local", "local", false},
		{"config none", "", "none", "none", false},
		{"flag overrides config", "none", "local", "none", false},
		{"invalid flag errors", "garbage", "", "", true},
		{"invalid config errors", "", "garbage", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origMode := DDCMode
			origCfg := Cfg
			t.Cleanup(func() {
				DDCMode = origMode
				Cfg = origCfg
			})

			DDCMode = tt.flag
			Cfg = &config.Config{}
			Cfg.DDC.Mode = tt.cfgMode

			got, err := ResolveDDCMode()
			if (err != nil) != tt.wantErr {
				t.Errorf("ResolveDDCMode() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ResolveDDCMode() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveDDCPath(t *testing.T) {
	absPath := "/custom/ddc"
	if runtime.GOOS == "windows" {
		absPath = `C:\custom\ddc`
	}

	tests := []struct {
		name      string
		localPath string
		nilCfg    bool
		wantPath  string // exact match; empty means "any non-empty"
		wantErr   bool
	}{
		{"config path", absPath, false, absPath, false},
		{"relative path errors", "relative/ddc", false, "", true},
		{"default path", "", false, "", false},
		{"nil config uses default", "", true, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origCfg := Cfg
			t.Cleanup(func() { Cfg = origCfg })

			if tt.nilCfg {
				Cfg = nil
			} else {
				Cfg = &config.Config{}
				Cfg.DDC.LocalPath = tt.localPath
			}

			got, err := ResolveDDCPath()
			if (err != nil) != tt.wantErr {
				t.Fatalf("ResolveDDCPath() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if tt.wantPath != "" && got != tt.wantPath {
				t.Errorf("ResolveDDCPath() = %q, want %q", got, tt.wantPath)
			}
			if tt.wantPath == "" && got == "" {
				t.Error("ResolveDDCPath() returned empty string")
			}
		})
	}
}

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
		{"custom image name with version", "", "custom-engine", "5.6.1", false, "custom-engine:5.6", false},
		{"default image name with version", "", "", "5.7.4", false, "ludus-engine:5.7", false},
		{"no version defaults to latest", "", "", "", false, "ludus-engine:latest", false},
		{"requireVersion with version succeeds", "", "", "5.7.4", true, "ludus-engine:5.7", false},
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

func TestResolveDDC(t *testing.T) {
	absPath := "/test/ddc"
	if runtime.GOOS == "windows" {
		absPath = `C:\test\ddc`
	}

	tests := []struct {
		name     string
		mode     string
		ddcPath  string
		wantMode string
		wantPath string
		wantErr  bool
	}{
		{"local mode returns both", "local", absPath, "local", absPath, false},
		{"none mode returns empty path", "none", "", "none", "", false},
		{"invalid mode errors", "garbage", "", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origMode := DDCMode
			origCfg := Cfg
			t.Cleanup(func() {
				DDCMode = origMode
				Cfg = origCfg
			})

			DDCMode = tt.mode
			Cfg = &config.Config{}
			Cfg.DDC.LocalPath = tt.ddcPath

			mode, path, err := ResolveDDC()
			if (err != nil) != tt.wantErr {
				t.Fatalf("ResolveDDC() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if mode != tt.wantMode {
				t.Errorf("mode = %q, want %q", mode, tt.wantMode)
			}
			if path != tt.wantPath {
				t.Errorf("path = %q, want %q", path, tt.wantPath)
			}
		})
	}
}
