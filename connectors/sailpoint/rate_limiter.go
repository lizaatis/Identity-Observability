package sailpoint

import (
	"context"
	"fmt"
	"time"
)

// RateLimiter handles IdentityNow API rate limiting with exponential backoff
type RateLimiter struct {
	maxRetries   int
	baseBackoff  time.Duration
	lastRequest  time.Time
	requestCount int
	windowStart  time.Time
	windowSize   time.Duration
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(maxRetries int, baseBackoff, windowSize time.Duration) *RateLimiter {
	return &RateLimiter{
		maxRetries:  maxRetries,
		baseBackoff: baseBackoff,
		windowSize:  windowSize,
		windowStart: time.Now(),
	}
}

// WaitIfNeeded waits if we're approaching rate limits
func (rl *RateLimiter) WaitIfNeeded(ctx context.Context) error {
	now := time.Now()

	// Reset window if needed
	if now.Sub(rl.windowStart) >= rl.windowSize {
		rl.requestCount = 0
		rl.windowStart = now
	}

	// IdentityNow rate limit: typically 300 requests per minute
	// Be conservative: wait if we've made 250 requests in the window
	if rl.requestCount >= 250 {
		waitTime := rl.windowSize - now.Sub(rl.windowStart)
		if waitTime > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(waitTime):
				rl.requestCount = 0
				rl.windowStart = time.Now()
			}
		}
	}

	// Throttle: minimum time between requests
	if !rl.lastRequest.IsZero() {
		elapsed := now.Sub(rl.lastRequest)
		if elapsed < 200*time.Millisecond {
			time.Sleep(200*time.Millisecond - elapsed)
		}
	}

	rl.lastRequest = time.Now()
	rl.requestCount++
	return nil
}

// RetryWithBackoff executes a function with exponential backoff on errors
func (rl *RateLimiter) RetryWithBackoff(ctx context.Context, fn func() error) error {
	var lastErr error
	backoff := rl.baseBackoff

	for attempt := 0; attempt < rl.maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
			backoff = time.Duration(float64(backoff) * 1.5)
		}

		err := fn()
		if err == nil {
			return nil
		}

		lastErr = err

		// Don't retry 404 errors (endpoint not found)
		if isNotFoundError(err) {
			return err
		}

		// Check if it's a rate limit error (429)
		if isRateLimitError(err) {
			// Wait longer for rate limit
			waitTime := backoff * 2
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(waitTime):
			}
		}
	}

	return fmt.Errorf("max retries exceeded: %w", lastErr)
}

// isRateLimitError checks if error is a rate limit (429)
func isRateLimitError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return contains(errStr, "429") ||
		contains(errStr, "rate limit") ||
		contains(errStr, "too many requests")
}

// isNotFoundError checks if error is a 404 (not found)
func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return contains(errStr, "404") ||
		contains(errStr, "endpoint_not_found") ||
		contains(errStr, "not found")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr ||
			(len(s) > len(substr) &&
				(s[:len(substr)] == substr ||
					s[len(s)-len(substr):] == substr ||
					containsMiddle(s, substr))))
}

func containsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
