package doctor

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/state"
	"github.com/jpvelasco/ludus/internal/testsupport"
)

func captureDoctorOutput(t *testing.T, run func() error) (string, error) {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	previous := os.Stdout
	os.Stdout = writer
	runErr := run()
	os.Stdout = previous
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return string(data), runErr
}

func TestPrintDiagnostics(t *testing.T) {
	tests := []struct {
		name      string
		checks    []diagnostic
		wantLines []string
		wantErr   bool
	}{
		{
			name: "all ok prints markers",
			checks: []diagnostic{
				{name: "One", status: "ok", message: "fine"},
				{name: "Two", status: "ok", message: "fine"},
			},
			wantLines: []string{"[OK]  ", "One", "No issues found."},
		},
		{
			name: "details are printed",
			checks: []diagnostic{
				{name: "Warn", status: "warn", message: "heads up", details: []string{"detail line"}},
			},
			wantLines: []string{"[WARN]", "Warn", "heads up", "detail line", "No issues found (1 warning(s))."},
		},
		{
			name: "fail returns error",
			checks: []diagnostic{
				{name: "Bad", status: "fail", message: "broken"},
			},
			wantLines: []string{"[FAIL]", "Bad", "1 issue(s) found"},
			wantErr:   true,
		},
		{
			name: "fail with warning summary",
			checks: []diagnostic{
				{name: "Bad", status: "fail", message: "broken"},
				{name: "W", status: "warn", message: "w"},
			},
			wantLines: []string{"1 issue(s) found, 1 warning(s)"},
			wantErr:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := captureDoctorOutput(t, func() error { return printDiagnostics(tt.checks) })
			if tt.wantErr && err == nil {
				t.Error("printDiagnostics() expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("printDiagnostics() unexpected error = %v", err)
			}
			for _, line := range tt.wantLines {
				if !strings.Contains(output, line) {
					t.Errorf("output %q missing %q", output, line)
				}
			}
		})
	}
}

func TestCheckBuildStateWarnPaths(t *testing.T) {
	t.Chdir(t.TempDir())
	state.SetProfile("")
	t.Cleanup(func() { state.SetProfile("") })

	tests := []struct {
		name    string
		state   string
		wantMsg string
	}{
		{name: "corrupt state file warns", state: "{bad json", wantMsg: "could not read"},
		{name: "missing client binary warns",
			state:   `{"client":{"binaryPath":"` + filepath.ToSlash(filepath.Join(t.TempDir(), "missing.exe")) + `"}}`,
			wantMsg: "client binary missing"},
		{name: "active deploy without fleet warns", state: `{"deploy":{"status":"active"}}`, wantMsg: "deploy marked active"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := os.MkdirAll(".ludus", 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(".ludus", "state.json"), []byte(tt.state), 0o600); err != nil {
				t.Fatal(err)
			}
			d := checkBuildState()
			if d.status != "warn" || !strings.Contains(d.message, tt.wantMsg) {
				t.Errorf("checkBuildState() = %+v, want warn containing %q", d, tt.wantMsg)
			}
		})
	}
}

func TestCheckCacheIntegrityUnreadable(t *testing.T) {
	t.Chdir(t.TempDir())
	// A corrupt cache JSON is not os.IsNotExist, so cache.Load returns an real
	// error and checkCacheIntegrity reports the cache as unreadable.
	if err := os.MkdirAll(".ludus", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(".ludus", "cache.json"), []byte("{not valid json"), 0o600); err != nil {
		t.Fatal(err)
	}
	d := checkCacheIntegrity()
	if d.status != "warn" || !strings.Contains(d.message, "cache unreadable") {
		t.Errorf("checkCacheIntegrity() = %+v, want warn cache unreadable", d)
	}
}

func TestCheckAWSCredentialExpiry(t *testing.T) {
	tests := []struct {
		name       string
		behavior   testsupport.ToolBehavior
		wantStatus string
		wantMsg    string
	}{
		{name: "valid credentials", behavior: testsupport.ToolBehavior{}, wantStatus: "ok", wantMsg: "credentials valid"},
		{name: "expired credentials", behavior: testsupport.ToolBehavior{ExitCode: 1}, wantStatus: "warn", wantMsg: "credentials expired"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testsupport.FakeTool(t, "aws", tt.behavior)
			d := checkAWSCredentialExpiry()
			assertDiagnostic(t, d, tt.wantStatus, tt.wantMsg)
		})
	}
}

func TestCheckAWSCredentialExpirySkippedWithoutAWS(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	d := checkAWSCredentialExpiry()
	assertDiagnostic(t, d, "ok", "AWS CLI not installed")
}

func TestCheckDockerDaemon(t *testing.T) {
	t.Run("daemon running", func(t *testing.T) {
		testsupport.FakeTool(t, "docker", testsupport.ToolBehavior{})
		d := checkDockerDaemon()
		assertDiagnostic(t, d, "ok", "daemon running")
	})
	t.Run("daemon not running", func(t *testing.T) {
		testsupport.FakeTool(t, "docker", testsupport.ToolBehavior{ExitCode: 1})
		d := checkDockerDaemon()
		assertDiagnostic(t, d, "warn", "daemon not running")
	})
	t.Run("docker not installed", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())
		d := checkDockerDaemon()
		if d.name != "Docker Daemon" {
			t.Errorf("name = %q, want Docker Daemon", d.name)
		}
	})
}

func TestCheckContainerImage(t *testing.T) {
	cfg := &config.Config{}
	cfg.Container.ImageName = "ludus-server"
	cfg.Container.Tag = "latest"

	t.Run("trivy not installed skips", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())
		d := checkContainerImage(cfg)
		assertDiagnostic(t, d, "ok", "install trivy")
	})

	t.Run("vulnerabilities found warns", func(t *testing.T) {
		trivyJSON := `{"Results":[{"Vulnerabilities":[{"VulnerabilityID":"CVE-2026-1","Severity":"HIGH","Title":"heap overflow","PkgName":"libc"}]}]}`
		testsupport.FakeTools(t, map[string]testsupport.ToolBehavior{
			"docker": {Stdout: "abc123"},
			"trivy":  {Stdout: trivyJSON},
		})
		d := checkContainerImage(cfg)
		if d.status != "warn" {
			t.Errorf("checkContainerImage() = %+v, want warn", d)
		}
		if len(d.details) == 0 {
			t.Errorf("checkContainerImage() details empty, want vulnerability detail")
		}
	})

	t.Run("no findings ok", func(t *testing.T) {
		testsupport.FakeTools(t, map[string]testsupport.ToolBehavior{
			"docker": {Stdout: "abc123"},
			"trivy":  {Stdout: `{"Results":[]}`},
		})
		d := checkContainerImage(cfg)
		assertDiagnostic(t, d, "ok", "no HIGH/CRITICAL vulnerabilities")
	})
}

func TestCheckDockerfileSecurity(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // no trivy/docker on PATH
	cfg := &config.Config{}
	cfg.Game.ProjectName = "Lyra"
	cfg.Container.ServerPort = 7777

	checks := checkDockerfileSecurity(cfg)
	if len(checks) != 3 {
		t.Fatalf("checkDockerfileSecurity() returned %d checks, want 3", len(checks))
	}
	wantNames := []string{"Game Dockerfile", "Engine Dockerfile", "Container Image"}
	for i, want := range wantNames {
		if checks[i].name != want {
			t.Errorf("checks[%d].name = %q, want %q", i, checks[i].name, want)
		}
	}
	if checks[0].status == "" || checks[1].status == "" {
		t.Error("dockerfile checks missing status")
	}
}
