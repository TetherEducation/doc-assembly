package entity

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf16"
)

// Limits on the signing email, matching Documenso's ZDocumentMetaSubjectSchema
// and ZDocumentMetaMessageSchema (packages/lib/types/document-meta.ts). Zod
// counts JavaScript string length, so these are UTF-16 code units: an accented
// letter counts 1, an emoji 2.
const (
	MaxSigningEmailSubjectLength = 254
	MaxSigningEmailMessageLength = 5000
)

// jsLength is a string's length as JavaScript (and so Documenso) counts it.
func jsLength(s string) int {
	return len(utf16.Encode([]rune(s)))
}

// signingEmailMetadataKey is where a document keeps its signing email in
// Metadata, beside any other keys a caller stored there.
const signingEmailMetadataKey = "signingEmail"

// SigningEmail is an optional subject and message for the signing provider's
// invitation email, chosen per document by the sender. Empty fields leave the
// provider's default text.
type SigningEmail struct {
	Subject string `json:"subject,omitempty"`
	Message string `json:"message,omitempty"`
}

// Normalized returns the email with surrounding whitespace trimmed.
func (e SigningEmail) Normalized() SigningEmail {
	return SigningEmail{Subject: strings.TrimSpace(e.Subject), Message: strings.TrimSpace(e.Message)}
}

// IsEmpty reports whether neither a subject nor a message is set.
func (e SigningEmail) IsEmpty() bool {
	n := e.Normalized()
	return n.Subject == "" && n.Message == ""
}

// Validate checks the trimmed subject and message against the provider limits.
func (e SigningEmail) Validate() error {
	n := e.Normalized()
	if jsLength(n.Subject) > MaxSigningEmailSubjectLength {
		return fmt.Errorf("%w: the signing email subject is longer than %d characters", ErrFieldTooLong, MaxSigningEmailSubjectLength)
	}
	if jsLength(n.Message) > MaxSigningEmailMessageLength {
		return fmt.Errorf("%w: the signing email message is longer than %d characters", ErrFieldTooLong, MaxSigningEmailMessageLength)
	}
	return nil
}

// SetSigningEmail stores the trimmed subject and message in the document's
// metadata, keeping any other keys. An empty email stores nothing, so the
// document stays exactly as it was.
func (d *Document) SetSigningEmail(email SigningEmail) error {
	if email.IsEmpty() {
		return nil
	}
	if err := email.Validate(); err != nil {
		return err
	}
	meta := map[string]json.RawMessage{}
	if len(d.Metadata) > 0 {
		if err := json.Unmarshal(d.Metadata, &meta); err != nil {
			return fmt.Errorf("reading document metadata: %w", err)
		}
	}
	raw, err := json.Marshal(email.Normalized())
	if err != nil {
		return fmt.Errorf("encoding signing email: %w", err)
	}
	meta[signingEmailMetadataKey] = raw
	out, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("encoding document metadata: %w", err)
	}
	d.Metadata = out
	d.touch()
	return nil
}

// SigningEmail returns the document's signing email, or a zero value when none
// was set or the metadata can't be read.
func (d *Document) SigningEmail() SigningEmail {
	if len(d.Metadata) == 0 {
		return SigningEmail{}
	}
	var meta map[string]json.RawMessage
	if err := json.Unmarshal(d.Metadata, &meta); err != nil {
		return SigningEmail{}
	}
	var email SigningEmail
	if raw, ok := meta[signingEmailMetadataKey]; !ok || json.Unmarshal(raw, &email) != nil {
		return SigningEmail{}
	}
	return email.Normalized()
}
