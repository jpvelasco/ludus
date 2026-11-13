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

	// These explicit contexts identify account IDs in offline destroy plans.
	previewAccountRe = regexp.MustCompile(`(Account \[)(\d{12})(\])`)
	buildBucketRe    = regexp.MustCompile(`(\bludus-builds-)(\d{12})\b`)
)

// MaskAccountIDs replaces 12-digit AWS account IDs that appear inside an ECR
// hostname, ARN, preview account field, or account-derived build bucket with
// a fixed-width mask. Other numbers (timestamps, build IDs) are left untouched.
func MaskAccountIDs(s string) string {
	s = ecrAccountRe.ReplaceAllString(s, mask+"$2")
	s = arnAccountRe.ReplaceAllString(s, "${1}"+mask+"$3")
	s = previewAccountRe.ReplaceAllString(s, "${1}"+mask+"$3")
	s = buildBucketRe.ReplaceAllString(s, "${1}"+mask)
	return s
}
