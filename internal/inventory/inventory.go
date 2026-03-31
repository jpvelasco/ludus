package inventory

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	"github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi"
	tagtypes "github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/devrecon/ludus/internal/awsutil"
)

// Resource represents a single AWS resource managed by ludus.
type Resource struct {
	Type   string `json:"type"`
	Name   string `json:"name"`
	ARN    string `json:"arn,omitempty"`
	Status string `json:"status,omitempty"`
	Detail string `json:"detail,omitempty"`
}

// Inventory holds the scan results for a single region.
type Inventory struct {
	Region    string     `json:"region"`
	Resources []Resource `json:"resources"`
}

// taggingAPI is the subset of Resource Groups Tagging API operations needed.
type taggingAPI interface {
	GetResources(ctx context.Context, params *resourcegroupstaggingapi.GetResourcesInput, optFns ...func(*resourcegroupstaggingapi.Options)) (*resourcegroupstaggingapi.GetResourcesOutput, error)
}

// ecrAPI is the subset of ECR operations needed for inventory.
type ecrAPI interface {
	DescribeRepositories(ctx context.Context, params *ecr.DescribeRepositoriesInput, optFns ...func(*ecr.Options)) (*ecr.DescribeRepositoriesOutput, error)
	ListImages(ctx context.Context, params *ecr.ListImagesInput, optFns ...func(*ecr.Options)) (*ecr.ListImagesOutput, error)
}

// s3API is the subset of S3 operations needed for inventory.
type s3API interface {
	ListBuckets(ctx context.Context, params *s3.ListBucketsInput, optFns ...func(*s3.Options)) (*s3.ListBucketsOutput, error)
	GetBucketTagging(ctx context.Context, params *s3.GetBucketTaggingInput, optFns ...func(*s3.Options)) (*s3.GetBucketTaggingOutput, error)
}

// Scanner discovers ludus-managed AWS resources.
type Scanner struct {
	tagging        taggingAPI
	ecr            ecrAPI
	s3             s3API
	region         string
	ecrRepoNames   []string
	s3BucketPrefix string
}

// NewScanner creates a Scanner from an AWS config.
func NewScanner(awsCfg aws.Config, region string, ecrRepoNames []string, s3BucketPrefix string) *Scanner {
	return &Scanner{
		tagging:        resourcegroupstaggingapi.NewFromConfig(awsCfg),
		ecr:            ecr.NewFromConfig(awsCfg),
		s3:             s3.NewFromConfig(awsCfg),
		region:         region,
		ecrRepoNames:   ecrRepoNames,
		s3BucketPrefix: s3BucketPrefix,
	}
}

// Scan discovers all ludus-managed resources in the account.
func (s *Scanner) Scan(ctx context.Context) (*Inventory, error) {
	inv := &Inventory{
		Region:    s.region,
		Resources: []Resource{},
	}

	seen := make(map[string]bool)

	// 1. Resource Groups Tagging API
	if err := s.scanTaggedResources(ctx, inv, seen); err != nil {
		return nil, err
	}

	// 2. ECR repos by known name
	s.scanECRRepos(ctx, inv, seen)

	// 3. S3 buckets by prefix
	s.scanS3Buckets(ctx, inv, seen)

	return inv, nil
}

func (s *Scanner) scanTaggedResources(ctx context.Context, inv *Inventory, seen map[string]bool) error {
	token := ""

	for {
		input := &resourcegroupstaggingapi.GetResourcesInput{
			TagFilters: []tagtypes.TagFilter{
				{
					Key:    aws.String("ManagedBy"),
					Values: []string{"ludus"},
				},
			},
		}
		if token != "" {
			input.PaginationToken = aws.String(token)
		}

		output, err := s.tagging.GetResources(ctx, input)
		if err != nil {
			return fmt.Errorf("get tagged resources: %w", err)
		}

		for _, mapping := range output.ResourceTagMappingList {
			arn := aws.ToString(mapping.ResourceARN)
			if arn == "" || seen[arn] {
				continue
			}
			seen[arn] = true

			service, resourceType, resourceName := parseARN(arn)
			inv.Resources = append(inv.Resources, Resource{
				Type: humanizeResourceType(service, resourceType),
				Name: resourceName,
				ARN:  arn,
			})
		}

		token = aws.ToString(output.PaginationToken)
		if token == "" {
			break
		}
	}

	return nil
}

