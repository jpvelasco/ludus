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
	t.Run("config path", func(t *testing.T) {
		origCfg := Cfg
		t.Cleanup(func() { Cfg = origCfg })

		path := "/custom/ddc"
		if runtime.GOOS == "windows" {
			path = `C:\custom\ddc`
		}

		Cfg = &config.Config{}
		Cfg.DDC.LocalPath = path

		got, err := ResolveDDCPath()
		if err != nil {
			t.Fatalf("ResolveDDCPath() error: %v", err)
		}
		if got != path {
			t.Errorf("ResolveDDCPath() = %q, want %q", got, path)
		}
	})

	t.Run("relative path errors", func(t *testing.T) {
		origCfg := Cfg
		t.Cleanup(func() { Cfg = origCfg })

		Cfg = &config.Config{}
		Cfg.DDC.LocalPath = "relative/ddc"

		_, err := ResolveDDCPath()
		if err == nil {
			t.Error("ResolveDDCPath() should error for relative path")
		}
	})

	t.Run("default path", func(t *testing.T) {
		origCfg := Cfg
		t.Cleanup(func() { Cfg = origCfg })

		Cfg = &config.Config{}

		got, err := ResolveDDCPath()
		if err != nil {
			t.Fatalf("ResolveDDCPath() error: %v", err)
		}
		if got == "" {
			t.Error("ResolveDDCPath() returned empty string for default")
		}
	})

	t.Run("nil config uses default", func(t *testing.T) {
		origCfg := Cfg
		t.Cleanup(func() { Cfg = origCfg })

		Cfg = nil

		got, err := ResolveDDCPath()
		if err != nil {
			t.Fatalf("ResolveDDCPath() error: %v", err)
		}
		if got == "" {
			t.Error("ResolveDDCPath() returned empty string when Cfg is nil")
		}
	})
}

func TestResolveWarmupEngineImage(t *testing.T) {
	tests := []struct {
		name        string
		dockerImage string
		imageName   string
		version     string
		want        string
		wantErr     bool
	}{
		{
			name:        "explicit docker image",
			dockerImage: "my-registry/engine:latest",
			want:        "my-registry/engine:latest",
		},
		{
			name:      "custom image name with version",
			imageName: "custom-engine",
			version:   "5.6.1",
			want:      "custom-engine:5.6",
		},
		{
			name:    "default image name with version",
			version: "5.7.4",
			want:    "ludus-engine:5.7",
		},
		{
			name:    "undetectable version errors",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{}
			cfg.Engine.DockerImage = tt.dockerImage
			cfg.Engine.DockerImageName = tt.imageName
			cfg.Engine.Version = tt.version

			got, err := ResolveWarmupEngineImage(cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("ResolveWarmupEngineImage() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ResolveWarmupEngineImage() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveDDC(t *testing.T) {
	t.Run("local mode returns both", func(t *testing.T) {
		origMode := DDCMode
		origCfg := Cfg
		t.Cleanup(func() {
			DDCMode = origMode
			Cfg = origCfg
		})

		ddcPath := "/test/ddc"
		if runtime.GOOS == "windows" {
			ddcPath = `C:\test\ddc`
		}

		DDCMode = "local"
		Cfg = &config.Config{}
		Cfg.DDC.LocalPath = ddcPath

		mode, path, err := ResolveDDC()
		if err != nil {
			t.Fatalf("ResolveDDC() error: %v", err)
		}
		if mode != "local" {
			t.Errorf("mode = %q, want %q", mode, "local")
		}
		if path != ddcPath {
			t.Errorf("path = %q, want %q", path, ddcPath)
		}
	})

	t.Run("none mode returns empty path", func(t *testing.T) {
		origMode := DDCMode
		origCfg := Cfg
		t.Cleanup(func() {
			DDCMode = origMode
			Cfg = origCfg
		})

		DDCMode = "none"
		Cfg = &config.Config{}

		mode, path, err := ResolveDDC()
		if err != nil {
			t.Fatalf("ResolveDDC() error: %v", err)
		}
		if mode != "none" {
			t.Errorf("mode = %q, want %q", mode, "none")
		}
		if path != "" {
			t.Errorf("path = %q, want empty for mode=none", path)
		}
	})

	t.Run("invalid mode errors", func(t *testing.T) {
		origMode := DDCMode
		origCfg := Cfg
		t.Cleanup(func() {
			DDCMode = origMode
			Cfg = origCfg
		})

		DDCMode = "garbage"
		Cfg = &config.Config{}

		_, _, err := ResolveDDC()
		if err == nil {
			t.Error("ResolveDDC() should error for invalid mode")
		}
	})
}
