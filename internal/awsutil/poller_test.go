package awsutil

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestPollWithOptionsDone(t *testing.T) {
	calls := 0
	err := PollWithOptions(context.Background(), PollOptions{
		Interval: time.Millisecond,
		Timeout:  time.Second,
	}, func() (bool, error) {
		calls++
		return calls == 2, nil
	})

	if err != nil {
		t.Fatalf("PollWithOptions returned error: %v", err)
	}
	if calls != 2 {
		t.Errorf("calls = %d, want 2", calls)
	}
}

func TestPollWithOptionsTimeout(t *testing.T) {
	err := PollWithOptions(context.Background(), PollOptions{
		Interval: time.Millisecond,
		Timeout:  time.Millisecond,
	}, func() (bool, error) {
		return false, nil
	})

	if !errors.Is(err, ErrPollTimeout) {
		t.Fatalf("error = %v, want ErrPollTimeout", err)
	}
}

func TestPollWithOptionsContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := PollWithOptions(ctx, PollOptions{
		Interval: time.Second,
		Timeout:  time.Second,
	}, func() (bool, error) {
		return false, nil
	})

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}
