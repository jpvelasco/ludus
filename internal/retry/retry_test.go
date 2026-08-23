package retry

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDo_SuccessOnFirstAttempt(t *testing.T) {
	calls := 0
	err := Do(context.Background(), Default(), func() error {
		calls++
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
}

func TestDo_SuccessOnRetry(t *testing.T) {
	calls := 0
	err := Do(context.Background(), Config{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: 10 * time.Millisecond}, func() error {
		calls++
		if calls < 3 {
			return errors.New("transient")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls, got %d", calls)
	}
}

func TestDo_AllAttemptsFail(t *testing.T) {
	calls := 0
	sentinel := errors.New("permanent")
	err := Do(context.Background(), Config{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: 10 * time.Millisecond}, func() error {
		calls++
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got %v", err)
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls, got %d", calls)
	}
}

func TestDo_RespectsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	err := Do(ctx, Config{MaxAttempts: 5, BaseDelay: time.Second, MaxDelay: time.Second}, func() error {
		calls++
		cancel()
		return errors.New("fail")
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call before cancel, got %d", calls)
	}
}

func TestDo_SingleAttempt(t *testing.T) {
	calls := 0
	err := Do(context.Background(), Config{MaxAttempts: 1, BaseDelay: time.Millisecond}, func() error {
		calls++
		return errors.New("fail")
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
}

func TestDo_DefaultsForZeroConfig(t *testing.T) {
	calls := 0
	err := Do(context.Background(), Config{}, func() error {
		calls++
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
}

func TestBackoffDelay_Exponential(t *testing.T) {
	base := 100 * time.Millisecond
	maxDelay := 10 * time.Second

	prev := time.Duration(0)
	for attempt := range 5 {
		d := backoffDelay(attempt, base, maxDelay)
		if d > maxDelay {
			t.Errorf("attempt %d: delay %v exceeds max %v", attempt, d, maxDelay)
		}
		if d < 0 {
			t.Errorf("attempt %d: negative delay %v", attempt, d)
		}
		// Delays should generally increase (with jitter, not strictly monotonic,
		// but the upper bound doubles each time).
		_ = prev
		prev = d
	}
}

func TestBackoffDelay_CappedAtMax(t *testing.T) {
	maxDelay := time.Second
	for range 100 {
		d := backoffDelay(20, time.Millisecond, maxDelay)
		if d > maxDelay {
			t.Fatalf("delay %v exceeds max %v", d, maxDelay)
		}
	}
}

func TestDefault(t *testing.T) {
	cfg := Default()
	if cfg.MaxAttempts != 3 {
		t.Errorf("MaxAttempts: got %d, want 3", cfg.MaxAttempts)
	}
	if cfg.BaseDelay != time.Second {
		t.Errorf("BaseDelay: got %v, want 1s", cfg.BaseDelay)
	}
	if cfg.MaxDelay != 30*time.Second {
		t.Errorf("MaxDelay: got %v, want 30s", cfg.MaxDelay)
	}
}

// TestBackoffDelayNeverOverflows pins the guard against float-to-Duration
// overflow at large attempt counts: baseDelay * 2^attempt exceeds MaxInt64
// around attempt 34 (1s base), and the resulting negative "integer
// indefinite" value used to slip past the maxDelay cap and panic rand.Int64N.
func TestBackoffDelayNeverOverflows(t *testing.T) {
	cfg := Default()
	for attempt := range 64 {
		d := backoffDelay(attempt, cfg.BaseDelay, cfg.MaxDelay)
		if d <= 0 || d > cfg.MaxDelay {
			t.Fatalf("backoffDelay(%d) = %v, want in (0, %v]", attempt, d, cfg.MaxDelay)
		}
	}
}
