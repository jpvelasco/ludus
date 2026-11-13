package inventory

import (
	"context"
	"fmt"
	"slices"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi"
	tagtypes "github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi/types"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/tags"
)

// Options scopes discovery to the effective configured identity and names.
type Options struct {
	Region         string
	ECRRepoNames   []string
	S3BucketPrefix string
	Tags           map[string]string
}

// OptionsForConfig resolves the shared CLI/MCP discovery scope.
func OptionsForConfig(cfg *config.Config, region string) Options {
	server, engine := cfg.AWS.ECRRepository, cfg.Engine.DockerImageName
	if server == "" {
		server = "ludus-server"
	}
	if engine == "" {
		engine = "ludus-engine"
	}
	return Options{
		Region: region, ECRRepoNames: slices.Compact([]string{server, engine}),
		S3BucketPrefix: "ludus-builds-", Tags: tags.Build(cfg),
	}
}

func (s *Scanner) identityFilters() ([][]tagtypes.TagFilter, error) {
	managerKey, manager := tags.Lookup(s.tags, "ManagedBy")
	projectKey, project := tags.Lookup(s.tags, "Project")
	if manager == "" || project == "" {
		return nil, fmt.Errorf("resource discovery requires nonempty ManagedBy and Project tags")
	}
	var filters [][]tagtypes.TagFilter
	for _, mk := range tagSpellings(managerKey, "ManagedBy", "managedby") {
		for _, pk := range tagSpellings(projectKey, "Project", "project") {
			filters = append(filters, []tagtypes.TagFilter{
				{Key: aws.String(mk), Values: []string{manager}},
				{Key: aws.String(pk), Values: []string{project}},
			})
		}
	}
	return filters, nil
}

func tagSpellings(keys ...string) []string {
	var unique []string
	for _, key := range keys {
		if !slices.Contains(unique, key) {
			unique = append(unique, key)
		}
	}
	return unique
}

func (s *Scanner) scanTaggedQuery(ctx context.Context, inv *Inventory, seen map[string]bool, filters []tagtypes.TagFilter) error {
	var token *string
	for {
		out, err := s.tagging.GetResources(ctx, &resourcegroupstaggingapi.GetResourcesInput{
			TagFilters: filters, PaginationToken: token,
		})
		if err != nil {
			return fmt.Errorf("get tagged resources: %w", err)
		}
		for _, mapping := range out.ResourceTagMappingList {
			arn := aws.ToString(mapping.ResourceARN)
			if arn == "" || seen[arn] {
				continue
			}
			seen[arn] = true
			service, resourceType, resourceName := parseARN(arn)
			inv.Resources = append(inv.Resources, Resource{
				Type: humanizeResourceType(service, resourceType), Name: resourceName,
				ARN: arn, Scope: "project-tags",
			})
		}
		token = out.PaginationToken
		if aws.ToString(token) == "" {
			return nil
		}
	}
}

func (s *Scanner) bucketIdentityDetail(observed []s3types.Tag) string {
	mk, manager := tags.Lookup(s.tags, "ManagedBy")
	pk, project := tags.Lookup(s.tags, "Project")
	if !hasTag(observed, mk, manager) || !hasTag(observed, pk, project) {
		return ""
	}
	return fmt.Sprintf("tagged %s=%s, %s=%s", mk, manager, pk, project)
}
