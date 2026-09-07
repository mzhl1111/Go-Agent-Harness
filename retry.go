package main

import "time"

// RetryPolicy bounds retries for one operation. A zero MaxAttempts means one.
type RetryPolicy struct {
	MaxAttempts int
	Backoff     time.Duration
}
