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

func TestWrapTimeout(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		operation string
		wantMsg   string
		wantNil   bool
	}{
		{
			name:    "nil passes through",
			err:     nil,
			wantNil: true,
		},
		{
			name:      "ErrPollTimeout becomes formatted message",
			err:       ErrPollTimeout,
			operation: "fleet to become ACTIVE",
			wantMsg:   "timed out waiting for fleet to become ACTIVE",
		},
		{
			name:    "other errors pass through unchanged",
			err:     errors.New("some other error"),
			wantMsg: "some other error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := WrapTimeout(tt.err, tt.operation)
			if tt.wantNil {
				if got != nil {
					t.Errorf("got %v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatal("got nil, want error")
			}
			if got.Error() != tt.wantMsg {
				t.Errorf("got %q, want %q", got.Error(), tt.wantMsg)
			}
		})
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
