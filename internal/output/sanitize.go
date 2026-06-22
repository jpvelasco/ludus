// Package output provides terminal-output sanitization for ludus, masking
// sensitive identifiers (AWS account IDs) from human-readable output while
// leaving JSON/MCP output untouched.
package output

import "regexp"

// mask is the placeholder substituted for a 12-digit AWS account ID.
const mask = "************"

var (
	// ecrAccountRe matches the account ID at the start of an ECR registry host,
	// e.g. "123456789012.dkr.ecr.us-east-1.amazonaws.com".
	ecrAccountRe = regexp.MustCompile(`(\d{12})(\.dkr\.ecr\.)`)

	// arnAccountRe matches the account ID field (the 5th colon-delimited field)
	// of an ARN, e.g. "arn:aws:gamelift:us-east-1:123456789012:fleet/...".
	arnAccountRe = regexp.MustCompile(`(arn:[a-z0-9-]*:[a-z0-9-]*:[a-z0-9-]*:)(\d{12})(:)`)
)

// MaskAccountIDs replaces 12-digit AWS account IDs that appear inside an ECR
// hostname or an ARN with a fixed-width mask. Bare 12-digit numbers in other
// contexts (timestamps, build IDs) are left untouched to avoid false positives.
func MaskAccountIDs(s string) string {
	s = ecrAccountRe.ReplaceAllString(s, mask+"$2")
	s = arnAccountRe.ReplaceAllString(s, "${1}"+mask+"$3")
	return s
}
