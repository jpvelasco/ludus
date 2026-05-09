package dflint

import (
	"strings"
	"testing"
)

func TestLintResult_Summary(t *testing.T) {
	tests := []struct {
		name     string
		result   LintResult
		contains string
	}{
		{
			name:     "no findings",
			result:   LintResult{},
			contains: "no issues",
		},
		{
			name: "errors only",
			result: LintResult{
				Findings: []Finding{
					{Level: SeverityError, Rule: "test"},
				},
			},
			contains: "1 error(s)",
		},
		{
			name: "warnings only",
			result: LintResult{
				Findings: []Finding{
					{Level: SeverityWarning, Rule: "test1"},
					{Level: SeverityWarning, Rule: "test2"},
				},
			},
			contains: "2 warning(s)",
		},
		{
			name: "mixed",
			result: LintResult{
				Findings: []Finding{
					{Level: SeverityError, Rule: "err"},
					{Level: SeverityWarning, Rule: "warn"},
					{Level: SeverityInfo, Rule: "info"},
				},
			},
			contains: "1 error(s), 1 warning(s), 1 info",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary := tt.result.Summary()
			if !strings.Contains(summary, tt.contains) {
				t.Errorf("expected summary to contain %q, got %q", tt.contains, summary)
			}
		})
	}
}

func TestLintResult_HasErrors(t *testing.T) {
	r := &LintResult{}
	if r.HasErrors() {
		t.Error("empty result should not have errors")
	}

	r.Findings = []Finding{{Level: SeverityWarning}}
	if r.HasErrors() {
		t.Error("warning-only result should not have errors")
	}

	r.Findings = append(r.Findings, Finding{Level: SeverityError})
	if !r.HasErrors() {
		t.Error("result with error should have errors")
	}
}

func TestLintResult_HasWarnings(t *testing.T) {
	r := &LintResult{}
	if r.HasWarnings() {
		t.Error("empty result should not have warnings")
	}

	r.Findings = []Finding{{Level: SeverityInfo}}
	if r.HasWarnings() {
		t.Error("info-only result should not have warnings")
	}

	r.Findings = []Finding{{Level: SeverityWarning}}
	if !r.HasWarnings() {
		t.Error("result with warning should have warnings")
	}

	r.Findings = []Finding{{Level: SeverityError}}
	if !r.HasWarnings() {
		t.Error("result with error should also count as having warnings")
	}
}

func TestCountSeverities(t *testing.T) {
	tests := []struct {
		name         string
		findings     []Finding
		wantErrors   int
		wantWarnings int
		wantInfos    int
	}{
		{
			name:     "empty findings",
			findings: nil,
		},
		{
			name: "all errors",
			findings: []Finding{
				{Level: SeverityError, Rule: "a"},
				{Level: SeverityError, Rule: "b"},
			},
			wantErrors: 2,
		},
		{
			name: "mixed severities",
			findings: []Finding{
				{Level: SeverityError, Rule: "e1"},
				{Level: SeverityWarning, Rule: "w1"},
				{Level: SeverityWarning, Rule: "w2"},
				{Level: SeverityInfo, Rule: "i1"},
			},
			wantErrors:   1,
			wantWarnings: 2,
			wantInfos:    1,
		},
		{
			name: "all info",
			findings: []Finding{
				{Level: SeverityInfo, Rule: "i1"},
				{Level: SeverityInfo, Rule: "i2"},
				{Level: SeverityInfo, Rule: "i3"},
			},
			wantInfos: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := countSeverities(tt.findings)
			if got.Errors != tt.wantErrors {
				t.Errorf("Errors = %d, want %d", got.Errors, tt.wantErrors)
			}
			if got.Warnings != tt.wantWarnings {
				t.Errorf("Warnings = %d, want %d", got.Warnings, tt.wantWarnings)
			}
			if got.Infos != tt.wantInfos {
				t.Errorf("Infos = %d, want %d", got.Infos, tt.wantInfos)
			}
		})
	}
}

func TestSeverityCountsFormatCounts(t *testing.T) {
	tests := []struct {
		name   string
		counts severityCounts
		want   string
	}{
		{
			name: "all zeros",
			want: "",
		},
		{
			name:   "errors only",
			counts: severityCounts{Errors: 3},
			want:   "3 error(s)",
		},
		{
			name:   "warnings only",
			counts: severityCounts{Warnings: 2},
			want:   "2 warning(s)",
		},
		{
			name:   "infos only",
			counts: severityCounts{Infos: 5},
			want:   "5 info",
		},
		{
			name:   "all three",
			counts: severityCounts{Errors: 1, Warnings: 2, Infos: 3},
			want:   "1 error(s), 2 warning(s), 3 info",
		},
		{
			name:   "errors and infos",
			counts: severityCounts{Errors: 4, Infos: 1},
			want:   "4 error(s), 1 info",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.counts.formatCounts()
			if got != tt.want {
				t.Errorf("formatCounts() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestToolsSummary(t *testing.T) {
	tests := []struct {
		name              string
		hadolintAvailable bool
		trivyAvailable    bool
		wantContains      []string
		wantNotContains   []string
	}{
		{
			name:            "no external tools",
			wantContains:    []string{"4 built-in rules"},
			wantNotContains: []string{"hadolint", "trivy"},
		},
		{
			name:              "hadolint only",
			hadolintAvailable: true,
			wantContains:      []string{"4 built-in rules", "hadolint"},
			wantNotContains:   []string{"trivy"},
		},
		{
			name:            "trivy only",
			trivyAvailable:  true,
			wantContains:    []string{"4 built-in rules", "trivy"},
			wantNotContains: []string{"hadolint"},
		},
		{
			name:              "both tools",
			hadolintAvailable: true,
			trivyAvailable:    true,
			wantContains:      []string{"4 built-in rules", "hadolint", "trivy"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &LintResult{
				HadolintAvailable: tt.hadolintAvailable,
				TrivyAvailable:    tt.trivyAvailable,
			}
			got := r.toolsSummary()
			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("toolsSummary() = %q, want it to contain %q", got, want)
				}
			}
			for _, notWant := range tt.wantNotContains {
				if strings.Contains(got, notWant) {
					t.Errorf("toolsSummary() = %q, want it NOT to contain %q", got, notWant)
				}
			}
		})
	}
}
