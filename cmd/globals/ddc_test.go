package globals

import (
	"runtime"
	"testing"

	"github.com/jpvelasco/ludus/internal/config"
)

func TestResolveContainerBackend(t *testing.T) {
	tests := []struct {
		name       string
		flag       string
		cfgBackend string
		want       string
	}{
		{"explicit docker flag", "docker", "", "docker"},
		{"explicit podman flag", "podman", "", "podman"},
		{"flag overrides config", "docker", "podman", "docker"},
		{"wsl2 flag filtered out", "wsl2", "", ""},
		{"native flag filtered out", "native", "", ""},
		{"empty flag uses config docker", "", "docker", "docker"},
		{"empty flag uses config podman", "", "podman", "podman"},
		{"wsl2 config filtered out", "", "wsl2", ""},
		{"native config filtered out", "", "native", ""},
		{"both empty returns empty", "", "", ""},
		{"nil config returns empty", "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origCfg := Cfg
			t.Cleanup(func() { Cfg = origCfg })

			if tt.name == "nil config returns empty" {
				Cfg = nil
			} else {
				Cfg = &config.Config{}
				Cfg.Engine.Backend = tt.cfgBackend
			}

			got := ResolveContainerBackend(tt.flag)
			if got != tt.want {
				t.Errorf("ResolveContainerBackend(%q) = %q, want %q", tt.flag, got, tt.want)
			}
		})
	}
}

func TestResolveDDCMode(t *testing.T) {
	tests := []struct {
		name    string
		flag    string
		cfgMode string
		want    string
		wantErr bool
	}{
		{"default", "", "", "zen", false},
		{"flag zen", "zen", "", "zen", false},
		{"flag local", "local", "", "local", false},
		{"flag none", "none", "", "none", false},
		{"config zen", "", "zen", "zen", false},
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

// assertOptionalEqual checks got == want, but treats an empty want as
// "don't assert" (the resolver fills some defaults the table doesn't pin).
func assertOptionalEqual(t *testing.T, label, got, want string) {
	t.Helper()
	if want != "" && got != want {
		t.Errorf("%s = %q, want %q", label, got, want)
	}
}

func TestResolveDDC(t *testing.T) {
	absPath := "/test/ddc"
	if runtime.GOOS == "windows" {
		absPath = `C:\test\ddc`
	}

	absZen := "/test/zen"
	if runtime.GOOS == "windows" {
		absZen = `C:\test\zen`
	}

	tests := []struct {
		name        string
		mode        string
		ddcPath     string
		zenPath     string
		wantMode    string
		wantPath    string
		wantZenPath string
		wantErr     bool
	}{
		{"default (empty) resolves to zen", "", "", absZen, "zen", "", absZen, false},
		{"zen mode returns zen path", "zen", "", absZen, "zen", "", absZen, false},
		{"local mode returns local path", "local", absPath, "", "local", absPath, "", false},
		{"none mode returns empty paths", "none", "", "", "none", "", "", false},
		{"invalid mode errors", "garbage", "", "", "", "", "", true},
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
			Cfg.DDC.ZenPath = tt.zenPath

			mode, path, zenPath, err := ResolveDDC()
			if (err != nil) != tt.wantErr {
				t.Fatalf("ResolveDDC() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if mode != tt.wantMode {
				t.Errorf("mode = %q, want %q", mode, tt.wantMode)
			}
			// wantPath/wantZenPath of "" means "don't assert exact value"
			// (the resolver fills defaults we don't pin here).
			assertOptionalEqual(t, "path", path, tt.wantPath)
			assertOptionalEqual(t, "zenPath", zenPath, tt.wantZenPath)
			if tt.wantMode == "none" && (path != "" || zenPath != "") {
				t.Errorf("none mode should return empty paths, got path=%q zenPath=%q", path, zenPath)
			}
		})
	}
}

func TestResolveContainerGameOptions(t *testing.T) {
	t.Chdir(t.TempDir())
	origCfg, origMode := Cfg, DDCMode
	t.Cleanup(func() { Cfg, DDCMode = origCfg, origMode })
	DDCMode = "none"
	Cfg = &config.Config{}
	Cfg.Engine.Version = "5.7.4"
	Cfg.Engine.DockerImageName = "custom-engine"
	Cfg.Game.ProjectPath = `C:\projects\Lyra`
	Cfg.Game.ProjectName = "Lyra"

	got, err := ResolveContainerGameOptions(Cfg, "podman")
	if err != nil {
		t.Fatalf("ResolveContainerGameOptions() error = %v", err)
	}
	if got.EngineImage != "custom-engine:5.7.4" {
		t.Errorf("EngineImage = %q, want custom-engine:5.7.4", got.EngineImage)
	}
	if got.EngineVersion != "5.7" || got.Runtime != "podman" {
		t.Errorf("resolved engine/runtime = %q/%q, want 5.7/podman", got.EngineVersion, got.Runtime)
	}
	if got.ProjectPath != Cfg.Game.ProjectPath || got.ProjectName != Cfg.Game.ProjectName {
		t.Errorf("project fields = %q/%q, want %q/%q", got.ProjectPath, got.ProjectName, Cfg.Game.ProjectPath, Cfg.Game.ProjectName)
	}
	if got.DDCMode != "none" || got.DDCPath != "" || got.DDCZenPath != "" {
		t.Errorf("DDC fields = %q/%q/%q, want none and empty paths", got.DDCMode, got.DDCPath, got.DDCZenPath)
	}
}

func TestResolveContainerGameOptionsInvalidDDC(t *testing.T) {
	origCfg, origMode := Cfg, DDCMode
	t.Cleanup(func() { Cfg, DDCMode = origCfg, origMode })
	Cfg = &config.Config{}
	DDCMode = "invalid"
	if _, err := ResolveContainerGameOptions(Cfg, "docker"); err == nil {
		t.Fatal("expected invalid DDC mode error")
	}
}
