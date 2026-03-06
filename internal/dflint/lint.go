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

// Summary returns a human-readable summary of findings.
func (r *LintResult) Summary() string {
	if len(r.Findings) == 0 {
		tools := "4 built-in rules"
		if r.HadolintAvailable {
			tools += " + hadolint"
		}
		if r.TrivyAvailable {
			tools += " + trivy"
		}
		return fmt.Sprintf("no issues (%s)", tools)
	}

	errors := 0
	warnings := 0
	infos := 0
	for _, f := range r.Findings {
		switch f.Level {
		case SeverityError:
			errors++
		case SeverityWarning:
			warnings++
		case SeverityInfo:
			infos++
		}
	}

	var parts []string
	if errors > 0 {
		parts = append(parts, fmt.Sprintf("%d error(s)", errors))
	}
	if warnings > 0 {
		parts = append(parts, fmt.Sprintf("%d warning(s)", warnings))
	}
	if infos > 0 {
		parts = append(parts, fmt.Sprintf("%d info", infos))
	}
	return strings.Join(parts, ", ")
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

// checkNoRootUser warns if the last USER instruction is root or absent.
func checkNoRootUser(content string) []Finding {
	lines := strings.Split(content, "\n")
	lastUser := ""
	lastUserLine := 0

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToUpper(trimmed), "USER ") {
			lastUser = strings.TrimSpace(trimmed[5:])
			lastUserLine = i + 1
		}
	}

	if lastUser == "" {
		return []Finding{{
			Source:  "builtin",
			Rule:    "no-root-user",
			Level:   SeverityWarning,
			Message: "no USER instruction found; container will run as root",
		}}
	}

	if lastUser == "root" || lastUser == "0" {
		return []Finding{{
			Source:  "builtin",
			Rule:    "no-root-user",
			Line:    lastUserLine,
			Level:   SeverityWarning,
			Message: "last USER instruction sets root; container should run as non-root",
		}}
	}

	return nil
}

// checkUnpinnedBaseImage warns if FROM uses :latest or no tag.
func checkUnpinnedBaseImage(content string) []Finding {
	lines := strings.Split(content, "\n")
	var findings []Finding

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		upper := strings.ToUpper(trimmed)
		if !strings.HasPrefix(upper, "FROM ") {
			continue
		}

		// Parse the image reference (skip "FROM " prefix)
		parts := strings.Fields(trimmed)
		if len(parts) < 2 {
			continue
		}
		imageRef := parts[1]

		// Skip build stages ("FROM ... AS ...")
		// but still check the image ref

		// Check for :latest or no tag
		if strings.HasSuffix(imageRef, ":latest") {
			findings = append(findings, Finding{
				Source:  "builtin",
				Rule:    "unpinned-base-image",
				Line:    i + 1,
				Level:   SeverityWarning,
				Message: fmt.Sprintf("base image %q uses :latest tag; pin to a specific version", imageRef),
			})
		} else if !strings.Contains(imageRef, ":") && !strings.Contains(imageRef, "@") {
			findings = append(findings, Finding{
				Source:  "builtin",
				Rule:    "unpinned-base-image",
				Line:    i + 1,
				Level:   SeverityWarning,
				Message: fmt.Sprintf("base image %q has no tag; pin to a specific version", imageRef),
			})
		}
	}

	return findings
}

// checkNoPackageCleanup warns if apt-get install or dnf install runs without cleanup in the same RUN block.
func checkNoPackageCleanup(content string) []Finding {
	// Parse RUN blocks (may span multiple lines with \ continuation)
	runBlocks := parseRunBlocks(content)
	var findings []Finding

	for _, block := range runBlocks {
		text := block.text

		if strings.Contains(text, "apt-get install") && !strings.Contains(text, "rm -rf /var/lib/apt/lists") {
			findings = append(findings, Finding{
				Source:  "builtin",
				Rule:    "no-package-cleanup",
				Line:    block.startLine,
				Level:   SeverityWarning,
				Message: "apt-get install without 'rm -rf /var/lib/apt/lists/*' increases image size",
			})
		}

		if strings.Contains(text, "dnf install") && !strings.Contains(text, "dnf clean all") {
			findings = append(findings, Finding{
				Source:  "builtin",
				Rule:    "no-package-cleanup",
				Line:    block.startLine,
				Level:   SeverityWarning,
				Message: "dnf install without 'dnf clean all' increases image size",
			})
		}
	}

	return findings
}

// checkSensitiveEnv flags ENV keys that look like secrets.
func checkSensitiveEnv(content string) []Finding {
	lines := strings.Split(content, "\n")
	var findings []Finding

	sensitiveKeys := []string{"PASSWORD", "SECRET", "TOKEN", "KEY"}

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		upper := strings.ToUpper(trimmed)
		if !strings.HasPrefix(upper, "ENV ") {
			continue
		}

		// Parse ENV instruction: "ENV KEY=value" or "ENV KEY value"
		envPart := strings.TrimSpace(trimmed[4:])
		var envKey string
		if idx := strings.IndexAny(envPart, "= "); idx > 0 {
			envKey = envPart[:idx]
		} else {
			envKey = envPart
		}

		upperKey := strings.ToUpper(envKey)
		for _, sensitive := range sensitiveKeys {
			if strings.Contains(upperKey, sensitive) {
				findings = append(findings, Finding{
					Source:  "builtin",
					Rule:    "sensitive-env",
					Line:    i + 1,
					Level:   SeverityError,
					Message: fmt.Sprintf("ENV %q may contain a secret; use build args or runtime secrets instead", envKey),
				})
				break
			}
		}
	}

	return findings
}

// runBlock represents a RUN instruction with its text and starting line.
type runBlock struct {
	text      string
	startLine int
}

// parseRunBlocks extracts RUN instructions from Dockerfile content,
// handling line continuations with backslash.
func parseRunBlocks(content string) []runBlock {
	lines := strings.Split(content, "\n")
	var blocks []runBlock

	i := 0
	for i < len(lines) {
		trimmed := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(strings.ToUpper(trimmed), "RUN ") {
			i++
			continue
		}

		startLine := i + 1
		// Collect the full RUN block including continuations
		var blockLines []string
		blockLines = append(blockLines, trimmed[4:]) // skip "RUN "

		for strings.HasSuffix(strings.TrimSpace(lines[i]), "\\") && i+1 < len(lines) {
			i++
			blockLines = append(blockLines, lines[i])
		}

		blocks = append(blocks, runBlock{
			text:      strings.Join(blockLines, "\n"),
			startLine: startLine,
		})
		i++
	}

	return blocks
}
