package document

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity/portabledoc"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

// DirectSender implements the opt-in "direct send" policy (signing.direct_send).
//
// By default every new document waits in AWAITING_INPUT until a signer opens
// the public signing page and proceeds, which is when the signing attempt is
// created (PreSigningService.ProceedToSigning). With direct send enabled, a
// document whose template has no signer-fillable interactive fields starts that
// same attempt at creation time, so the signing provider receives the envelope
// (and emails the signers) without any signer action in this engine.
//
// A nil or disabled DirectSender is inert: creation behaves exactly as before.
type DirectSender struct {
	enabled        bool
	signingUOW     port.SigningExecutionUnitOfWork
	signerRoleRepo port.TemplateVersionSignerRoleRepository
}

// NewDirectSender builds the direct-send policy. When enabled is false the
// returned sender never acts.
func NewDirectSender(
	enabled bool,
	signingUOW port.SigningExecutionUnitOfWork,
	signerRoleRepo port.TemplateVersionSignerRoleRepository,
) *DirectSender {
	return &DirectSender{enabled: enabled, signingUOW: signingUOW, signerRoleRepo: signerRoleRepo}
}

// isEnabled reports whether the policy can act at all (nil-safe).
func (d *DirectSender) isEnabled() bool {
	return d != nil && d.enabled
}

// appliesTo reports whether doc should skip the pre-signing step: the policy is
// on, the document is waiting for input with no signing attempt yet, and the
// template content has no signer-fillable fields.
func (d *DirectSender) appliesTo(doc *entity.Document, content json.RawMessage) bool {
	if !d.isEnabled() || doc == nil {
		return false
	}
	if !doc.IsAwaitingInput() {
		return false
	}
	if doc.ActiveAttemptID != nil && *doc.ActiveAttemptID != "" {
		return false
	}
	return !hasSignerFillableFields(content)
}

// appliesToNew reports whether a document about to be created from a template
// with this content should start signing at creation: the policy is on and the
// content has no signer-fillable fields.
func (d *DirectSender) appliesToNew(content json.RawMessage) bool {
	return d.isEnabled() && !hasSignerFillableFields(content)
}

// createAndStart persists a new document (already in AWAITING_INPUT), its
// recipients and its first signing attempt in one transaction
// (SigningExecutionUnitOfWork.CreateDocumentAndStartSigning). On error nothing
// is written, so the caller can report the failure and a retry creates exactly
// one document. On success doc and recipients carry their persisted IDs.
func (d *DirectSender) createAndStart(
	ctx context.Context,
	doc *entity.Document,
	recipients []*entity.DocumentRecipient,
) (*entity.SigningAttempt, error) {
	signerOrders, err := d.signerOrders(ctx, doc.TemplateVersionID)
	if err != nil {
		return nil, err
	}

	attempt, err := d.signingUOW.CreateDocumentAndStartSigning(ctx, doc, recipients, signerOrders)
	if err != nil {
		slog.ErrorContext(ctx, "direct send: creating document with its signing attempt failed; nothing was persisted",
			slog.String("template_version_id", doc.TemplateVersionID),
			slog.String("error", err.Error()),
		)
		return nil, fmt.Errorf("direct send: creating document and signing attempt: %w", err)
	}

	slog.InfoContext(ctx, "direct send: document created with its signing attempt enqueued",
		slog.String("document_id", doc.ID),
		slog.String("attempt_id", attempt.ID),
		slog.Int("recipients", len(recipients)),
	)
	return attempt, nil
}

// signerOrders maps each signer role of a template version to its signing order.
func (d *DirectSender) signerOrders(ctx context.Context, templateVersionID string) (map[string]int, error) {
	roles, err := d.signerRoleRepo.FindByVersionID(ctx, templateVersionID)
	if err != nil {
		return nil, fmt.Errorf("direct send: loading signer roles: %w", err)
	}
	signerOrders := make(map[string]int, len(roles))
	for _, role := range roles {
		signerOrders[role.ID] = role.SignerOrder
	}
	return signerOrders, nil
}

// start creates the signing attempt and enqueues the render job through the
// same unit of work ProceedToSigning uses, for a document that already exists
// (the internal API, which persists first and recovers a failed start through
// its idempotent replay). The attempt, the document's PREPARING_SIGNATURE
// projection, the River job and the ATTEMPT_CREATED event commit atomically; on
// error nothing is written and the document stays in AWAITING_INPUT, where the
// normal signer flow can still start the attempt.
func (d *DirectSender) start(
	ctx context.Context,
	doc *entity.Document,
	recipients []*entity.DocumentRecipient,
) (*entity.SigningAttempt, error) {
	signerOrders, err := d.signerOrders(ctx, doc.TemplateVersionID)
	if err != nil {
		return nil, err
	}

	attempt, err := d.signingUOW.CreateAttemptAndEnqueueRender(ctx, doc.ID, recipients, signerOrders)
	if err != nil {
		slog.ErrorContext(ctx, "direct send: creating signing attempt failed; document left awaiting input",
			slog.String("document_id", doc.ID),
			slog.String("error", err.Error()),
		)
		return nil, fmt.Errorf("direct send: creating signing attempt: %w", err)
	}

	slog.InfoContext(ctx, "direct send: signing attempt enqueued at creation",
		slog.String("document_id", doc.ID),
		slog.String("attempt_id", attempt.ID),
		slog.Int("recipients", len(recipients)),
	)
	return attempt, nil
}

// hasSignerFillableFields reports whether template content contains any
// interactive field (the inputs the pre-signing form collects). Content that
// cannot be parsed counts as having fields, so the document falls back to the
// default AWAITING_INPUT flow instead of being sent without its inputs.
func hasSignerFillableFields(content json.RawMessage) bool {
	doc, err := portabledoc.Parse(content)
	if err != nil {
		return true
	}
	if doc == nil {
		return false
	}
	return doc.HasNodeOfType(portabledoc.NodeTypeInteractiveField)
}
