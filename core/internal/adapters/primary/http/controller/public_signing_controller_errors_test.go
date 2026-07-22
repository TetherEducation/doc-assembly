package controller

import (
	"errors"
	"testing"
)

func TestIsPublicUserError_TreatsMissingSignedPDFAsSafeToShow(t *testing.T) {
	if !isPublicUserError(errors.New("signed PDF not available for this document")) {
		t.Fatal("missing signed PDF should be returned as a public error")
	}
}
