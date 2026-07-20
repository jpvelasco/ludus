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

type cgdDeleteResult struct {
	out *gamelift.DeleteContainerGroupDefinitionOutput
	err error
}

type fakeCGDClient struct {
	createResults   []cgdCreateResult
	describeResults []cgdDescribeResult
	deleteResults   []cgdDeleteResult
	createCalls     int
	describeCalls   int
	deleteCalls     int
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

func (f *fakeCGDClient) DeleteContainerGroupDefinition(context.Context, *gamelift.DeleteContainerGroupDefinitionInput, ...func(*gamelift.Options)) (*gamelift.DeleteContainerGroupDefinitionOutput, error) {
	call := f.deleteCalls
	f.deleteCalls++
	if call >= len(f.deleteResults) {
		return &gamelift.DeleteContainerGroupDefinitionOutput{}, nil
	}
	result := f.deleteResults[call]
	return result.out, result.err
}

type cgdAPIError struct {
	code string
}

func (e *cgdAPIError) Error() string                 { return e.code }
func (e *cgdAPIError) ErrorCode() string             { return e.code }
func (e *cgdAPIError) ErrorMessage() string          { return e.code }
func (e *cgdAPIError) ErrorFault() smithy.ErrorFault { return smithy.FaultUnknown }

func TestNewDeployerInitializesDependencies(t *testing.T) {
	opts := DeployOptions{
		Region:             "us-west-2",
		ContainerGroupName: "ludus-test",
	}
	deployer := NewDeployer(opts, aws.Config{Region: opts.Region})

	checks := map[string]bool{
		"options are preserved":             deployer.opts.Region == opts.Region && deployer.opts.ContainerGroupName == opts.ContainerGroupName,
		"GameLift client is initialized":    deployer.glClient != nil,
		"container client uses GameLift":    deployer.cgdClient == deployer.glClient,
		"default retry policy is installed": deployer.cgdCreateRetryConfig == retry.Default(),
		"IAM client is initialized":         deployer.iamClient != nil,
	}
	for name, ok := range checks {
		t.Run(name, func(t *testing.T) {
			if !ok {
				t.Errorf("NewDeployer() check %q failed", name)
			}
		})
	}
}

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
	assertCGDCallCounts(t, client, 2, 2, 0)
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
	assertCGDCallCounts(t, client, 3, 3, 0)
}

func TestCreateContainerGroupDefinitionStopsOnNonConflict(t *testing.T) {
	client := &fakeCGDClient{
		createResults: []cgdCreateResult{{err: &cgdAPIError{code: "AccessDeniedException"}}},
	}

	_, err := newTestCGDDeployer(client).CreateContainerGroupDefinition(context.Background())
	assertCGDErrorContains(t, err, "creating container group definition: AccessDeniedException")
	assertCGDCallCounts(t, client, 1, 0, 0)
}

func TestCreateContainerGroupDefinitionReusesMatchingDefinition(t *testing.T) {
	client := &fakeCGDClient{
		createResults: []cgdCreateResult{{err: &cgdAPIError{code: "ConflictException"}}},
		describeResults: []cgdDescribeResult{
			{out: matchingCGDOutput("arn:existing")},
			{out: matchingCGDOutput("arn:existing")},
		},
	}

	arn, err := newTestCGDDeployer(client).CreateContainerGroupDefinition(context.Background())
	assertCGDSuccess(t, arn, "arn:existing", err)
	assertCGDCallCounts(t, client, 1, 2, 0)
}

func TestCreateContainerGroupDefinitionReplacesMismatch(t *testing.T) {
	client := &fakeCGDClient{
		createResults: []cgdCreateResult{
			{err: &cgdAPIError{code: "ConflictException"}},
			{out: createCGDOutput("arn:replacement")},
		},
		describeResults: []cgdDescribeResult{
			{out: readyCGDOutput("arn:stale")},
			{out: readyCGDOutput("arn:replacement")},
		},
	}

	arn, err := newTestCGDDeployer(client).CreateContainerGroupDefinition(context.Background())
	assertCGDSuccess(t, arn, "arn:replacement", err)
	assertCGDCallCounts(t, client, 2, 2, 1)
}

