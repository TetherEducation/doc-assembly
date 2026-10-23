package entity

import (
	"testing"
	"time"
)

func TestSigningAttemptStatusTerminalAndRetryEligibility(t *testing.T) {
	terminal := []SigningAttemptStatus{
		SigningAttemptStatusCompleted,
		SigningAttemptStatusDeclined,
		SigningAttemptStatusInvalidated,
		SigningAttemptStatusSuperseded,
		SigningAttemptStatusCancelled,
		SigningAttemptStatusRequiresReview,
		SigningAttemptStatusFailedPermanent,
	}
	for _, status := range terminal {
		attempt := &SigningAttempt{Status: status}
		if !attempt.IsTerminal() {
			t.Fatalf("%s should be terminal", status)
		}
		if attempt.CanRetryAutomatically() {
			t.Fatalf("%s should not be automatically retryable", status)
		}
	}

	for _, status := range []SigningAttemptStatus{SigningAttemptStatusProviderRetryWaiting, SigningAttemptStatusSubmissionUnknown, SigningAttemptStatusReconcilingProvider} {
		attempt := &SigningAttempt{Status: status}
		if attempt.IsTerminal() {
			t.Fatalf("%s should not be terminal", status)
		}
	}
}

func TestDocumentStatusProjectionFromActiveAttempt(t *testing.T) {
	cases := []struct {
		attempt SigningAttemptStatus
		expect  DocumentStatus
	}{
		{SigningAttemptStatusCreated, DocumentStatusPreparingSignature},
		{SigningAttemptStatusRendering, DocumentStatusPreparingSignature},
		{SigningAttemptStatusProviderRetryWaiting, DocumentStatusPreparingSignature},
		{SigningAttemptStatusSubmissionUnknown, DocumentStatusPreparingSignature},
		{SigningAttemptStatusSigningReady, DocumentStatusReadyToSign},
		{SigningAttemptStatusSigning, DocumentStatusSigning},
		{SigningAttemptStatusCompleted, DocumentStatusCompleted},
		{SigningAttemptStatusDeclined, DocumentStatusDeclined},
		{SigningAttemptStatusFailedPermanent, DocumentStatusError},
		{SigningAttemptStatusRequiresReview, DocumentStatusError},
	}

	for _, tc := range cases {
		if got := ProjectDocumentStatusFromAttempt(tc.attempt); got != tc.expect {
			t.Fatalf("%s projected to %s, want %s", tc.attempt, got, tc.expect)
		}
	}
}

func TestProviderErrorClassMapsToAttemptStatus(t *testing.T) {
	cases := []struct {
		class  ProviderErrorClass
		expect SigningAttemptStatus
	}{
		{ProviderErrorClassTransient, SigningAttemptStatusProviderRetryWaiting},
		{ProviderErrorClassPermanent, SigningAttemptStatusFailedPermanent},
		{ProviderErrorClassAmbiguous, SigningAttemptStatusSubmissionUnknown},
		{ProviderErrorClassConflictStale, SigningAttemptStatusRequiresReview},
	}

	for _, tc := range cases {
		if got := AttemptStatusForProviderError(tc.class); got != tc.expect {
			t.Fatalf("%s mapped to %s, want %s", tc.class, got, tc.expect)
		}
	}
}

var inFlightAttemptStatuses = []SigningAttemptStatus{
	SigningAttemptStatusCreated,
	SigningAttemptStatusRendering,
	SigningAttemptStatusPDFReady,
	SigningAttemptStatusReadyToSubmit,
	SigningAttemptStatusSubmittingProvider,
	SigningAttemptStatusProviderRetryWaiting,
	SigningAttemptStatusSubmissionUnknown,
	SigningAttemptStatusReconcilingProvider,
}

func TestApplyProviderReportLeavesInFlightSubmissionToTheWorker(t *testing.T) {
	now := time.Date(2026, 10, 22, 18, 9, 7, 0, time.UTC)
	for _, current := range inFlightAttemptStatuses {
		for _, echoed := range []SigningAttemptStatus{SigningAttemptStatusSigningReady, SigningAttemptStatusSigning} {
			// Documenso sends DOCUMENT_CREATED (SIGNING_READY) while the worker is still
			// adding recipients and fields; accepting it strands the next phase job.
			phase := ProviderSubmitPhaseCreateFields
			attempt := &SigningAttempt{Status: current, ProviderSubmitPhase: &phase}
			changed := attempt.ApplyProviderReport(ProviderReport{Status: &echoed}, now)
			if changed || attempt.Status != current || *attempt.ProviderSubmitPhase != phase {
				t.Fatalf("%s moved an in-flight %s attempt to %s", echoed, current, attempt.Status)
			}
		}
	}
}

func TestApplyProviderReportAlwaysAppliesTerminalStatuses(t *testing.T) {
	now := time.Date(2026, 10, 22, 18, 9, 7, 0, time.UTC)
	for _, current := range append(inFlightAttemptStatuses, SigningAttemptStatusSigningReady, SigningAttemptStatusSigning) {
		for _, final := range []SigningAttemptStatus{SigningAttemptStatusCompleted, SigningAttemptStatusDeclined} {
			attempt := &SigningAttempt{Status: current}
			changed := attempt.ApplyProviderReport(ProviderReport{Status: &final}, now)
			if !changed || attempt.Status != final || attempt.TerminalAt == nil || !attempt.TerminalAt.Equal(now) {
				t.Fatalf("terminal %s applied to %s as %s at %v", final, current, attempt.Status, attempt.TerminalAt)
			}
		}
	}
}

func TestApplyProviderReportMovesASubmittedAttempt(t *testing.T) {
	now := time.Now()
	signing := SigningAttemptStatusSigning
	ready := &SigningAttempt{Status: SigningAttemptStatusSigningReady}
	if !ready.ApplyProviderReport(ProviderReport{Status: &signing}, now) || ready.Status != SigningAttemptStatusSigning {
		t.Fatalf("DOCUMENT_OPENED must move a signing-ready attempt to SIGNING, got %s", ready.Status)
	}
	if ready.TerminalAt != nil {
		t.Fatalf("a non-terminal status must not set TerminalAt")
	}
	if ready.ApplyProviderReport(ProviderReport{Status: &signing}, now) {
		t.Fatalf("a repeated status must not count as a change")
	}
}

func TestApplyProviderReportOnlyFillsMissingIdentifiers(t *testing.T) {
	now := time.Now()
	workerKey, workerDoc := "doc:attempt", "envelope_worker"
	attempt := &SigningAttempt{Status: SigningAttemptStatusSubmittingProvider, ProviderCorrelationKey: &workerKey, ProviderDocumentID: &workerDoc}
	attempt.ApplyProviderReport(ProviderReport{CorrelationKey: "other", DocumentID: "envelope_webhook"}, now)
	if *attempt.ProviderCorrelationKey != workerKey || *attempt.ProviderDocumentID != workerDoc {
		t.Fatalf("webhook overwrote worker identifiers: %s / %s", *attempt.ProviderCorrelationKey, *attempt.ProviderDocumentID)
	}

	empty := &SigningAttempt{Status: SigningAttemptStatusSigningReady}
	empty.ApplyProviderReport(ProviderReport{CorrelationKey: "doc:attempt", DocumentID: "envelope_webhook"}, now)
	if empty.ProviderCorrelationKey == nil || *empty.ProviderCorrelationKey != "doc:attempt" || empty.ProviderDocumentID == nil || *empty.ProviderDocumentID != "envelope_webhook" {
		t.Fatalf("webhook identifiers were not recorded on an attempt that had none")
	}
}
