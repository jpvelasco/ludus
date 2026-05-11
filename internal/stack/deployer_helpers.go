package stack

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cftypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
)

const (
	pollInterval = 15 * time.Second
	maxPollWait  = 30 * time.Minute
)

func (d *StackDeployer) describeStack(ctx context.Context) (*cftypes.Stack, error) {
	out, err := d.cfnClient.DescribeStacks(ctx, &cloudformation.DescribeStacksInput{
		StackName: aws.String(d.opts.StackName),
	})
	if err != nil {
		return nil, err
	}
	if len(out.Stacks) == 0 {
		return nil, fmt.Errorf("stack %q not found", d.opts.StackName)
	}
	return &out.Stacks[0], nil
}

func (d *StackDeployer) readFleetIDFromOutputs(ctx context.Context) string {
	status, err := d.Status(ctx)
	if err != nil {
		return ""
	}
	return status.FleetID
}

func (d *StackDeployer) readStackStatus(ctx context.Context) string {
	stack, err := d.describeStack(ctx)
	if err != nil {
		return "UNKNOWN"
	}
	return string(stack.StackStatus)
}

func isStackNotFound(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "does not exist") ||
		strings.Contains(msg, "NotFoundException") ||
		strings.Contains(msg, "NotFound")
}

func stackDeletionPollResult(status, reason string) (bool, error) {
	if status == "DELETE_COMPLETE" {
		fmt.Println("Stack deleted.")
		return true, nil
	}
	if status == "DELETE_FAILED" {
		return false, fmt.Errorf("stack deletion failed: %s", reason)
	}
	return false, nil
}
