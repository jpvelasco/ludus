package dflint

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// trivyOutput represents the top-level trivy JSON output.
type trivyOutput struct {
	Results []trivyResult `json:"Results"`
}

// trivyResult represents a single result from trivy scan.
type trivyResult struct {
	Vulnerabilities []trivyVuln `json:"Vulnerabilities"`
}

// trivyVuln represents a single vulnerability found by trivy.
type trivyVuln struct {
	VulnerabilityID string `json:"VulnerabilityID"`
	Severity        string `json:"Severity"`
	Title           string `json:"Title"`
	PkgName         string `json:"PkgName"`
}

// runTrivy runs trivy image scan on the given image reference.
// Returns findings and whether trivy was available.
func runTrivy(imageRef string) ([]Finding, bool) {
	path, err := exec.LookPath("trivy")
	if err != nil {
		return nil, false
	}

	if !imageExistsLocally(imageRef) {
		return nil, true
	}

	output, ok := execTrivyScan(path, imageRef)
	if !ok {
		return nil, true
	}

	return parseVulnerabilities(output), true
}

// imageExistsLocally checks whether the Docker image is available locally.
func imageExistsLocally(imageRef string) bool {
	dockerPath, err := exec.LookPath("docker")
	if err != nil {
		return false
	}
	inspectCmd := exec.Command(dockerPath, "image", "inspect", imageRef)
	inspectCmd.Stdout = nil
	inspectCmd.Stderr = nil
	return inspectCmd.Run() == nil
}

// execTrivyScan runs trivy and parses the JSON output.
func execTrivyScan(trivyPath, imageRef string) (trivyOutput, bool) {
	cmd := exec.Command(trivyPath, "image",
		"--format", "json",
		"--severity", "HIGH,CRITICAL",
		"--quiet",
		imageRef,
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if stdout.Len() == 0 {
			return trivyOutput{}, false
		}
	}

	var output trivyOutput
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		return trivyOutput{}, false
	}
	return output, true
}

// vulnCounts tracks how many vulnerabilities have been seen per severity.
type vulnCounts struct {
	critical int
	high     int
}

// parseVulnerabilities collects findings from trivy output, capping each
// severity level at maxPerSeverity and appending overflow notes.
func parseVulnerabilities(output trivyOutput) []Finding {
	const maxPerSeverity = 5
	var counts vulnCounts
	var findings []Finding

	for _, result := range output.Results {
		for _, vuln := range result.Vulnerabilities {
			if f, ok := collectVuln(vuln, &counts, maxPerSeverity); ok {
				findings = append(findings, f)
			}
		}
	}
	return appendOverflowNotes(findings, counts, maxPerSeverity)
}

// collectVuln evaluates a single vulnerability against the per-severity cap.
// Returns the finding and true if it should be included.
func collectVuln(vuln trivyVuln, counts *vulnCounts, max int) (Finding, bool) {
	severity := strings.ToUpper(vuln.Severity)

	switch severity {
	case "CRITICAL":
		counts.critical++
		if counts.critical > max {
			return Finding{}, false
		}
	case "HIGH":
		counts.high++
		if counts.high > max {
			return Finding{}, false
		}
	}

	msg := vuln.Title
	if vuln.PkgName != "" {
		msg = fmt.Sprintf("%s (%s)", vuln.Title, vuln.PkgName)
	}

	return Finding{
		Source:  "trivy",
		Rule:    vuln.VulnerabilityID,
		Level:   mapTrivySeverity(severity),
		Message: msg,
	}, true
}

// appendOverflowNotes adds summary findings when vulnerabilities were truncated.
func appendOverflowNotes(findings []Finding, counts vulnCounts, max int) []Finding {
	if counts.critical > max {
		findings = append(findings, Finding{
			Source:  "trivy",
			Rule:    "overflow",
			Level:   SeverityInfo,
			Message: fmt.Sprintf("... and %d more CRITICAL vulnerabilities", counts.critical-max),
		})
	}
	if counts.high > max {
		findings = append(findings, Finding{
			Source:  "trivy",
			Rule:    "overflow",
			Level:   SeverityInfo,
			Message: fmt.Sprintf("... and %d more HIGH vulnerabilities", counts.high-max),
		})
	}
	return findings
}

// mapTrivySeverity converts trivy severity to our Severity type.
func mapTrivySeverity(severity string) Severity {
	switch severity {
	case "CRITICAL":
		return SeverityError
	case "HIGH":
		return SeverityWarning
	default:
		return SeverityInfo
	}
}
