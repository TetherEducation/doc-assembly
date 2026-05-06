package riverqueue

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRefreshAttemptProviderStatusArgsIncludesTimeBucket(t *testing.T) {
	attemptID := "attempt-1"
	at := time.Date(2026, 5, 6, 12, 0, 8, 0, time.UTC)

	got := refreshAttemptProviderStatusArgs(attemptID, at)

	assert.Equal(t, attemptID, got.AttemptID)
	assert.Equal(t, "2026-05-06T12:00:00Z", got.Bucket)
	assert.Equal(t, got.Bucket, refreshAttemptProviderStatusArgs(attemptID, at.Add(time.Second)).Bucket)
	assert.NotEqual(t, got.Bucket, refreshAttemptProviderStatusArgs(attemptID, at.Add(publicSigningRefreshBucket)).Bucket)
}
