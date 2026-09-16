package document

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// The reason a document was cancelled is the audit trail. Every envelope voided by
// hand during the August 2026 cleanup had to have its justification reconstructed
// from admission records afterwards, because the panel writes "cancelled by user"
// for every cause. Service-to-service callers know better, and these are the rules
// for what gets stored.
func TestResolveCancellationReason(t *testing.T) {
	t.Parallel()

	ptr := func(s string) *string { return &s }

	cases := []struct {
		name   string
		reason *string
		want   string
		why    string
	}{
		{
			name:   "nil falls back to the panel wording",
			reason: nil,
			want:   defaultCancellationReason,
			why:    "panel cancellations must read exactly as they did before this change",
		},
		{
			name:   "a real reason is kept",
			reason: ptr("admission cancelled by campus"),
			want:   "admission cancelled by campus",
			why:    "the whole point: the caller knows the cause and it must survive",
		},
		{
			name:   "surrounding whitespace is trimmed",
			reason: ptr("  admission withdrawn by guardian  "),
			want:   "admission withdrawn by guardian",
		},
		{
			name:   "empty string is not a reason",
			reason: ptr(""),
			want:   defaultCancellationReason,
		},
		{
			name:   "whitespace-only is not a reason",
			reason: ptr("   \t\n "),
			want:   defaultCancellationReason,
			why:    "storing blanks reads as a recorded reason while carrying none",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, resolveCancellationReason(tc.reason), tc.why)
		})
	}
}