func TestCreateContainerGroupDefinitionStopsOnDeleteFailure(t *testing.T) {
	client := &fakeCGDClient{
		createResults:   []cgdCreateResult{{err: &cgdAPIError{code: "ConflictException"}}},
		describeResults: []cgdDescribeResult{{out: readyCGDOutput("arn:stale")}},
		deleteResults: []cgdDeleteResult{
			{err: &cgdAPIError{code: "AccessDeniedException"}},
		},
	}

	_, err := newTestCGDDeployer(client).CreateContainerGroupDefinition(context.Background())
	assertCGDErrorContains(t, err, "could not be removed: AccessDeniedException")
	assertCGDCallCounts(t, client, 1, 1, 1)
}

func TestCreateContainerGroupDefinitionRetriesNilDescription(t *testing.T) {
	client := &fakeCGDClient{
		createResults: []cgdCreateResult{
			{err: &cgdAPIError{code: "ConflictException"}},
			{out: createCGDOutput("arn:created")},
		},
		describeResults: []cgdDescribeResult{
			{out: &gamelift.DescribeContainerGroupDefinitionOutput{}},
			{out: readyCGDOutput("arn:created")},
		},
	}

	arn, err := newTestCGDDeployer(client).CreateContainerGroupDefinition(context.Background())
	assertCGDSuccess(t, arn, "arn:created", err)
	assertCGDCallCounts(t, client, 2, 2, 0)
}

func TestCreateContainerGroupDefinitionReturnsWaitError(t *testing.T) {
	client := &fakeCGDClient{
		createResults:   []cgdCreateResult{{out: createCGDOutput("arn:created")}},
		describeResults: []cgdDescribeResult{{err: &cgdAPIError{code: "InternalServiceException"}}},
	}

	arn, err := newTestCGDDeployer(client).CreateContainerGroupDefinition(context.Background())
	if arn != "arn:created" {
		t.Errorf("CreateContainerGroupDefinition() ARN = %q, want arn:created", arn)
	}
	assertCGDErrorContains(t, err, "polling container group definition status")
	assertCGDCallCounts(t, client, 1, 1, 0)
}

func TestDeleteContainerGroupDefinitionUsesCGDClient(t *testing.T) {
	client := &fakeCGDClient{}
	deployer := newTestCGDDeployer(client)

	if err := deployer.deleteContainerGroupDefinition(context.Background()); err != nil {
		t.Fatalf("deleteContainerGroupDefinition() error = %v", err)
	}
	assertCGDCallCounts(t, client, 0, 0, 1)
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

func matchingCGDOutput(arn string) *gamelift.DescribeContainerGroupDefinitionOutput {
	return &gamelift.DescribeContainerGroupDefinitionOutput{
		ContainerGroupDefinition: &gltypes.ContainerGroupDefinition{
			ContainerGroupDefinitionArn: aws.String(arn),
			GameServerContainerDefinition: &gltypes.GameServerContainerDefinition{
				ImageUri:         aws.String(""),
				ServerSdkVersion: aws.String("5.4.0"),
				PortConfiguration: &gltypes.ContainerPortConfiguration{
					ContainerPortRanges: []gltypes.ContainerPortRange{
						{
							FromPort: aws.Int32(0),
							ToPort:   aws.Int32(0),
							Protocol: gltypes.IpProtocolUdp,
						},
					},
				},
			},
			TotalMemoryLimitMebibytes: aws.Int32(1024),
			TotalVcpuLimit:            aws.Float64(1.0),
			Status:                    gltypes.ContainerGroupDefinitionStatusReady,
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

func assertCGDSuccess(t *testing.T, got, want string, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("CreateContainerGroupDefinition() error = %v", err)
	}
	if got != want {
		t.Fatalf("CreateContainerGroupDefinition() = %q, want %q", got, want)
	}
}

func assertCGDErrorContains(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("error = nil, want error containing %q", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %q, want error containing %q", err, want)
	}
}

func assertCGDCallCounts(t *testing.T, client *fakeCGDClient, createWant, describeWant, deleteWant int) {
	t.Helper()
	if client.createCalls != createWant {
		t.Errorf("CreateContainerGroupDefinition calls = %d, want %d", client.createCalls, createWant)
	}
	if client.describeCalls != describeWant {
		t.Errorf("DescribeContainerGroupDefinition calls = %d, want %d", client.describeCalls, describeWant)
	}
	if client.deleteCalls != deleteWant {
		t.Errorf("DeleteContainerGroupDefinition calls = %d, want %d", client.deleteCalls, deleteWant)
	}
}
