package tags

import (
	"encoding/json"
	"sort"

	"github.com/aws/aws-sdk-go-v2/aws"
	cftypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	gltypes "github.com/aws/aws-sdk-go-v2/service/gamelift/types"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/devrecon/ludus/internal/config"
)

// Build constructs the full tag set from config. It starts with cfg.AWS.Tags,
// adds Project from cfg.Game.ProjectName if not already set, and ensures
// ManagedBy is present.
func Build(cfg *config.Config) map[string]string {
	tags := make(map[string]string, len(cfg.AWS.Tags)+2)

	// Start with user-configured tags
	for k, v := range cfg.AWS.Tags {
		tags[k] = v
	}

	// Auto-derive Project from game project name if not set
	if _, ok := tags["Project"]; !ok {
		if cfg.Game.ProjectName != "" {
			tags["Project"] = cfg.Game.ProjectName
		}
	}

	// Ensure ManagedBy is always present
	if _, ok := tags["ManagedBy"]; !ok {
		tags["ManagedBy"] = "ludus"
	}

	return tags
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
	result := make([]gltypes.Tag, 0, len(tags))
	for k, v := range tags {
		result = append(result, gltypes.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		})
	}
	return result
}

// ToIAMTags converts a tag map to IAM SDK tag slice.
func ToIAMTags(tags map[string]string) []iamtypes.Tag {
	result := make([]iamtypes.Tag, 0, len(tags))
	for k, v := range tags {
		result = append(result, iamtypes.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		})
	}
	return result
}

// ToCFNTags converts a tag map to CloudFormation SDK tag slice.
func ToCFNTags(tags map[string]string) []cftypes.Tag {
	result := make([]cftypes.Tag, 0, len(tags))
	for k, v := range tags {
		result = append(result, cftypes.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		})
	}
	return result
}

// ToS3Tags converts a tag map to S3 SDK tag slice.
func ToS3Tags(tags map[string]string) []s3types.Tag {
	result := make([]s3types.Tag, 0, len(tags))
	for k, v := range tags {
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
