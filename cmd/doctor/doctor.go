package doctor

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/jpvelasco/ludus/cmd/globals"
	"github.com/spf13/cobra"
)

// Cmd is the doctor command for comprehensive diagnostics.
var Cmd = &cobra.Command{
	Use:   "doctor",
	Short: "Run comprehensive diagnostics on the Ludus environment",
	Long: `Performs deeper diagnostics beyond 'ludus init', including:

  - Stale DLLs or build artifacts from previous engine versions
  - Toolchain version mismatch (env vs registry vs engine requirement)
  - Disk space for upcoming builds
  - Partial or corrupted build state
  - AWS credential expiry
  - Build cache integrity
  - Common misconfigurations

Use this when something isn't working and 'ludus init' passes.`,
	RunE: runDoctor,
}

// diagnostic represents a single diagnostic check result.
type diagnostic struct {
	name    string
	status  string // "ok", "warn", "fail"
	message string
	details []string // optional per-finding detail lines
}

// diagnosticResult is the --json wire form of a diagnostic.
type diagnosticResult struct {
	Name    string   `json:"name"`
	Status  string   `json:"status"`
	Message string   `json:"message"`
	Details []string `json:"details,omitempty"`
}

func runDoctor(cmd *cobra.Command, args []string) error {
	cfg := globals.Cfg

	var checks []diagnostic

	checks = append(checks, checkToolchainConsistency(cfg))
	checks = append(checks, checkStaleBuildArtifacts(cfg))
	checks = append(checks, checkBuildState())
	checks = append(checks, checkCacheIntegrity())
	checks = append(checks, checkDDCMode(cfg))
	checks = append(checks, checkDiskSpace(cfg))
	checks = append(checks, checkAWSCredentialExpiry())
	checks = append(checks, checkDockerDaemon())
	checks = append(checks, checkAppleSiliconContainer(cfg))
	checks = append(checks, checkDockerfileSecurity(cfg)...)
	checks = append(checks, checkGitState())
	checks = append(checks, checkAccountIDMasking(cfg))
	checks = append(checks, checkOTLPTracing(cfg))

	if globals.JSONOutput {
		return printDiagnosticsJSON(checks)
	}

	fmt.Println("Running diagnostics...")
	fmt.Println()
	return printDiagnostics(checks)
}

// printDiagnosticsJSON encodes the check results as JSON on stdout, returning
// an error if any checks failed.
func printDiagnosticsJSON(checks []diagnostic) error {
	results := make([]diagnosticResult, len(checks))
	for i, d := range checks {
		results[i] = diagnosticResult{Name: d.name, Status: d.status, Message: d.message, Details: d.details}
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(results); err != nil {
		return fmt.Errorf("encoding diagnostics: %w", err)
	}
	if fails, _ := countDiagnostics(checks); fails > 0 {
		return fmt.Errorf("%d diagnostic check(s) failed", fails)
	}
	return nil
}

// printDiagnostics formats and prints diagnostic results, returning an error if any checks failed.
func printDiagnostics(checks []diagnostic) error {
	fails, warns := countDiagnostics(checks)

	for _, d := range checks {
		marker := diagnosticMarker(d.status)
		fmt.Printf("  %s %-30s %s\n", marker, d.name, d.message)
		for _, detail := range d.details {
			fmt.Printf("         %-30s   %s\n", "", detail)
		}
	}

	fmt.Println()
	return formatDiagnosticSummary(fails, warns)
}

// countDiagnostics counts failures and warnings in the check results.
func countDiagnostics(checks []diagnostic) (fails, warns int) {
	for _, d := range checks {
		switch d.status {
		case "fail":
			fails++
		case "warn":
			warns++
		}
	}
	return
}

// diagnosticMarker returns the display marker for a diagnostic status.
func diagnosticMarker(status string) string {
	switch status {
	case "fail":
		return "[FAIL]"
	case "warn":
		return "[WARN]"
	default:
		return "[OK]  "
	}
}

// formatDiagnosticSummary prints the summary line and returns an error if any checks failed.
func formatDiagnosticSummary(fails, warns int) error {
	if fails > 0 {
		fmt.Printf("%d issue(s) found", fails)
		if warns > 0 {
			fmt.Printf(", %d warning(s)", warns)
		}
		fmt.Println()
		return fmt.Errorf("%d diagnostic check(s) failed", fails)
	}
	if warns > 0 {
		fmt.Printf("No issues found (%d warning(s)).\n", warns)
	} else {
		fmt.Println("No issues found.")
	}
	return nil
}
