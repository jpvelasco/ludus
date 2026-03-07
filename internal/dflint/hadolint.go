package dflint

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"strings"
)

// hadolintResult represents a single hadolint JSON output entry.
type hadolintResult struct {
	Line    int    `json:"line"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Level   string `json:"level"`
}

// runHadolint runs hadolint on Dockerfile content via stdin.
// Returns findings and whether hadolint was available.
func runHadolint(content string) ([]Finding, bool) {
	path, err := exec.LookPath("hadolint")
	if err != nil {
		return nil, false
	}

	cmd := exec.Command(path, "--format", "json", "-")
	cmd.Stdin = strings.NewReader(content)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Hadolint exits 1 when it finds issues — still parse stdout
	_ = cmd.Run()

	if stdout.Len() == 0 {
		return nil, true
	}

	var results []hadolintResult
	if err := json.Unmarshal(stdout.Bytes(), &results); err != nil {
		return nil, true
	}

	var findings []Finding
	for _, r := range results {
		findings = append(findings, Finding{
			Source:  "hadolint",
			Rule:    r.Code,
			Line:    r.Line,
			Level:   mapHadolintLevel(r.Level),
			Message: r.Message,
		})
	}

	return findings, true
}

// mapHadolintLevel converts hadolint severity names to our Severity type.
func mapHadolintLevel(level string) Severity {
	switch strings.ToLower(level) {
	case "error":
		return SeverityError
	case "warning":
		return SeverityWarning
	default:
		return SeverityInfo
	}
}
