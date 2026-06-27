package ddc

import (
	"fmt"
	"strings"
)

// DDC mode constants. Use these instead of raw string comparisons.
const (
	ModeZen   = "zen"   // Unreal Zen Store DDC (default; UE's default local backend since 5.4).
	ModeLocal = "local" // Legacy FileSystem DDC (deprecated; delete-only in UE since 5.4).
	ModeNone  = "none"  // DDC disabled; no cache volume mounted.
)

// ValidateDDCMode returns the normalized mode or an error for unknown values.
// Empty string is normalized to ModeZen (the default): Unreal Engine uses the
// Zen Store as its default local DDC backend from UE 5.4 onward (the legacy
// FileSystem DDC is delete-only since 5.4), and Ludus supports UE 5.4+.
func ValidateDDCMode(mode string) (string, error) {
	switch mode {
	case "", ModeZen:
		return ModeZen, nil
	case ModeLocal:
		return ModeLocal, nil
	case ModeNone:
		return ModeNone, nil
	default:
		return "", fmt.Errorf("invalid DDC mode %q: valid values are %q (Zen Store, default), %q (legacy FileSystem cache, deprecated), or %q (disable cache)", mode, ModeZen, ModeLocal, ModeNone)
	}
}

// LocalModeDeprecationWarning is the user-facing message shown when an explicit
// ddc.mode: local is in effect. Surfaced from config load, ludus doctor, and
// ludus ddc status so the guidance is consistent everywhere.
const LocalModeDeprecationWarning = "ddc.mode: local uses the legacy FileSystem DDC (delete-only since UE 5.4). " +
	"Zen is now the default and recommended for best performance — set ddc.mode: zen."

// EnvOverride returns the environment variable string that redirects UE5's
// local DDC backend to path. UE5's BaseEngine.ini configures the Local backend
// with EnvPathOverride=UE-LocalDataCachePath, so setting this env var overrides
// the default path without modifying any project or engine files.
//
// Uses strings.ReplaceAll instead of filepath.ToSlash because ToSlash is a
// no-op on Linux (backslash is a valid filename char, not a separator), but
// Windows paths passed here may still contain backslashes that Docker and
// UE5 need converted to forward slashes.
func EnvOverride(path string) string {
	normalized := strings.ReplaceAll(path, `\`, "/")
	return fmt.Sprintf("UE-LocalDataCachePath=%s", normalized)
}

// ZenContainerPath is the fixed path inside the container where UE5 writes
// its ZenStore data. This resolves from the Zen.AutoLaunch DataPath template
// (%ENGINEVERSIONAGNOSTICINSTALLEDUSERDIR%Zen/Data) under the ue user's home,
// where %ENGINEVERSIONAGNOSTICINSTALLEDUSERDIR% resolves to
// ~/.config/Epic/UnrealEngine/Common/, giving the full path below.
// UE uses the Zen Store as its default local DDC backend from 5.4 onward, so
// mounting a host directory here persists the cook DDC across --rm container runs.
const ZenContainerPath = "/home/ue/.config/Epic/UnrealEngine/Common/Zen/Data"