func (s *Scanner) scanECRRepos(ctx context.Context, inv *Inventory, seen map[string]bool) {
	for _, repoName := range s.ecrRepoNames {
		if seen[repoName] {
			continue
		}

		descOutput, err := s.ecr.DescribeRepositories(ctx, &ecr.DescribeRepositoriesInput{
			RepositoryNames: []string{repoName},
		})
		if err != nil {
			if awsutil.IsNotFound(err) {
				continue
			}
			fmt.Printf("Warning: failed to describe ECR repository %s: %v\n", repoName, err)
			continue
		}

		for _, repo := range descOutput.Repositories {
			arn := aws.ToString(repo.RepositoryArn)
			if arn != "" && seen[arn] {
				continue
			}

			listOutput, err := s.ecr.ListImages(ctx, &ecr.ListImagesInput{
				RepositoryName: aws.String(repoName),
			})
			detail := ""
			if err != nil {
				fmt.Printf("Warning: failed to list images in ECR repository %s: %v\n", repoName, err)
			} else {
				detail = fmt.Sprintf("%d images", len(listOutput.ImageIds))
			}

			if arn != "" {
				seen[arn] = true
			}
			seen[repoName] = true

			inv.Resources = append(inv.Resources, Resource{
				Type:   "ECR Repository",
				Name:   repoName,
				ARN:    arn,
				Detail: detail,
			})
		}
	}
}

func (s *Scanner) scanS3Buckets(ctx context.Context, inv *Inventory, seen map[string]bool) {
	if s.s3BucketPrefix == "" {
		return
	}

	listOutput, err := s.s3.ListBuckets(ctx, &s3.ListBucketsInput{})
	if err != nil {
		fmt.Printf("Warning: failed to list S3 buckets: %v\n", err)
		return
	}

	for _, bucket := range listOutput.Buckets {
		name := aws.ToString(bucket.Name)
		if !strings.HasPrefix(name, s.s3BucketPrefix) {
			continue
		}

		// Build a pseudo-ARN for dedup against tagging results
		bucketARN := "arn:aws:s3:::" + name
		if seen[bucketARN] {
			continue
		}

		// Try to verify it has the ManagedBy tag
		detail := ""
		tagOutput, err := s.s3.GetBucketTagging(ctx, &s3.GetBucketTaggingInput{
			Bucket: aws.String(name),
		})
		if err == nil && hasTag(tagOutput.TagSet, "ManagedBy", "ludus") {
			detail = "tagged ManagedBy=ludus"
		}

		seen[bucketARN] = true
		inv.Resources = append(inv.Resources, Resource{
			Type:   "S3 Bucket",
			Name:   name,
			ARN:    bucketARN,
			Detail: detail,
		})
	}
}

// hasTag returns true if the tag set contains a tag with the given key and value.
func hasTag(tags []s3types.Tag, key, value string) bool {
	for _, tag := range tags {
		if aws.ToString(tag.Key) == key && aws.ToString(tag.Value) == value {
			return true
		}
	}
	return false
}

// parseARN extracts service, resource type, and resource name from an ARN.
// Handles formats:
//   - arn:aws:service:region:account:resource-type/resource-name
//   - arn:aws:service:region:account:resource-type:resource-name
//   - arn:aws:s3:::bucket-name
func parseARN(arn string) (service, resourceType, resourceName string) {
	parts := strings.SplitN(arn, ":", 7)
	if len(parts) < 6 {
		return "", "", arn
	}

	service = parts[2]
	resource := parts[5]

	// If there's a 7th part, the separator was ":" between type and name
	if len(parts) == 7 {
		resourceType = resource
		resourceName = parts[6]
		// CloudFormation stacks have format stack/name/guid — strip guid
		if idx := strings.Index(resourceName, "/"); idx >= 0 {
			resourceName = resourceName[:idx]
		}
		return service, resourceType, resourceName
	}

	// Check for "/" separator between type and name
	if resourceType, resourceName, ok := strings.Cut(resource, "/"); ok {
		// CloudFormation stacks: stack/name/guid — strip guid
		if slashIdx := strings.Index(resourceName, "/"); slashIdx >= 0 {
			resourceName = resourceName[:slashIdx]
		}
		return service, resourceType, resourceName
	}

	// No separator — resource is just a name (e.g., S3 bucket)
	return service, "", resource
}

// humanizeResourceType produces a human-friendly label from ARN components.
func humanizeResourceType(service, resourceType string) string {
	key := service + "+" + resourceType

	known := map[string]string{
		"gamelift+fleet":                    "GameLift Fleet",
		"gamelift+containerfleet":           "GameLift Container Fleet",
		"gamelift+containergroupdefinition": "GameLift Container Group",
		"gamelift+build":                    "GameLift Build",
		"gamelift+alias":                    "GameLift Alias",
		"gamelift+gamesessionqueue":         "GameLift Queue",
		"gamelift+matchmakingconfiguration": "GameLift Matchmaking Config",
		"gamelift+matchmakingruleset":       "GameLift Matchmaking Rule Set",
		"iam+role":                          "IAM Role",
		"iam+policy":                        "IAM Policy",
		"iam+instance-profile":              "IAM Instance Profile",
		"cloudformation+stack":              "CloudFormation Stack",
		"s3+":                               "S3 Bucket",
		"ecr+repository":                    "ECR Repository",
		"ec2+instance":                      "EC2 Instance",
		"ec2+security-group":                "EC2 Security Group",
	}

	if label, ok := known[key]; ok {
		return label
	}

	// Fall back to title-cased service + resourceType
	return titleCase(service) + " " + titleCase(resourceType)
}

// titleCase uppercases the first letter of a string.
func titleCase(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
