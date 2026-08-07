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
	if len(result.Findings) != 0 {
		t.Errorf("LintImage() findings = %+v, want none", result.Findings)
	}
}

func TestRunTrivyToolUnavailable(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	findings, available := runTrivy("example:latest")
	if available || findings != nil {
		t.Errorf("runTrivy() = (%+v, %v), want nil unavailable result", findings, available)
	}
}

func TestRunTrivyMissingImageSkipsScan(t *testing.T) {
	testsupport.FakeTools(t, map[string]testsupport.ToolBehavior{
		"docker": {ExitCode: 1},
		"trivy":  {Stdout: "unused"},
	})
	findings, available := runTrivy("missing:latest")
	if !available || findings != nil {
		t.Errorf("runTrivy() = (%+v, %v), want nil available result", findings, available)
	}
}

func TestRunTrivyMalformedOutput(t *testing.T) {
	testsupport.FakeTools(t, map[string]testsupport.ToolBehavior{
		"docker": {ExitCode: 0},
		"trivy":  {Stdout: "not-json"},
	})
	findings, available := runTrivy("example:latest")
	if !available || findings != nil {
		t.Errorf("runTrivy() = (%+v, %v), want nil available result on malformed output", findings, available)
	}
}

func TestRunTrivyExitWithNoOutput(t *testing.T) {
	testsupport.FakeTools(t, map[string]testsupport.ToolBehavior{
		"docker": {ExitCode: 0},
		"trivy":  {ExitCode: 1},
	})
	findings, available := runTrivy("example:latest")
	if !available || findings != nil {
		t.Errorf("runTrivy() = (%+v, %v), want nil available result on empty failed output", findings, available)
	}
}

func TestImageExistsLocallyCases(t *testing.T) {
	t.Run("docker reports present", func(t *testing.T) {
		testsupport.FakeTool(t, "docker", testsupport.ToolBehavior{ExitCode: 0})
		if !imageExistsLocally("example:latest") {
			t.Error("imageExistsLocally() = false, want true")
		}
	})
	t.Run("docker reports missing", func(t *testing.T) {
		testsupport.FakeTool(t, "docker", testsupport.ToolBehavior{ExitCode: 1})
		if imageExistsLocally("example:latest") {
			t.Error("imageExistsLocally() = true, want false")
		}
	})
	t.Run("no docker on path", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())
		if imageExistsLocally("example:latest") {
			t.Error("imageExistsLocally() = true, want false without docker")
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
