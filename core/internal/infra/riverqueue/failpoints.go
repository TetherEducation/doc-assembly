package riverqueue

import "fmt"

const (
	failpointRenderBefore                        = "render.before"
	failpointRenderAfterStoreBeforeCommit        = "render.after_store_before_commit"
	failpointSubmitBeforeProvider                = "submit.before_provider"
	failpointSubmitCorruptPDFChecksum            = "submit.corrupt_pdf_checksum"
	failpointSubmitAfterProviderBeforeCommit     = "submit.after_provider_accepted_before_commit"
	failpointSubmitAfterEnvelopeBeforeRecipients = "submit_after_envelope_before_recipients"
	failpointSubmitAfterRecipientsBeforeFields   = "submit_after_recipients_before_fields"
	failpointSubmitAfterFieldsBeforeDistribute   = "submit_after_fields_before_distribute"
	failpointSubmitAfterDistributeBeforeRefs     = "submit_after_distribute_before_refs"
	failpointCleanupFail                         = "cleanup.fail"
)

// AttemptFailpoints contains non-production failure injection toggles for live
// DoD verification. It is intentionally inert unless explicitly enabled by
// WorkerConfig and blocked for production runtime config in riverqueue.New.
type AttemptFailpoints map[string]bool

func newAttemptFailpoints(names []string) AttemptFailpoints {
	fp := make(AttemptFailpoints, len(names))
	for _, name := range names {
		if name != "" {
			fp[name] = true
		}
	}
	return fp
}

func (f AttemptFailpoints) Enabled(name string) bool {
	if len(f) == 0 {
		return false
	}
	return f[name]
}

func failpointErr(name string) error {
	return fmt.Errorf("signing attempt failpoint triggered: %s", name)
}
