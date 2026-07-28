package sdk

import "github.com/TetherEducation/doc-assembly/core/internal/core/port"

// DocumentCompletedHandler is the callback invoked when a document reaches
// COMPLETED status. Return an error to trigger automatic retry.
type DocumentCompletedHandler = port.DocumentCompletedHandler

// DocumentCompletedEvent carries data about a completed document.
type DocumentCompletedEvent = port.DocumentCompletedEvent

// CompletedRecipient holds signer information within a completed document event.
type CompletedRecipient = port.CompletedRecipient
