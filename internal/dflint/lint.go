package dflint

import (
	"fmt"
	"strings"
)

// Severity represents the severity of a finding.
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
	SeverityInfo    Severity = "info"
)

// Finding represents a single security or best-practice issue.
type Finding struct {
	Source  string   // "builtin", "hadolint", "trivy"
	Rule    string   // e.g. "no-root-user", "DL3006", "CVE-2024-..."
	Line    int      // 0 if not line-specific
	Level   Severity // "error", "warning", "info"
	Message string
}

// LintResult aggregates findings from all sources.
type LintResult struct {
	Findings          []Finding
	HadolintAvailable bool
	TrivyAvailable    bool
}

// HasErrors returns true if any finding has error severity.
func (r *LintResult) HasErrors() bool {
	for _, f := range r.Findings {
		if f.Level == SeverityError {
			return true
		}
	}
	return false
}

// HasWarnings returns true if any finding has warning or error severity.
func (r *LintResult) HasWarnings() bool {
	for _, f := range r.Findings {
		if f.Level == SeverityWarning || f.Level == SeverityError {
			return true
		}
	}
	return false
}

// FindingsDetail returns a formatted string per finding for display.
func (r *LintResult) FindingsDetail() []string {
	var lines []string
	for _, f := range r.Findings {
		prefix := string(f.Level)
		loc := ""
		if f.Line > 0 {
			loc = fmt.Sprintf(" (line %d)", f.Line)
		}
		lines = append(lines, fmt.Sprintf("[%s] %s%s: %s", prefix, f.Rule, loc, f.Message))
	}
	return lines
}

// severityCounts holds counts of findings by severity level.
type severityCounts struct {
	Errors   int
	Warnings int
	Infos    int
}

// countSeverities tallies findings by severity level.
func countSeverities(findings []Finding) severityCounts {
	var c severityCounts
	for _, f := range findings {
		switch f.Level {
		case SeverityError:
			c.Errors++
		case SeverityWarning:
			c.Warnings++
		case SeverityInfo:
			c.Infos++
		}
	}
	return c
}

// formatCounts returns a comma-separated summary string.
func (c severityCounts) formatCounts() string {
	var parts []string
	if c.Errors > 0 {
		parts = append(parts, fmt.Sprintf("%d error(s)", c.Errors))
	}
	if c.Warnings > 0 {
		parts = append(parts, fmt.Sprintf("%d warning(s)", c.Warnings))
	}
	if c.Infos > 0 {
		parts = append(parts, fmt.Sprintf("%d info", c.Infos))
	}
	return strings.Join(parts, ", ")
}

// Summary returns a human-readable summary of findings.
func (r *LintResult) Summary() string {
	if len(r.Findings) == 0 {
		return fmt.Sprintf("no issues (%s)", r.toolsSummary())
	}
	return countSeverities(r.Findings).formatCounts()
}

// toolsSummary returns a string describing which tools were used.
func (r *LintResult) toolsSummary() string {
	tools := "4 built-in rules"
	if r.HadolintAvailable {
		tools += " + hadolint"
	}
	if r.TrivyAvailable {
		tools += " + trivy"
	}
	return tools
}

// LintDockerfile runs built-in rules and hadolint (if available) against Dockerfile content.
func LintDockerfile(content string) *LintResult {
	result := &LintResult{}

	// Run built-in rules
	result.Findings = append(result.Findings, checkNoRootUser(content)...)
	result.Findings = append(result.Findings, checkUnpinnedBaseImage(content)...)
	result.Findings = append(result.Findings, checkNoPackageCleanup(content)...)
	result.Findings = append(result.Findings, checkSensitiveEnv(content)...)

	// Run hadolint if available
	hadolintFindings, available := runHadolint(content)
	result.HadolintAvailable = available
	result.Findings = append(result.Findings, hadolintFindings...)

	return result
}

// LintImage runs trivy (if available) to scan a container image for vulnerabilities.
func LintImage(imageRef string) *LintResult {
	result := &LintResult{}

	trivyFindings, available := runTrivy(imageRef)
	result.TrivyAvailable = available
	result.Findings = append(result.Findings, trivyFindings...)

	return result
}
