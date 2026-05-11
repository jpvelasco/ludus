package stack

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cftypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"

	"github.com/devrecon/ludus/internal/awsutil"
)

func (d *StackDeployer) createStack(ctx context.Context, templateBody string, params []cftypes.Parameter, stackTags []cftypes.Tag) (*StackResult, error) {
	fmt.Printf("Creating CloudFormation stack %q...\n", d.opts.StackName)

	out, err := d.cfnClient.CreateStack(ctx, &cloudformation.CreateStackInput{
		StackName:    aws.String(d.opts.StackName),
		TemplateBody: aws.String(templateBody),
		Parameters:   params,
		Tags:         stackTags,
		Capabilities: []cftypes.Capability{cftypes.CapabilityCapabilityNamedIam},
	})
	if err != nil {
		return nil, fmt.Errorf("creating stack: %w", err)
	}

	result := &StackResult{
		StackName: d.opts.StackName,
		StackID:   aws.ToString(out.StackId),
	}

	status, err := d.pollStack(ctx, "CREATE_COMPLETE", "CREATE_FAILED", "ROLLBACK_COMPLETE")
	if err != nil {
		result.Status = status
		return result, err
	}

	result.Status = status
	result.FleetID = d.readFleetIDFromOutputs(ctx)
	return result, nil
}

func (d *StackDeployer) updateStack(ctx context.Context, templateBody string, params []cftypes.Parameter, stackTags []cftypes.Tag) (*StackResult, error) {
	fmt.Printf("Updating CloudFormation stack %q...\n", d.opts.StackName)

	_, err := d.cfnClient.UpdateStack(ctx, &cloudformation.UpdateStackInput{
		StackName:    aws.String(d.opts.StackName),
		TemplateBody: aws.String(templateBody),
		Parameters:   params,
		Tags:         stackTags,
		Capabilities: []cftypes.Capability{cftypes.CapabilityCapabilityNamedIam},
	})
	if err != nil {
		if strings.Contains(err.Error(), "No updates are to be performed") {
			fmt.Println("Stack is already up to date.")
			status := d.readStackStatus(ctx)
			return &StackResult{
				StackName: d.opts.StackName,
				Status:    status,
				FleetID:   d.readFleetIDFromOutputs(ctx),
			}, nil
		}
		return nil, fmt.Errorf("updating stack: %w", err)
	}

	result := &StackResult{
		StackName: d.opts.StackName,
	}

	status, err := d.pollStack(ctx, "UPDATE_COMPLETE", "UPDATE_ROLLBACK_COMPLETE", "UPDATE_FAILED")
	if err != nil {
		result.Status = status
		return result, err
	}

	result.Status = status
	result.FleetID = d.readFleetIDFromOutputs(ctx)
	return result, nil
}

func (d *StackDeployer) deleteStack(ctx context.Context) error {
	_, err := d.cfnClient.DeleteStack(ctx, &cloudformation.DeleteStackInput{
		StackName: aws.String(d.opts.StackName),
	})
	if err == nil {
		return nil
	}
	if isStackNotFound(err) {
		fmt.Println("Stack not found, skipping.")
		return nil
	}
	return fmt.Errorf("deleting stack: %w", err)
}

func (d *StackDeployer) waitForStackDeletion(ctx context.Context) error {
	err := awsutil.Poll(ctx, pollInterval, maxPollWait, func() (bool, error) {
		return d.pollStackDeletion(ctx)
	})
	if err != nil && !errors.Is(err, awsutil.ErrPollTimeout) {
		return err
	}
	if errors.Is(err, awsutil.ErrPollTimeout) {
		return fmt.Errorf("timed out waiting for stack deletion")
	}
	return nil
}

func (d *StackDeployer) pollStackDeletion(ctx context.Context) (bool, error) {
	stack, err := d.describeStack(ctx)
	if err != nil {
		if isStackNotFound(err) {
			fmt.Println("Stack deleted.")
			return true, nil
		}
		return false, fmt.Errorf("polling stack deletion: %w", err)
	}

	status := string(stack.StackStatus)
	fmt.Printf("  Stack status: %s\n", status)
	return stackDeletionPollResult(status, aws.ToString(stack.StackStatusReason))
}

func (d *StackDeployer) pollStack(ctx context.Context, successStatus, failStatus, rollbackStatus string) (string, error) {
	deadline := time.Now().Add(maxPollWait)
	for time.Now().Before(deadline) {
		status, err := d.pollStackStatus(ctx, successStatus, failStatus, rollbackStatus)
		if err != nil {
			return status, err
		}
		if status == successStatus {
			return status, nil
		}

		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(pollInterval):
		}
	}

	return "", fmt.Errorf("timed out waiting for stack to reach %s", successStatus)
}

func (d *StackDeployer) pollStackStatus(ctx context.Context, successStatus, failStatus, rollbackStatus string) (string, error) {
	stack, err := d.describeStack(ctx)
	if err != nil {
		return "", fmt.Errorf("polling stack status: %w", err)
	}

	status := string(stack.StackStatus)
	fmt.Printf("  Stack status: %s\n", status)
	if status == successStatus {
		return status, nil
	}
	if status == failStatus || status == rollbackStatus {
		reason := aws.ToString(stack.StackStatusReason)
		return status, fmt.Errorf("stack operation failed (%s): %s", status, reason)
	}
	return status, nil
}
