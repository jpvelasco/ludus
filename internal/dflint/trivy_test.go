package dflint

import (
	"strings"
	"testing"
)

func TestParseVulnerabilities(t *testing.T) {
	vulns := []trivyVuln{
		{VulnerabilityID: "CVE-C1", Severity: "critical", Title: "critical issue", PkgName: "openssl"},
		{VulnerabilityID: "CVE-H1", Severity: "HIGH", Title: "high issue"},
		{VulnerabilityID: "CVE-L1", Severity: "LOW", Title: "low issue"},
	}
	output := trivyOutput{Results: []trivyResult{{Vulnerabilities: vulns}}}

	findings := parseVulnerabilities(output)
	if len(findings) != 3 {
		t.Fatalf("parseVulnerabilities returned %d findings, want 3", len(findings))
	}

	wants := []struct {
		rule    string
		level   Severity
		message string
	}{
		{"CVE-C1", SeverityError, "critical issue (openssl)"},
		{"CVE-H1", SeverityWarning, "high issue"},
		{"CVE-L1", SeverityInfo, "low issue"},
	}
	for i, want := range wants {
		got := findings[i]
		if got.Source != "trivy" || got.Rule != want.rule || got.Level != want.level || got.Message != want.message {
			t.Errorf("finding[%d] = %+v, want rule=%q level=%q message=%q", i, got, want.rule, want.level, want.message)
		}
	}
}

func TestParseVulnerabilitiesCapsEachSeverity(t *testing.T) {
	var vulns []trivyVuln
	for i := 0; i < 7; i++ {
		vulns = append(vulns,
			trivyVuln{VulnerabilityID: "critical", Severity: "CRITICAL", Title: "critical"},
			trivyVuln{VulnerabilityID: "high", Severity: "HIGH", Title: "high"},
		)
	}

	findings := parseVulnerabilities(trivyOutput{Results: []trivyResult{{Vulnerabilities: vulns}}})
	if len(findings) != 12 {
		t.Fatalf("parseVulnerabilities returned %d findings, want 12", len(findings))
	}
	for _, want := range []string{"2 more CRITICAL", "2 more HIGH"} {
		found := false
		for _, finding := range findings {
			if finding.Rule == "overflow" && strings.Contains(finding.Message, want) {
				found = true
			}
		}
		if !found {
			t.Errorf("missing overflow finding containing %q", want)
		}
	}
}

func TestMapToolSeverities(t *testing.T) {
	tests := []struct {
		name string
		got  Severity
		want Severity
	}{
		{"trivy critical", mapTrivySeverity("CRITICAL"), SeverityError},
		{"trivy high", mapTrivySeverity("HIGH"), SeverityWarning},
		{"trivy other", mapTrivySeverity("LOW"), SeverityInfo},
		{"hadolint error", mapHadolintLevel("ERROR"), SeverityError},
		{"hadolint warning", mapHadolintLevel("Warning"), SeverityWarning},
		{"hadolint other", mapHadolintLevel("style"), SeverityInfo},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("severity = %q, want %q", tt.got, tt.want)
			}
		})
	}
}

func TestFindingsDetail(t *testing.T) {
	result := &LintResult{Findings: []Finding{
		{Rule: "DL3006", Line: 4, Level: SeverityWarning, Message: "pin the image"},
		{Rule: "overflow", Level: SeverityInfo, Message: "more findings"},
	}}
	got := result.FindingsDetail()
	want := []string{
		"[warning] DL3006 (line 4): pin the image",
		"[info] overflow: more findings",
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("FindingsDetail()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
