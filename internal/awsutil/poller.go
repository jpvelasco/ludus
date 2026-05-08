package awsutil

import (
	"context"
	"time"
)

// Poll calls fn until it returns done, an error, the context is canceled, or the timeout expires.
func Poll(ctx context.Context, interval, timeout time.Duration, fn func() (bool, error)) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		done, err := fn()
		if done || err != nil {
			return err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}
	return nil
}
