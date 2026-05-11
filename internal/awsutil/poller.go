package awsutil

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// ErrPollTimeout indicates that Poll reached its timeout before fn completed.
var ErrPollTimeout = errors.New("poll timed out")

// PollOptions controls polling behavior.
type PollOptions struct {
	Interval time.Duration
	Timeout  time.Duration
}

// Poll calls fn until it returns done, an error, the context is canceled, or the timeout expires.
func Poll(ctx context.Context, interval, timeout time.Duration, fn func() (bool, error)) error {
	return PollWithOptions(ctx, PollOptions{Interval: interval, Timeout: timeout}, fn)
}

// WrapTimeout returns a formatted error if err is ErrPollTimeout, otherwise
// returns err unchanged (including nil). Use after awsutil.Poll to convert
// the generic timeout sentinel into a caller-specific message.
func WrapTimeout(err error, operation string) error {
	if errors.Is(err, ErrPollTimeout) {
		return fmt.Errorf("timed out waiting for %s", operation)
	}
	return err
}

// PollWithOptions calls fn using the provided polling options.
func PollWithOptions(ctx context.Context, opts PollOptions, fn func() (bool, error)) error {
	deadline := time.Now().Add(opts.Timeout)
	for time.Now().Before(deadline) {
		done, err := fn()
		if done || err != nil {
			return err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(opts.Interval):
		}
	}
	return ErrPollTimeout
}
