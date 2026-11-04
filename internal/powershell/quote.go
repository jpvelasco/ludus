package powershell

import "strings"

// QuoteSingleQuoted returns a PowerShell single-quoted string literal.
func QuoteSingleQuoted(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}
