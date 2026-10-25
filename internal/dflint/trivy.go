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

// TrivyScanStatus describes the outcome of a trivy image-scan attempt,
// separating an authoritative completed scan from one that never produced
// findings (#724).
type TrivyScanStatus string

const (
	// TrivyNotInstalled means trivy was not found on PATH.
	TrivyNotInstalled TrivyScanStatus = "not-installed"
	// TrivySkipped means the scan did not run because the image is not
	// available locally.
	TrivySkipped TrivyScanStatus = "skipped"
	// TrivyFailed means trivy ran but produced no usable scan result.
	TrivyFailed TrivyScanStatus = "failed"
	// TrivyScanned means the scan completed and its findings (including an
	// empty result) are authoritative.
	TrivyScanned TrivyScanStatus = "scanned"
)

// trivyScan reports the outcome of one trivy image-scan attempt.
type trivyScan struct {
	findings []Finding
	status   TrivyScanStatus
	reason   string
}

// runTrivy runs trivy image scan on the given image reference. The returned
// status distinguishes a completed scan from a skipped or failed one so
// callers never mistake a missing scan for a clean one (#724).
func runTrivy(imageRef string) trivyScan {
	path, err := exec.LookPath("trivy")
	if err != nil {
		return trivyScan{status: TrivyNotInstalled}
	}

	present, reason := imageExistsLocally(imageRef)
	if !present {
		if reason == "" {
			reason = "image not present locally"
		}
		return trivyScan{status: TrivySkipped, reason: reason}
	}

	output, failReason := execTrivyScan(path, imageRef)
	if failReason != "" {
		return trivyScan{status: TrivyFailed, reason: failReason}
	}
	return trivyScan{findings: parseVulnerabilities(output), status: TrivyScanned}
}

// imageExistsLocally checks whether the Docker image is available locally.
// A false result with a non-empty reason means the check itself could not
// run (e.g. docker is not installed).
func imageExistsLocally(imageRef string) (bool, string) {
	dockerPath, err := exec.LookPath("docker")
	if err != nil {
		return false, "docker not available"
	}
	inspectCmd := exec.Command(dockerPath, "image", "inspect", imageRef)
	inspectCmd.Stdout = nil
	inspectCmd.Stderr = nil
	if err := inspectCmd.Run(); err != nil {
		// inspect exits nonzero when the image is absent.
		return false, ""
	}
	return true, ""
}

// execTrivyScan runs trivy and parses the JSON output. The returned reason is
// empty when the scan produced a usable result.
func execTrivyScan(trivyPath, imageRef string) (trivyOutput, string) {
	cmd := exec.Command(trivyPath, "image",
		"--format", "json",
		"--severity", "HIGH,CRITICAL",
		"--quiet",
		imageRef,
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil && stdout.Len() == 0 {
		return trivyOutput{}, fmt.Sprintf("trivy scan failed: %v", err)
	}

	var output trivyOutput
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		return trivyOutput{}, fmt.Sprintf("trivy output not parseable: %v", err)
	}
	return output, ""
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
func collectVuln(vuln trivyVuln, counts *vulnCounts, limit int) (Finding, bool) {
	severity := strings.ToUpper(vuln.Severity)

	switch severity {
	case "CRITICAL":
		counts.critical++
		if counts.critical > limit {
			return Finding{}, false
		}
	case "HIGH":
		counts.high++
		if counts.high > limit {
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
func appendOverflowNotes(findings []Finding, counts vulnCounts, limit int) []Finding {
	if counts.critical > limit {
		findings = append(findings, Finding{
			Source:  "trivy",
			Rule:    "overflow",
			Level:   SeverityInfo,
			Message: fmt.Sprintf("... and %d more CRITICAL vulnerabilities", counts.critical-limit),
		})
	}
	if counts.high > limit {
		findings = append(findings, Finding{
			Source:  "trivy",
			Rule:    "overflow",
			Level:   SeverityInfo,
			Message: fmt.Sprintf("... and %d more HIGH vulnerabilities", counts.high-limit),
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
