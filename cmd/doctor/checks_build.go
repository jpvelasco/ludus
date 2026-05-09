package doctor

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/devrecon/ludus/internal/cache"
	"github.com/devrecon/ludus/internal/config"
	"github.com/devrecon/ludus/internal/state"
)

// checkStaleBuildArtifacts looks for build artifacts that might be from a
// different engine version.
func checkStaleBuildArtifacts(cfg *config.Config) diagnostic {
	d := diagnostic{name: "Build Artifacts"}

	if cfg.Engine.SourcePath == "" {
		d.status = "ok"
		d.message = "skipped — no engine source configured"
		return d
	}

	// Check if UnrealEditor exists but is very old (> 30 days)
	var editorPath string
	if runtime.GOOS == "windows" {
		editorPath = filepath.Join(cfg.Engine.SourcePath, "Engine", "Binaries", "Win64", "UnrealEditor.exe")
	} else {
		editorPath = filepath.Join(cfg.Engine.SourcePath, "Engine", "Binaries", "Linux", "UnrealEditor")
	}

	info, err := os.Stat(editorPath)
	if err != nil {
		d.status = "ok"
		d.message = "no engine build found (clean state)"
		return d
	}

	age := time.Since(info.ModTime())
	if age > 30*24*time.Hour {
		d.status = "warn"
		d.message = fmt.Sprintf("engine build is %d days old; consider rebuilding", int(age.Hours()/24))
		return d
	}

	d.status = "ok"
	d.message = fmt.Sprintf("engine build is %d days old", int(age.Hours()/24))
	return d
}

// checkBuildState verifies state.json consistency — checks if referenced
// files and directories still exist.
func checkBuildState() diagnostic {
	st, err := state.Load()
	if err != nil {
		return diagnostic{name: "Build State", status: "warn", message: "could not read .ludus/state.json"}
	}

	var issues []string
	if issue := clientBinaryIssue(st); issue != "" {
		issues = append(issues, issue)
	}
	if issue := fleetStateIssue(st); issue != "" {
		issues = append(issues, issue)
	}

	if len(issues) > 0 {
		return diagnostic{name: "Build State", status: "warn", message: strings.Join(issues, "; ")}
	}
	return diagnostic{name: "Build State", status: "ok", message: "state references are consistent"}
}

// clientBinaryIssue returns a warning if the client binary path is set but the file is missing.
func clientBinaryIssue(st *state.State) string {
	if st.Client == nil || st.Client.BinaryPath == "" {
		return ""
	}
	if _, err := os.Stat(st.Client.BinaryPath); err != nil {
		if os.IsNotExist(err) {
			return "client binary missing: " + st.Client.BinaryPath
		}
		return fmt.Sprintf("client binary error: %v", err)
	}
	return ""
}

// fleetStateIssue returns a warning if deploy is active but no fleet state exists.
func fleetStateIssue(st *state.State) string {
	if st.Deploy == nil || st.Deploy.Status != "active" {
		return ""
	}
	if st.Fleet != nil || st.EC2Fleet != nil || st.Anywhere != nil {
		return ""
	}
	return "deploy marked active but no fleet state found"
}

// checkCacheIntegrity verifies the build cache is readable.
func checkCacheIntegrity() diagnostic {
	d := diagnostic{name: "Build Cache"}

	c, err := cache.Load()
	if err != nil {
		d.status = "warn"
		d.message = fmt.Sprintf("cache unreadable: %v; builds will re-run from scratch", err)
		return d
	}

	_ = c // cache loaded successfully — that's all we need to verify

	d.status = "ok"
	d.message = "cache file readable"
	return d
}
