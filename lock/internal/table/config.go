package table

import (
	"log/slog"
	"time"

	"github.com/sethvargo/go-retry"
)

// Config holds configuration for table locker.
type Config struct {
	TableName         string
	LockID            int64
	LeaseDuration     time.Duration
	HeartbeatInterval time.Duration
	LockTimeout       ProbeConfig
	UnlockTimeout     ProbeConfig

	// Optional logger for lock operations
	Logger *slog.Logger

	// Optional custom retry policy for database errors
	RetryPolicy RetryPolicyFunc
}

// ProbeConfig holds retry configuration.
type ProbeConfig struct {
	IntervalDuration time.Duration
	FailureThreshold uint64
}

// backoff constructs fresh retry state for one Lock or Unlock operation.
func (p ProbeConfig) backoff() retry.Backoff {
	constant := retry.BackoffFunc(func() (time.Duration, bool) {
		return p.IntervalDuration, false
	})
	return retry.WithMaxRetries(
		p.FailureThreshold,
		retry.WithJitterPercent(25, constant),
	)
}
