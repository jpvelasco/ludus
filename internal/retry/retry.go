package retry

import (
	"context"
	"fmt"
	"math"
	"math/rand/v2"
	"time"
)

// Config controls retry behavior.
type Config struct {
	// MaxAttempts is the total number of attempts (1 = no retry). Default 3.
	MaxAttempts int
	// BaseDelay is the initial delay before the first retry. Default 1s.
	BaseDelay time.Duration
	// MaxDelay caps the backoff delay. Default 30s.
	MaxDelay time.Duration
}

// Default returns a Config suitable for CLI commands (docker push, git clone, etc.).
func Default() Config {
	return Config{
		MaxAttempts: 3,
		BaseDelay:   time.Second,
		MaxDelay:    30 * time.Second,
	}
}

// Do calls fn up to cfg.MaxAttempts times with exponential backoff and jitter.
// It returns nil on the first successful call or the last error if all attempts fail.
// The context is checked between attempts; cancellation stops retries immediately.
func Do(ctx context.Context, cfg Config, fn func() error) error {
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 3
	}
	if cfg.BaseDelay <= 0 {
		cfg.BaseDelay = time.Second
	}
	if cfg.MaxDelay <= 0 {
		cfg.MaxDelay = 30 * time.Second
	}

	var lastErr error
	for attempt := range cfg.MaxAttempts {
		lastErr = fn()
		if lastErr == nil {
			return nil
		}

		// Don't sleep after the last attempt.
		if attempt == cfg.MaxAttempts-1 {
			break
		}

		delay := backoffDelay(attempt, cfg.BaseDelay, cfg.MaxDelay)
		fmt.Printf("    Attempt %d/%d failed, retrying in %s...\n", attempt+1, cfg.MaxAttempts, delay.Round(time.Millisecond))

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
	return lastErr
}

// backoffDelay calculates delay with exponential backoff and full jitter.
// delay = random(0, min(maxDelay, baseDelay * 2^attempt))
func backoffDelay(attempt int, base, max time.Duration) time.Duration {
	exp := math.Pow(2, float64(attempt))
	delay := time.Duration(float64(base) * exp)
	if delay > max {
		delay = max
	}
	// Full jitter: uniform random in [delay/2, delay]
	half := delay / 2
	jitter := time.Duration(rand.Int64N(int64(half) + 1)) //nolint:gosec // jitter for backoff timing, not security
	return half + jitter
}
