//go:build !windows

package game

// readSystemEnvVar is a no-op on non-Windows platforms. On Linux/macOS,
// environment variables set by installers are immediately available in the
// current shell (no registry indirection).
func readSystemEnvVar(_ string) string {
	return ""
}
