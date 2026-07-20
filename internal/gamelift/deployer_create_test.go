package gamelift

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/gamelift"
	gltypes "github.com/aws/aws-sdk-go-v2/service/gamelift/types"
	"github.com/aws/smithy-go"
	"github.com/jpvelasco/ludus/internal/retry"
)

type cgdCreateResult struct {
	out *gamelift.CreateContainerGroupDefinitionOutput
	err error
}

type cgdDescribeResult struct {
	out *gamelift.DescribeContainerGroupDefinitionOutput
	err error
}

type fakeCGDClient struct {
	createResults   []cgdCreateResult
	describeResults []cgdDescribeResult
	createCalls     int
	describeCalls   int
}

func (f *fakeCGDClient) CreateContainerGroupDefinition(context.Context, *gamelift.CreateContainerGroupDefinitionInput, ...func(*gamelift.Options)) (*gamelift.CreateContainerGroupDefinitionOutput, error) {
	result := f.createResults[f.createCalls]
	f.createCalls++
	return result.out, result.err
}

func (f *fakeCGDClient) DescribeContainerGroupDefinition(context.Context, *gamelift.DescribeContainerGroupDefinitionInput, ...func(*gamelift.Options)) (*gamelift.DescribeContainerGroupDefinitionOutput, error) {
	result := f.describeResults[f.describeCalls]
	f.describeCalls++
	return result.out, result.err
}

func (*fakeCGDClient) DeleteContainerGroupDefinition(context.Context, *gamelift.DeleteContainerGroupDefinitionInput, ...func(*gamelift.Options)) (*gamelift.DeleteContainerGroupDefinitionOutput, error) {
	return &gamelift.DeleteContainerGroupDefinitionOutput{}, nil
}

type cgdAPIError struct {
	code string
}

func (e *cgdAPIError) Error() string                 { return e.code }
func (e *cgdAPIError) ErrorCode() string             { return e.code }
func (e *cgdAPIError) ErrorMessage() string          { return e.code }
func (e *cgdAPIError) ErrorFault() smithy.ErrorFault { return smithy.FaultUnknown }

func TestCreateContainerGroupDefinitionRetriesConflictDuringDeletion(t *testing.T) {
	client := &fakeCGDClient{
		createResults: []cgdCreateResult{
			{err: &cgdAPIError{code: "ConflictException"}},
			{out: createCGDOutput("arn:created")},
		},
		describeResults: []cgdDescribeResult{
			{err: &cgdAPIError{code: "NotFoundException"}},
			{out: readyCGDOutput("arn:created")},
		},
	}

	arn, err := newTestCGDDeployer(client).CreateContainerGroupDefinition(context.Background())
	if err != nil {
		t.Fatalf("CreateContainerGroupDefinition() error = %v", err)
	}
	if arn != "arn:created" {
		t.Fatalf("CreateContainerGroupDefinition() = %q, want arn:created", arn)
	}
	assertCGDCallCounts(t, client, 2, 2)
}

func TestCreateContainerGroupDefinitionExhaustsDeletionConflictRetries(t *testing.T) {
	client := &fakeCGDClient{
		createResults:   repeatCreateErrors(3, "ConflictException"),
		describeResults: repeatDescribeErrors(3, "NotFoundException"),
	}

	_, err := newTestCGDDeployer(client).CreateContainerGroupDefinition(context.Background())
	if err == nil {
		t.Fatal("CreateContainerGroupDefinition() error = nil, want retry exhaustion")
	}
	if !strings.Contains(err.Error(), "creating container group definition after retries") {
		t.Fatalf("CreateContainerGroupDefinition() error = %q, want retry exhaustion context", err)
	}
	assertCGDCallCounts(t, client, 3, 3)
}

func newTestCGDDeployer(client containerGroupDefinitionClient) *Deployer {
	return &Deployer{
		opts: DeployOptions{
			ContainerGroupName: "ludus-test",
		},
		cgdClient: client,
		cgdCreateRetryConfig: retry.Config{
			MaxAttempts: 3,
			BaseDelay:   time.Nanosecond,
			MaxDelay:    time.Nanosecond,
		},
	}
}

func createCGDOutput(arn string) *gamelift.CreateContainerGroupDefinitionOutput {
	return &gamelift.CreateContainerGroupDefinitionOutput{
		ContainerGroupDefinition: &gltypes.ContainerGroupDefinition{
			ContainerGroupDefinitionArn: aws.String(arn),
		},
	}
}

func readyCGDOutput(arn string) *gamelift.DescribeContainerGroupDefinitionOutput {
	return &gamelift.DescribeContainerGroupDefinitionOutput{
		ContainerGroupDefinition: &gltypes.ContainerGroupDefinition{
			ContainerGroupDefinitionArn: aws.String(arn),
			Status:                      gltypes.ContainerGroupDefinitionStatusReady,
		},
	}
}

func repeatCreateErrors(count int, code string) []cgdCreateResult {
	results := make([]cgdCreateResult, count)
	for i := range results {
		results[i].err = &cgdAPIError{code: code}
	}
	return results
}

func repeatDescribeErrors(count int, code string) []cgdDescribeResult {
	results := make([]cgdDescribeResult, count)
	for i := range results {
		results[i].err = &cgdAPIError{code: code}
	}
	return results
}

func assertCGDCallCounts(t *testing.T, client *fakeCGDClient, createWant, describeWant int) {
	t.Helper()
	if client.createCalls != createWant {
		t.Errorf("CreateContainerGroupDefinition calls = %d, want %d", client.createCalls, createWant)
	}
	if client.describeCalls != describeWant {
		t.Errorf("DescribeContainerGroupDefinition calls = %d, want %d", client.describeCalls, describeWant)
	}
}
