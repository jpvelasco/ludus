package inventory

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	ecrtypes "github.com/aws/aws-sdk-go-v2/service/ecr/types"
	"github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi"
	tagtypes "github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	"github.com/jpvelasco/ludus/internal/config"
	"github.com/jpvelasco/ludus/internal/tags"
)

func TestScanConfiguredIdentityAndLegacySpellings(t *testing.T) {
	mapping := tagtypes.ResourceTagMapping{ResourceARN: aws.String("arn:aws:s3:::example-build-storage")}
	client := &mockTaggingClient{outputs: []*resourcegroupstaggingapi.GetResourcesOutput{
		{ResourceTagMappingList: []tagtypes.ResourceTagMapping{mapping}, PaginationToken: aws.String("page-2")},
		{ResourceTagMappingList: []tagtypes.ResourceTagMapping{mapping}},
		{ResourceTagMappingList: []tagtypes.ResourceTagMapping{mapping}},
	}}
	s := newTestScanner(client, notFoundECRClient(), emptyS3Client(), nil, "")
	s.tags = map[string]string{"managedBY": "example-team", "PROJECT": "example-game"}
	inv, err := s.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(inv.Resources) != 1 || inv.Resources[0].Scope != "project-tags" {
		t.Fatalf("expected one deduplicated project resource: %+v", inv)
	}
	assertIdentityQueries(t, client)
}

func assertIdentityQueries(t *testing.T, client *mockTaggingClient) {
	t.Helper()
	keys := map[string]bool{}
	for _, in := range client.inputs {
		if len(in.TagFilters) != 2 {
			t.Fatalf("unscoped query: %+v", in)
		}
		if !reflect.DeepEqual(in.TagFilters[0].Values, []string{"example-team"}) ||
			!reflect.DeepEqual(in.TagFilters[1].Values, []string{"example-game"}) {
			t.Fatalf("wrong identity in query: %+v", in.TagFilters)
		}
		keys[aws.ToString(in.TagFilters[0].Key)+"/"+aws.ToString(in.TagFilters[1].Key)] = true
	}
	if len(keys) != 9 || len(client.inputs) != 10 {
		t.Errorf("queries = %d unique, %d total; want 9 plus pagination", len(keys), len(client.inputs))
	}
	if aws.ToString(client.inputs[1].PaginationToken) != "page-2" {
		t.Error("pagination token not forwarded")
	}
}

func TestScanTagAccessDeniedContinues(t *testing.T) {
	client := &mockTaggingClient{err: &smithy.GenericAPIError{Code: "AccessDeniedException"}}
	ecrClient := &mockECRClient{describeOutput: &ecr.DescribeRepositoriesOutput{
		Repositories: []ecrtypes.Repository{{RepositoryArn: aws.String("arn:aws:ecr:us-east-1:123456789012:repository/example")}},
	}}
	s3Client := &mockS3Client{listOutputs: []*s3.ListBucketsOutput{{}}}
	s := newTestScanner(client, ecrClient, s3Client, []string{"example"}, "ludus-builds-")
	inv, err := s.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(inv.Resources) != 1 || inv.Resources[0].Scope != "configured-name" {
		t.Fatalf("expected explicitly configured repository: %+v", inv)
	}
	if len(s3Client.listInputs) != 1 {
		t.Fatal("S3 scan was skipped")
	}
	if len(inv.Warnings) != 1 || !strings.Contains(inv.Warnings[0], "tag:GetResources") {
		t.Fatalf("missing permission warning: %v", inv.Warnings)
	}
}

func TestBucketIdentityUsesConfiguredProject(t *testing.T) {
	s := newTestScanner(emptyTaggingClient(), notFoundECRClient(), emptyS3Client(), nil, "")
	s.tags = map[string]string{"managedby": "example-team", "project": "example-game"}
	for _, tt := range []struct {
		name, project string
		want          bool
	}{
		{"matching", "example-game", true},
		{"foreign project", "other", false},
		{"empty project", "", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			detail := s.bucketIdentityDetail(tags.ToS3Tags(map[string]string{"ManagedBy": "example-team", "Project": tt.project}))
			if (detail != "") != tt.want {
				t.Errorf("bucket detail = %q, expected match %v", detail, tt.want)
			}
			if tt.want && !strings.Contains(detail, "managedby=example-team") {
				t.Errorf("incorrect tag annotation: %s", detail)
			}
		})
	}
}

func TestScanRejectsIncompleteIdentity(t *testing.T) {
	s := newTestScanner(emptyTaggingClient(), notFoundECRClient(), emptyS3Client(), nil, "")
	s.tags = map[string]string{"ManagedBy": "example-team"}
	if _, err := s.Scan(context.Background()); err == nil {
		t.Fatal("missing project allowed an account-wide scan")
	}
}

func TestOptionsForConfig(t *testing.T) {
	for _, tt := range []struct{ name, server, engine, want string }{
		{"defaults", "", "", "ludus-server,ludus-engine"},
		{"custom", "server", "engine", "server,engine"},
		{"deduplicates", "shared", "shared", "shared"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{}
			cfg.AWS.ECRRepository = tt.server
			cfg.Engine.DockerImageName = tt.engine
			cfg.AWS.Tags = map[string]string{"ManagedBy": "example-team", "project": "example-game"}
			opts := OptionsForConfig(cfg, "us-west-2")
			if strings.Join(opts.ECRRepoNames, ",") != tt.want || opts.Region != "us-west-2" {
				t.Fatalf("unexpected scope: %+v", opts)
			}
			if !reflect.DeepEqual(opts.Tags, cfg.AWS.Tags) {
				t.Errorf("configured tags lost: %v", opts.Tags)
			}
		})
	}
}
