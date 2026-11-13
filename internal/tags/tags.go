package tags

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	cftypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	gltypes "github.com/aws/aws-sdk-go-v2/service/gamelift/types"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/jpvelasco/ludus/internal/config"
)

// Build constructs the full tag set from config. It starts with cfg.AWS.Tags,
// adds Project from cfg.Game.ProjectName if not already set, and ensures
// ManagedBy is present. Derived defaults are compared case-insensitively so a
// user key that matches Project or ManagedBy ignoring case wins — IAM tag
// keys are case-insensitive, so adding both spellings would fail CreateRole
// with "Duplicate tag keys found" (#775).
func Build(cfg *config.Config) map[string]string {
	tags := make(map[string]string, len(cfg.AWS.Tags)+2)

	// Start with user-configured tags
	for k, v := range cfg.AWS.Tags {
		tags[k] = v
	}

	// Auto-derive Project from game project name if not set (case-insensitive)
	if !hasKeyFold(tags, "Project") && cfg.Game.ProjectName != "" {
		tags["Project"] = cfg.Game.ProjectName
	}

	// Ensure ManagedBy is always present (case-insensitive)
	if !hasKeyFold(tags, "ManagedBy") {
		tags["ManagedBy"] = "ludus"
	}

	return tags
}

// hasKeyFold reports whether m contains key under any casing. Used to keep
// derived defaults from duplicating an operator-supplied key whose spelling
// differs only by case.
func hasKeyFold(m map[string]string, key string) bool {
	for k := range m {
		if strings.EqualFold(k, key) {
			return true
		}
	}
	return false
}

// Effective returns the tags with case-insensitive duplicates removed; the
// first key encountered (in sorted order, for determinism) wins. IAM rejects
// tag sets whose keys differ only by case, so every To* converter routes
// through this before handing tags to an AWS API (#775).
func Effective(m map[string]string) map[string]string {
	if len(m) == 0 {
		return m
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make(map[string]string, len(m))
	seen := make(map[string]struct{}, len(m))
	for _, k := range keys {
		folded := strings.ToLower(k)
		if _, dup := seen[folded]; dup {
			continue
		}
		seen[folded] = struct{}{}
		out[k] = m[k]
	}
	return out
}

// Lookup returns the effective spelling and value for a case-insensitive key.
func Lookup(m map[string]string, key string) (string, string) {
	for k, v := range Effective(m) {
		if strings.EqualFold(k, key) {
			return k, v
		}
	}
	return key, ""
}

// Merge combines multiple tag maps. Later maps override earlier ones.
func Merge(baseTags map[string]string, extra ...map[string]string) map[string]string {
	result := make(map[string]string, len(baseTags))
	for k, v := range baseTags {
		result[k] = v
	}
	for _, m := range extra {
		for k, v := range m {
			result[k] = v
		}
	}
	return result
}

// WithResourceName returns a copy of tags with the Name tag set.
func WithResourceName(tags map[string]string, name string) map[string]string {
	return Merge(tags, map[string]string{"Name": name})
}

// ToGameLiftTags converts a tag map to GameLift SDK tag slice.
func ToGameLiftTags(tags map[string]string) []gltypes.Tag {
	deduped := Effective(tags)
	result := make([]gltypes.Tag, 0, len(deduped))
	for k, v := range deduped {
		result = append(result, gltypes.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		})
	}
	return result
}

// ToIAMTags converts a tag map to IAM SDK tag slice. Keys are deduplicated
// case-insensitively first because IAM rejects tag sets whose keys differ
// only by case (CreateRole fails with "Duplicate tag keys found") (#775).
func ToIAMTags(tags map[string]string) []iamtypes.Tag {
	deduped := Effective(tags)
	result := make([]iamtypes.Tag, 0, len(deduped))
	for k, v := range deduped {
		result = append(result, iamtypes.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		})
	}
	return result
}

// ToCFNTags converts a tag map to CloudFormation SDK tag slice.
func ToCFNTags(tags map[string]string) []cftypes.Tag {
	deduped := Effective(tags)
	result := make([]cftypes.Tag, 0, len(deduped))
	for k, v := range deduped {
		result = append(result, cftypes.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		})
	}
	return result
}

// ToS3Tags converts a tag map to S3 SDK tag slice.
func ToS3Tags(tags map[string]string) []s3types.Tag {
	deduped := Effective(tags)
	result := make([]s3types.Tag, 0, len(deduped))
	for k, v := range deduped {
		result = append(result, s3types.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		})
	}
	return result
}

// ToTemplateTags generates a JSON array string for embedding in CF templates.
// Output is deterministically sorted by key.
func ToTemplateTags(tags map[string]string) string {
	type cfTag struct {
		Key   string `json:"Key"`
		Value string `json:"Value"`
	}

	// Sort keys for deterministic output
	keys := make([]string, 0, len(tags))
	for k := range tags {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	cfTags := make([]cfTag, 0, len(tags))
	for _, k := range keys {
		cfTags = append(cfTags, cfTag{Key: k, Value: tags[k]})
	}

	data, _ := json.MarshalIndent(cfTags, "        ", "  ")
	return string(data)
}
