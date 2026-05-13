package doctor

import (
	"testing"

	"github.com/jpvelasco/ludus/internal/dflint"
)

func TestLintResultToDiagnostic(t *testing.T) {
	tests := []struct {
		name       string
		result     *dflint.LintResult
		wantStatus string
	}{
		{
			name:       "no findings is ok",
			result:     &dflint.LintResult{HadolintAvailable: true},
			wantStatus: "ok",
		},
		{
			name: "warnings only maps to warn",
			result: &dflint.LintResult{
				HadolintAvailable: true,
				Findings:          []dflint.Finding{{Level: dflint.SeverityWarning, Rule: "W1"}},
			},
			wantStatus: "warn",
		},
		{
			name: "errors map to fail",
			result: &dflint.LintResult{
				HadolintAvailable: true,
				Findings:          []dflint.Finding{{Level: dflint.SeverityError, Rule: "E1"}},
			},
			wantStatus: "fail",
		},
		{
			name: "mixed errors and warnings maps to fail",
			result: &dflint.LintResult{
				HadolintAvailable: true,
				Findings: []dflint.Finding{
					{Level: dflint.SeverityWarning, Rule: "W1"},
					{Level: dflint.SeverityError, Rule: "E1"},
				},
			},
			wantStatus: "fail",
		},
		{
			name:       "hadolint missing appends message",
			result:     &dflint.LintResult{HadolintAvailable: false},
			wantStatus: "ok",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := lintResultToDiagnostic("test", tt.result)
			if d.status != tt.wantStatus {
				t.Errorf("status = %q, want %q", d.status, tt.wantStatus)
			}
		})
	}
}

func TestDowngradeRule(t *testing.T) {
	tests := []struct {
		name       string
		findings   []dflint.Finding
		rule       string
		level      dflint.Severity
		wantLevels []dflint.Severity
	}{
		{
			name: "downgrades matching rule",
			findings: []dflint.Finding{
				{Rule: "no-root-user", Level: dflint.SeverityWarning},
				{Rule: "other-rule", Level: dflint.SeverityWarning},
			},
			rule:       "no-root-user",
			level:      dflint.SeverityInfo,
			wantLevels: []dflint.Severity{dflint.SeverityInfo, dflint.SeverityWarning},
		},
		{
			name: "no match leaves findings unchanged",
			findings: []dflint.Finding{
				{Rule: "other-rule", Level: dflint.SeverityError},
			},
			rule:       "no-root-user",
			level:      dflint.SeverityInfo,
			wantLevels: []dflint.Severity{dflint.SeverityError},
		},
		{
			name:       "empty findings is a no-op",
			findings:   nil,
			rule:       "no-root-user",
			level:      dflint.SeverityInfo,
			wantLevels: nil,
		},
		{
			name: "downgrades all matching findings",
			findings: []dflint.Finding{
				{Rule: "no-root-user", Level: dflint.SeverityWarning},
				{Rule: "no-root-user", Level: dflint.SeverityError},
			},
			rule:       "no-root-user",
			level:      dflint.SeverityInfo,
			wantLevels: []dflint.Severity{dflint.SeverityInfo, dflint.SeverityInfo},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := &dflint.LintResult{Findings: tt.findings}
			downgradeRule(result, tt.rule, tt.level)
			if len(result.Findings) != len(tt.wantLevels) {
				t.Fatalf("findings len = %d, want %d", len(result.Findings), len(tt.wantLevels))
			}
			for i, f := range result.Findings {
				if f.Level != tt.wantLevels[i] {
					t.Errorf("findings[%d].Level = %v, want %v", i, f.Level, tt.wantLevels[i])
				}
			}
		})
	}
}
