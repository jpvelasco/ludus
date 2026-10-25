package dflint

import (
	"testing"

	"github.com/jpvelasco/ludus/internal/testsupport"
)

// These tests exercise the trivy/hadolint exec paths on every platform.
// On Unix they overlap tools_unix_test.go coverage; on Windows they are the
// only coverage for runTrivy/imageExistsLocally/execTrivyScan/LintImage.

func TestLintImageWithStubTools(t *testing.T) {
	testsupport.FakeTools(t, map[string]testsupport.ToolBehavior{
		"docker": {ExitCode: 0},
		"trivy":  {Stdout: `{"Results":[{"Vulnerabilities":[{"VulnerabilityID":"CVE-1","Severity":"CRITICAL","Title":"issue","PkgName":"pkg"}]}]}`},
	})
	result := LintImage("example:latest")
	if !result.TrivyAvailable {
		t.Error("LintImage() TrivyAvailable = false, want true")
	}
	if result.TrivyStatus != TrivyScanned {
		t.Errorf("LintImage() TrivyStatus = %q, want %q", result.TrivyStatus, TrivyScanned)
	}
	if len(result.Findings) != 1 || result.Findings[0].Rule != "CVE-1" {
		t.Errorf("LintImage() findings = %+v, want one CVE-1", result.Findings)
	}
}

func TestLintImageNoTrivy(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	result := LintImage("example:latest")
	if result.TrivyAvailable {
		t.Error("LintImage() TrivyAvailable = true, want false without trivy")
	}
	if result.TrivyStatus != TrivyNotInstalled {
		t.Errorf("LintImage() TrivyStatus = %q, want %q", result.TrivyStatus, TrivyNotInstalled)
	}
	if len(result.Findings) != 0 {
		t.Errorf("LintImage() findings = %+v, want none", result.Findings)
	}
}

func TestRunTrivyToolUnavailable(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	scan := runTrivy("example:latest")
	if scan.status != TrivyNotInstalled || scan.findings != nil {
		t.Errorf("runTrivy() = %+v, want not-installed result without findings", scan)
	}
}

// TestRunTrivyScanStatus pins the #724 fix: a scan that did not complete is
// reported as skipped or failed — never as an authoritative zero-finding scan.
func TestRunTrivyScanStatus(t *testing.T) {
	tests := []struct {
		name          string
		tools         map[string]testsupport.ToolBehavior
		wantStatus    TrivyScanStatus
		wantReasonSet bool
	}{
		{
			name:       "image missing locally skips scan",
			tools:      map[string]testsupport.ToolBehavior{"docker": {ExitCode: 1}, "trivy": {Stdout: "unused"}},
			wantStatus: TrivySkipped, wantReasonSet: true,
		},
		{
			name:       "trivy exits nonzero without output",
			tools:      map[string]testsupport.ToolBehavior{"docker": {ExitCode: 0}, "trivy": {ExitCode: 1}},
			wantStatus: TrivyFailed, wantReasonSet: true,
		},
		{
			name:       "malformed trivy output",
			tools:      map[string]testsupport.ToolBehavior{"docker": {ExitCode: 0}, "trivy": {Stdout: "not-json"}},
			wantStatus: TrivyFailed, wantReasonSet: true,
		},
		{
			name: "completed scan",
			tools: map[string]testsupport.ToolBehavior{
				"docker": {ExitCode: 0},
				"trivy":  {Stdout: `{"Results":[{"Vulnerabilities":[{"VulnerabilityID":"CVE-1","Severity":"HIGH","Title":"issue"}]}]}`},
			},
			wantStatus: TrivyScanned, wantReasonSet: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testsupport.FakeTools(t, tt.tools)
			scan := runTrivy("example:latest")
			if scan.status != tt.wantStatus {
				t.Errorf("runTrivy() status = %q, want %q", scan.status, tt.wantStatus)
			}
			if (scan.reason != "") != tt.wantReasonSet {
				t.Errorf("runTrivy() reason = %q, wantReasonSet %v", scan.reason, tt.wantReasonSet)
			}
		})
	}
}

func TestLintImageReportsScanStatus(t *testing.T) {
	// Docker stub reports the image missing, so the scan is skipped and the
	// result must say so instead of reporting zero vulnerabilities (#724).
	testsupport.FakeTools(t, map[string]testsupport.ToolBehavior{
		"docker": {ExitCode: 1},
		"trivy":  {Stdout: "unused"},
	})
	result := LintImage("missing:latest")
	if !result.TrivyAvailable {
		t.Error("LintImage() TrivyAvailable = false, want true")
	}
	if result.TrivyStatus != TrivySkipped {
		t.Errorf("LintImage() TrivyStatus = %q, want %q", result.TrivyStatus, TrivySkipped)
	}
	if result.TrivyReason == "" {
		t.Error("LintImage() TrivyReason should explain why the scan was skipped")
	}
	if len(result.Findings) != 0 {
		t.Errorf("LintImage() findings = %+v, want none", result.Findings)
	}
}

func TestImageExistsLocallyCases(t *testing.T) {
	t.Run("docker reports present", func(t *testing.T) {
		testsupport.FakeTool(t, "docker", testsupport.ToolBehavior{ExitCode: 0})
		if present, _ := imageExistsLocally("example:latest"); !present {
			t.Error("imageExistsLocally() = false, want true")
		}
	})
	t.Run("docker reports missing", func(t *testing.T) {
		testsupport.FakeTool(t, "docker", testsupport.ToolBehavior{ExitCode: 1})
		if present, reason := imageExistsLocally("example:latest"); present || reason != "" {
			t.Errorf("imageExistsLocally() = (%v, %q), want absent without reason", present, reason)
		}
	})
	t.Run("no docker on path", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())
		if present, reason := imageExistsLocally("example:latest"); present || reason == "" {
			t.Errorf("imageExistsLocally() = (%v, %q), want absent with reason", present, reason)
		}
	})
}

func TestRunHadolintToolMissing(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	findings, available := runHadolint("FROM example\n")
	if available || findings != nil {
		t.Errorf("runHadolint() = (%+v, %v), want nil unavailable result", findings, available)
	}
}

func TestRunHadolintEmptyOutput(t *testing.T) {
	testsupport.FakeTool(t, "hadolint", testsupport.ToolBehavior{ExitCode: 1})
	findings, available := runHadolint("FROM example\n")
	if !available || len(findings) != 0 {
		t.Errorf("runHadolint() = (%+v, %v), want empty available result", findings, available)
	}
}

func TestRunHadolintLevelsViaStub(t *testing.T) {
	testsupport.FakeTool(t, "hadolint", testsupport.ToolBehavior{
		Stdout: `[{"line":1,"code":"DL3006","message":"pin base image","level":"warning"},{"line":2,"code":"DL4006","message":"set shell","level":"error"},{"line":3,"code":"DL1000","message":"other","level":"notice"}]`,
	})
	findings, available := runHadolint("FROM example\n")
	if !available || len(findings) != 3 {
		t.Fatalf("runHadolint() = (%+v, %v), want three findings", findings, available)
	}
	if findings[0].Level != SeverityWarning || findings[1].Level != SeverityError || findings[2].Level != SeverityInfo {
		t.Errorf("levels = %v/%v/%v, want warning/error/info", findings[0].Level, findings[1].Level, findings[2].Level)
	}
}
