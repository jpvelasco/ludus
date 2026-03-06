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

	// Check image exists locally before scanning
	dockerPath, err := exec.LookPath("docker")
	if err != nil {
		return nil, true
	}
	inspectCmd := exec.Command(dockerPath, "image", "inspect", imageRef)
	inspectCmd.Stdout = nil
	inspectCmd.Stderr = nil
	if err := inspectCmd.Run(); err != nil {
		// Image doesn't exist locally — skip scan
		return nil, true
	}

	cmd := exec.Command(path, "image",
		"--format", "json",
		"--severity", "HIGH,CRITICAL",
		"--quiet",
		imageRef,
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// Trivy may exit non-zero when vulnerabilities are found
		if stdout.Len() == 0 {
			return nil, true
		}
	}

	var output trivyOutput
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		return nil, true
	}

	var findings []Finding
	critCount := 0
	highCount := 0
	const maxPerSeverity = 5

	for _, result := range output.Results {
		for _, vuln := range result.Vulnerabilities {
			severity := strings.ToUpper(vuln.Severity)

			// Limit display to top 5 per severity level
			switch severity {
			case "CRITICAL":
				critCount++
				if critCount > maxPerSeverity {
					continue
				}
			case "HIGH":
				highCount++
				if highCount > maxPerSeverity {
					continue
				}
			}

			msg := vuln.Title
			if vuln.PkgName != "" {
				msg = fmt.Sprintf("%s (%s)", vuln.Title, vuln.PkgName)
			}

			findings = append(findings, Finding{
				Source:  "trivy",
				Rule:    vuln.VulnerabilityID,
				Level:   mapTrivySeverity(severity),
				Message: msg,
			})
		}
	}

	// Add overflow notes if we truncated
	if critCount > maxPerSeverity {
		findings = append(findings, Finding{
			Source:  "trivy",
			Rule:    "overflow",
			Level:   SeverityInfo,
			Message: fmt.Sprintf("... and %d more CRITICAL vulnerabilities", critCount-maxPerSeverity),
		})
	}
	if highCount > maxPerSeverity {
		findings = append(findings, Finding{
			Source:  "trivy",
			Rule:    "overflow",
			Level:   SeverityInfo,
			Message: fmt.Sprintf("... and %d more HIGH vulnerabilities", highCount-maxPerSeverity),
		})
	}

	return findings, true
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
