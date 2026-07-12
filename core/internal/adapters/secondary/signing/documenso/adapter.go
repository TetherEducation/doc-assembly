//nolint:gosec
package documenso

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/rendis/doc-assembly/core/internal/core/entity"
	"github.com/rendis/doc-assembly/core/internal/core/port"
)

const (
	providerName         = "documenso"
	documensoHTTPTimeout = 120 * time.Second
)

// Adapter implements port.SigningProvider for Documenso.
type Adapter struct {
	config     *Config
	httpClient *http.Client
}

// New creates a new Documenso adapter.
func New(config *Config) (*Adapter, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	return &Adapter{
		config: config,
		httpClient: &http.Client{
			Timeout: documensoHTTPTimeout,
		},
	}, nil
}

// ProviderName returns the name of this signing provider.
func (a *Adapter) ProviderName() string {
	return providerName
}

func (a *Adapter) ProviderCapabilities() port.ProviderCapabilities {
	return port.ProviderCapabilities{
		CanFindByCorrelationKey: true,
		CanCancel:               true,
		CanEmbedSigning:         true,
		CanDownloadCompletedPDF: true,
		WebhookIncludesIDs:      false,
	}
}

type signingURLResult struct {
	SigningURL string
	ExpiresAt  *time.Time
}

// setAuthHeader sets the authorization header on the request.
func (a *Adapter) setAuthHeader(req *http.Request) {
	req.Header.Set("Authorization", a.config.APIKey)
}

// SubmitAttemptDocument uploads a PDF document to Documenso and creates a signing envelope.
func (a *Adapter) SubmitAttemptDocument(ctx context.Context, req *port.SubmitAttemptDocumentRequest) (*port.SubmitAttemptDocumentResult, error) {
	envelopeID, err := a.createEnvelope(ctx, req.Title, req.CorrelationKey, req.PDF)
	if err != nil {
		return nil, err
	}

	recipientsResp, err := a.addRecipients(ctx, envelopeID, req.Recipients)
	if err != nil {
		return nil, err
	}

	// Create signature fields for each recipient before distributing
	if len(req.SignatureFields) > 0 {
		if err := a.createSignatureFields(ctx, envelopeID, recipientsResp, req.SignatureFields, req.Recipients); err != nil {
			return nil, fmt.Errorf("creating signature fields: %w", err)
		}
	}

	if err := a.distributeEnvelope(ctx, envelopeID); err != nil {
		return nil, err
	}

	// Fetch envelope details to get recipient tokens for signing URLs
	envDetails, err := a.fetchEnvelope(ctx, envelopeID)
	if err != nil {
		return nil, fmt.Errorf("fetching envelope details for signing URLs: %w", err)
	}

	return a.buildUploadResult(envelopeID, envDetails, req.Recipients, req.CorrelationKey), nil
}

// buildEnvelopeBody builds the multipart form body for creating an envelope.
func buildEnvelopeBody(title, externalRef string, pdf []byte) (*bytes.Buffer, string, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", `form-data; name="files"; filename="document.pdf"`)
	h.Set("Content-Type", "application/pdf")

	part, err := writer.CreatePart(h)
	if err != nil {
		return nil, "", fmt.Errorf("creating form file: %w", err)
	}
	if _, err := part.Write(pdf); err != nil {
		return nil, "", fmt.Errorf("writing PDF to form: %w", err)
	}

	payload := map[string]any{"title": title, "type": "DOCUMENT"}
	if externalRef != "" {
		payload["externalId"] = externalRef
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, "", fmt.Errorf("marshaling payload: %w", err)
	}
	if err := writer.WriteField("payload", string(payloadJSON)); err != nil {
		return nil, "", fmt.Errorf("writing payload field: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, "", fmt.Errorf("closing multipart writer: %w", err)
	}

	return &buf, writer.FormDataContentType(), nil
}

// createEnvelope creates a new envelope with the PDF document.
func (a *Adapter) createEnvelope(ctx context.Context, title string, externalRef string, pdf []byte) (string, error) {
	buf, contentType, err := buildEnvelopeBody(title, externalRef, pdf)
	if err != nil {
		return "", err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.config.BaseURL+"/envelope/create", buf)
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}

	a.setAuthHeader(httpReq)
	httpReq.Header.Set("Content-Type", contentType)

	resp, err := a.httpClient.Do(httpReq) //nolint:gosec // URL is built from configured provider base URL
	if err != nil {
		return "", fmt.Errorf("executing request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("documenso API error (status %d): %s", resp.StatusCode, string(body))
	}

	var createResp envelopeResponse
	if err := json.NewDecoder(resp.Body).Decode(&createResp); err != nil {
		return "", fmt.Errorf("decoding response: %w", err)
	}

	return createResp.ID, nil
}

// addRecipients adds recipients to an envelope.
func (a *Adapter) addRecipients(ctx context.Context, envelopeID string, recipients []port.SigningRecipient) (*recipientsResponse, error) {
	payloads := make([]recipientPayload, len(recipients))
	for i, r := range recipients {
		payloads[i] = recipientPayload{
			Email:        r.Email,
			Name:         r.Name,
			Role:         "SIGNER",
			SigningOrder: r.SignerOrder,
			ExternalID:   r.RoleID,
		}
	}

	recipientsReq := recipientsRequest{
		EnvelopeID: envelopeID,
		Data:       payloads,
	}

	recipientsBody, err := json.Marshal(recipientsReq)
	if err != nil {
		return nil, fmt.Errorf("marshaling recipients: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.config.BaseURL+"/envelope/recipient/create-many", bytes.NewReader(recipientsBody))
	if err != nil {
		return nil, fmt.Errorf("creating recipients request: %w", err)
	}

	a.setAuthHeader(httpReq)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := a.httpClient.Do(httpReq) //nolint:gosec // URL is built from configured provider base URL
	if err != nil {
		return nil, fmt.Errorf("executing recipients request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("documenso API error adding recipients (status %d): %s", resp.StatusCode, string(body))
	}

	var recipientsResp recipientsResponse
	if err := json.NewDecoder(resp.Body).Decode(&recipientsResp); err != nil {
		return nil, fmt.Errorf("decoding recipients response: %w", err)
	}

	return &recipientsResp, nil
}

// createSignatureFields creates signature fields for each recipient in the envelope.
func (a *Adapter) createSignatureFields(
	ctx context.Context,
	envelopeID string,
	recipientsResp *recipientsResponse,
	signatureFields []port.SignatureFieldPosition,
	recipients []port.SigningRecipient,
) error {
	if len(signatureFields) == 0 {
		return nil
	}

	fieldPayloads := a.buildFieldPayloads(signatureFields, recipients, recipientsResp)
	if len(fieldPayloads) == 0 {
		slog.WarnContext(ctx, "no field payloads built from signature fields")
		return nil
	}

	return a.sendFieldsToAPI(ctx, envelopeID, fieldPayloads)
}

// buildFieldPayloads creates field payloads from signature field positions.
func (a *Adapter) buildFieldPayloads(
	signatureFields []port.SignatureFieldPosition,
	recipients []port.SigningRecipient,
	recipientsResp *recipientsResponse,
) []fieldPayload {
	roleToRecipientIdx := make(map[string]int, len(recipients))
	for i, r := range recipients {
		roleToRecipientIdx[r.RoleID] = i
	}

	fieldPayloads := make([]fieldPayload, 0, len(signatureFields))
	for _, sf := range signatureFields {
		payload := a.buildSingleFieldPayload(sf, roleToRecipientIdx, recipientsResp)
		if payload != nil {
			fieldPayloads = append(fieldPayloads, *payload)
		}
	}
	return fieldPayloads
}

// buildSingleFieldPayload creates a single field payload or returns nil if not possible.
func (a *Adapter) buildSingleFieldPayload(
	sf port.SignatureFieldPosition,
	roleToRecipientIdx map[string]int,
	recipientsResp *recipientsResponse,
) *fieldPayload {
	recipientIdx, ok := roleToRecipientIdx[sf.RoleID]
	if !ok || recipientIdx >= len(recipientsResp.Data) {
		return nil
	}

	providerRecipientID := recipientsResp.Data[recipientIdx].ID

	return &fieldPayload{
		RecipientID: providerRecipientID,
		Type:        "SIGNATURE",
		Page:        sf.Page,
		PositionX:   sf.PositionX,
		PositionY:   sf.PositionY,
		Width:       sf.Width,
		Height:      sf.Height,
	}
}

// sendFieldsToAPI sends field creation request to Documenso API.
func (a *Adapter) sendFieldsToAPI(ctx context.Context, envelopeID string, fieldPayloads []fieldPayload) error {
	fieldsReq := fieldsRequest{EnvelopeID: envelopeID, Data: fieldPayloads}

	fieldsBody, err := json.Marshal(fieldsReq)
	if err != nil {
		return fmt.Errorf("marshaling fields request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		a.config.BaseURL+"/envelope/field/create-many", bytes.NewReader(fieldsBody))
	if err != nil {
		return fmt.Errorf("creating fields request: %w", err)
	}

	a.setAuthHeader(httpReq)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := a.httpClient.Do(httpReq) //nolint:gosec // URL is built from configured provider base URL
	if err != nil {
		return fmt.Errorf("executing fields request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("documenso API error creating fields (status %d): %s", resp.StatusCode, string(body))
	}

	return nil
}

// distributeEnvelope sends the envelope for signing.
func (a *Adapter) distributeEnvelope(ctx context.Context, envelopeID string) error {
	return a.postEnvelopeAction(
		ctx,
		envelopeID,
		"/envelope/distribute",
		"marshaling send request",
		"creating distribute request",
		"executing distribute request",
		"documenso API error distributing envelope",
	)
}

// buildUploadResult constructs the upload result from the envelope details.
// It matches recipients by index since the order is preserved from the original request.
func (a *Adapter) buildUploadResult(envelopeID string, envDetails *envelopeDetailResponse, originalRecipients []port.SigningRecipient, correlationKey string) *port.SubmitAttemptDocumentResult {
	result := &port.SubmitAttemptDocumentResult{
		ProviderDocumentID: envelopeID,
		ProviderName:       providerName,
		CorrelationKey:     correlationKey,
		InitialStatus:      entity.SigningAttemptStatusSigningReady,
		Recipients:         make([]port.RecipientResult, 0, len(originalRecipients)),
	}

	// Match recipients by index (order is preserved from request)
	for i, orig := range originalRecipients {
		if i >= len(envDetails.Recipients) {
			continue
		}
		providerRecipient := envDetails.Recipients[i]

		signingURL := fmt.Sprintf("%s/sign/%s", a.config.SigningBaseURL, providerRecipient.Token)

		result.Recipients = append(result.Recipients, port.RecipientResult{
			RoleID:               orig.RoleID,
			ProviderRecipientID:  strconv.Itoa(providerRecipient.ID),
			ProviderSigningToken: providerRecipient.Token,
			SigningURL:           signingURL,
			Status:               entity.RecipientStatusSent,
		})
	}

	return result
}

// FindProviderDocumentByCorrelationKey reconciles ambiguous submissions by
// scanning recent Documenso envelopes for the externalId written during create.
func (a *Adapter) FindProviderDocumentByCorrelationKey(ctx context.Context, req *port.FindProviderDocumentRequest) (*port.ProviderDocumentResult, error) {
	base := &port.ProviderDocumentResult{
		Found:          false,
		Usable:         false,
		ProviderName:   providerName,
		CorrelationKey: req.CorrelationKey,
	}

	detail, found, err := a.findEnvelopeByCorrelationKey(ctx, req.CorrelationKey)
	if err != nil {
		return nil, err
	}
	if !found {
		base.Reason = "documenso envelope with matching externalId was not found"
		return base, nil
	}

	return a.buildProviderDocumentResultFromEnvelope(detail, req.CorrelationKey), nil
}

// InspectProviderSubmission returns the durable provider-side submission stages
// that already exist for an attempt correlation key.
func (a *Adapter) InspectProviderSubmission(ctx context.Context, req *port.FindProviderDocumentRequest) (*port.ProviderSubmissionSnapshot, error) {
	base := &port.ProviderSubmissionSnapshot{
		ProviderName:   providerName,
		CorrelationKey: req.CorrelationKey,
		HasDocument:    false,
	}

	detail, found, err := a.findEnvelopeByCorrelationKey(ctx, req.CorrelationKey)
	if err != nil {
		return nil, err
	}
	if !found {
		base.Reason = "documenso envelope with matching externalId was not found"
		return base, nil
	}

	return a.buildProviderSubmissionSnapshotFromEnvelope(detail, req.CorrelationKey), nil
}

// EnsureProviderDocument creates the provider document once, reusing an
// existing Documenso envelope when its externalId matches the attempt
// correlation key.
func (a *Adapter) EnsureProviderDocument(ctx context.Context, req *port.EnsureProviderDocumentRequest) (*port.EnsureProviderDocumentResult, error) {
	detail, found, err := a.findEnvelopeByCorrelationKey(ctx, req.CorrelationKey)
	if err != nil {
		return nil, err
	}
	if found {
		return &port.EnsureProviderDocumentResult{
			ProviderDocumentID: detail.ID,
			ProviderName:       providerName,
			CorrelationKey:     req.CorrelationKey,
			RawStatus:          detail.Status,
		}, nil
	}

	envelopeID, err := a.createEnvelope(ctx, req.Title, req.CorrelationKey, req.PDF)
	if err != nil {
		return nil, err
	}

	detail, err = a.fetchEnvelope(ctx, envelopeID)
	if err != nil {
		return nil, err
	}

	return &port.EnsureProviderDocumentResult{
		ProviderDocumentID: envelopeID,
		ProviderName:       providerName,
		CorrelationKey:     req.CorrelationKey,
		RawStatus:          detail.Status,
	}, nil
}

// EnsureProviderRecipients creates recipients only when the envelope has none.
// If a partial recipient set already exists, it fails rather than risking a
// duplicate/partial create-many repair against an unknown provider state.
func (a *Adapter) EnsureProviderRecipients(ctx context.Context, req *port.EnsureProviderRecipientsRequest) (*port.EnsureProviderRecipientsResult, error) {
	detail, err := a.fetchEnvelope(ctx, req.ProviderDocumentID)
	if err != nil {
		return nil, err
	}
	if err := validateEnvelopeCorrelation(detail, req.CorrelationKey, entity.ProviderSubmitPhaseAddRecipients); err != nil {
		return nil, err
	}

	if len(detail.Recipients) == 0 {
		if _, err := a.addRecipients(ctx, req.ProviderDocumentID, req.Recipients); err != nil {
			return nil, err
		}
	} else if missing := missingSigningRecipientRoles(detail, req.Recipients); len(missing) > 0 {
		return nil, newDocumensoProviderError(
			entity.ProviderErrorClassPermanent,
			entity.ProviderSubmitPhaseAddRecipients,
			req.ProviderDocumentID,
			fmt.Sprintf("documenso envelope is missing expected recipients: %s", strings.Join(missing, ", ")),
		)
	}

	detail, err = a.fetchEnvelope(ctx, req.ProviderDocumentID)
	if err != nil {
		return nil, err
	}
	if err := validateEnvelopeCorrelation(detail, req.CorrelationKey, entity.ProviderSubmitPhaseAddRecipients); err != nil {
		return nil, err
	}

	return &port.EnsureProviderRecipientsResult{
		Recipients: a.buildRecipientResultsFromEnvelope(detail, req.Recipients, false),
	}, nil
}

// EnsureProviderFields creates signature fields only when the envelope does
// not already expose fields in its detail response.
func (a *Adapter) EnsureProviderFields(ctx context.Context, req *port.EnsureProviderFieldsRequest) (*port.EnsureProviderFieldsResult, error) {
	detail, err := a.fetchEnvelope(ctx, req.ProviderDocumentID)
	if err != nil {
		return nil, err
	}
	if err := validateEnvelopeCorrelation(detail, req.CorrelationKey, entity.ProviderSubmitPhaseCreateFields); err != nil {
		return nil, err
	}

	if len(req.SignatureFields) == 0 {
		return &port.EnsureProviderFieldsResult{FieldCount: 0}, nil
	}

	fieldPayloads := buildMissingSignatureFieldPayloads(detail, req.SignatureFields, req.Recipients)
	if len(fieldPayloads) == 0 {
		requiredCount := countRequiredSignatureFields(req.SignatureFields, req.Recipients)
		if requiredCount == 0 {
			slog.WarnContext(ctx, "no field payloads built from provider recipient references")
		}
		return &port.EnsureProviderFieldsResult{FieldCount: requiredCount}, nil
	}

	if err := a.sendFieldsToAPI(ctx, req.ProviderDocumentID, fieldPayloads); err != nil {
		return nil, err
	}

	return &port.EnsureProviderFieldsResult{FieldCount: countRequiredSignatureFields(req.SignatureFields, req.Recipients)}, nil
}

// EnsureProviderDistributed sends the envelope only when it is still a draft.
func (a *Adapter) EnsureProviderDistributed(ctx context.Context, req *port.EnsureProviderDistributedRequest) (*port.EnsureProviderDistributedResult, error) {
	detail, err := a.fetchEnvelope(ctx, req.ProviderDocumentID)
	if err != nil {
		return nil, err
	}
	if err := validateEnvelopeCorrelation(detail, req.CorrelationKey, entity.ProviderSubmitPhaseDistributeDocument); err != nil {
		return nil, err
	}

	if !isDistributedStatus(detail.Status) {
		if err := a.distributeEnvelope(ctx, req.ProviderDocumentID); err != nil {
			return nil, err
		}
		detail, err = a.fetchEnvelope(ctx, req.ProviderDocumentID)
		if err != nil {
			return nil, err
		}
		if err := validateEnvelopeCorrelation(detail, req.CorrelationKey, entity.ProviderSubmitPhaseDistributeDocument); err != nil {
			return nil, err
		}
	}

	return &port.EnsureProviderDistributedResult{RawStatus: detail.Status}, nil
}

// FetchProviderSigningReferences returns usable signing URLs and statuses from
// the provider envelope after distribution.
func (a *Adapter) FetchProviderSigningReferences(ctx context.Context, req *port.FetchProviderSigningReferencesRequest) (*port.FetchProviderSigningReferencesResult, error) {
	detail, err := a.fetchEnvelope(ctx, req.ProviderDocumentID)
	if err != nil {
		return nil, err
	}
	if err := validateEnvelopeCorrelation(detail, req.CorrelationKey, entity.ProviderSubmitPhaseFetchSigningReferences); err != nil {
		return nil, err
	}

	providerResult := a.buildProviderDocumentResultFromEnvelope(detail, req.CorrelationKey)
	if !providerResult.Usable && len(req.Recipients) > 0 {
		fallbackRecipients := a.buildRecipientResultsFromEnvelope(detail, req.Recipients, true)
		if len(fallbackRecipients) > 0 {
			providerResult.Usable = true
			providerResult.Reason = ""
			providerResult.Recipients = fallbackRecipients
		}
	}
	if !providerResult.Usable {
		return nil, newDocumensoProviderError(
			entity.ProviderErrorClassPermanent,
			entity.ProviderSubmitPhaseFetchSigningReferences,
			req.ProviderDocumentID,
			providerResult.Reason,
		)
	}

	return &port.FetchProviderSigningReferencesResult{
		Recipients: providerResult.Recipients,
		Status:     providerResult.Status,
		RawStatus:  providerResult.RawStatus,
	}, nil
}

// GetSigningURL returns the URL where a specific recipient can sign the document.
func (a *Adapter) getSigningURL(ctx context.Context, providerDocumentID, providerRecipientID string) (*signingURLResult, error) {
	envResp, err := a.fetchEnvelope(ctx, providerDocumentID)
	if err != nil {
		return nil, err
	}

	// Find the recipient and their signing token
	reqRecipientID, err := strconv.Atoi(providerRecipientID)
	if err != nil {
		return nil, fmt.Errorf("invalid recipient ID: %w", err)
	}

	for _, r := range envResp.Recipients {
		if r.ID == reqRecipientID {
			signingURL := fmt.Sprintf("%s/sign/%s", a.config.SigningBaseURL, r.Token)

			return &signingURLResult{
				SigningURL: signingURL,
			}, nil
		}
	}

	return nil, fmt.Errorf("recipient %s not found in envelope", providerRecipientID)
}

// GetEmbeddedSigningURL returns the signing URL suitable for iframe embedding.
// Documenso does not natively support redirect-based callbacks, so CallbackURL is ignored.
// Detection of completion relies on webhooks + polling.
func (a *Adapter) GetAttemptRecipientEmbeddedURL(ctx context.Context, req *port.GetAttemptRecipientEmbeddedURLRequest) (*port.GetAttemptRecipientEmbeddedURLResult, error) {
	signingResult, err := a.getSigningURL(ctx, req.ProviderDocumentID, req.ProviderRecipientID)
	if err != nil {
		return nil, err
	}

	// Replace /sign/ with /embed/sign/ for iframe embedding
	embeddedURL := strings.Replace(signingResult.SigningURL, "/sign/", "/embed/sign/", 1)

	// Documenso reads widget options (theme, prefilled signer name, language,
	// branding) from the URL fragment; the fragment never reaches the server.
	if fragment, ok := a.config.Embed.fragment(req.RecipientName); ok {
		embeddedURL += "#" + fragment
	}

	return &port.GetAttemptRecipientEmbeddedURLResult{
		EmbeddedURL:    embeddedURL,
		FrameSrcDomain: a.config.SigningBaseURL,
		ExpiresAt:      signingResult.ExpiresAt,
	}, nil
}

// GetDocumentStatus retrieves the current status of a document from Documenso.
func (a *Adapter) GetProviderDocumentStatus(ctx context.Context, req *port.GetProviderDocumentStatusRequest) (*port.ProviderDocumentStatusResult, error) {
	envResp, err := a.fetchEnvelope(ctx, req.ProviderDocumentID)
	if err != nil {
		return nil, err
	}

	recipientResults, allSigned, anyDeclined := processRecipients(envResp.Recipients)

	result := &port.ProviderDocumentStatusResult{
		Status:         MapEnvelopeStatus(envResp.Status),
		ProviderStatus: envResp.Status,
		Recipients:     recipientResults,
	}

	result.Status = determineFinalStatus(envResp.Status, allSigned, anyDeclined, len(envResp.Recipients), recipientResults)

	if result.Status == entity.SigningAttemptStatusCompleted && envResp.CompletedDocumentURL != "" {
		result.CompletedPDFURL = &envResp.CompletedDocumentURL
	}

	return result, nil
}

// DownloadCompletedPDF downloads the completed/signed PDF bytes from Documenso.
func (a *Adapter) DownloadCompletedPDF(ctx context.Context, req *port.DownloadCompletedPDFRequest) (*port.DownloadCompletedPDFResult, error) {
	envResp, err := a.fetchEnvelope(ctx, req.ProviderDocumentID)
	if err != nil {
		return nil, err
	}
	if envResp.Status != string(EnvelopeStatusCompleted) && envResp.Status != string(EnvelopeStatusSigned) {
		return nil, fmt.Errorf("documenso envelope %s is not completed", req.ProviderDocumentID)
	}
	if len(envResp.EnvelopeItems) == 0 || strings.TrimSpace(envResp.EnvelopeItems[0].ID) == "" {
		return nil, fmt.Errorf("documenso envelope %s has no downloadable item", req.ProviderDocumentID)
	}

	itemID := url.PathEscape(envResp.EnvelopeItems[0].ID)
	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		fmt.Sprintf("%s/envelope/item/%s/download?version=signed", a.config.BaseURL, itemID),
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("creating completed PDF request: %w", err)
	}
	a.setAuthHeader(httpReq)

	resp, err := a.httpClient.Do(httpReq) //nolint:gosec // URL is built from configured provider base URL
	if err != nil {
		return nil, fmt.Errorf("executing completed PDF request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("documenso API error downloading completed PDF (status %d): %s", resp.StatusCode, string(body))
	}
	pdf, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading completed PDF response: %w", err)
	}
	if len(pdf) == 0 {
		return nil, fmt.Errorf("documenso completed PDF response is empty")
	}

	return &port.DownloadCompletedPDFResult{
		PDF:         pdf,
		Filename:    "document_signed.pdf",
		ContentType: "application/pdf",
	}, nil
}

// fetchEnvelope retrieves envelope details from the Documenso API.
func (a *Adapter) fetchEnvelope(ctx context.Context, providerDocID string) (*envelopeDetailResponse, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s/envelope/%s", a.config.BaseURL, providerDocID), nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	a.setAuthHeader(httpReq)

	resp, err := a.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("documenso API error (status %d): %s", resp.StatusCode, string(body))
	}

	var envResp envelopeDetailResponse
	if err := json.NewDecoder(resp.Body).Decode(&envResp); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &envResp, nil
}

func (a *Adapter) listEnvelopePage(ctx context.Context, page, perPage int) (*envelopeListResponse, error) {
	endpoint, err := url.Parse(a.config.BaseURL + "/envelope")
	if err != nil {
		return nil, fmt.Errorf("parsing envelope list URL: %w", err)
	}
	query := endpoint.Query()
	query.Set("type", "DOCUMENT")
	query.Set("page", strconv.Itoa(page))
	query.Set("perPage", strconv.Itoa(perPage))
	query.Set("orderByColumn", "createdAt")
	query.Set("orderByDirection", "desc")
	endpoint.RawQuery = query.Encode()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("creating envelope list request: %w", err)
	}
	a.setAuthHeader(httpReq)

	resp, err := a.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("executing envelope list request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("documenso API error listing envelopes (status %d): %s", resp.StatusCode, string(body))
	}

	var listResp envelopeListResponse
	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		return nil, fmt.Errorf("decoding envelope list response: %w", err)
	}
	if listResp.Pagination.TotalPages == 0 {
		listResp.Pagination.TotalPages = 1
	}

	return &listResp, nil
}

func (a *Adapter) findEnvelopeByCorrelationKey(ctx context.Context, correlationKey string) (*envelopeDetailResponse, bool, error) {
	const perPage = 100

	for page := 1; ; page++ {
		listResp, err := a.listEnvelopePage(ctx, page, perPage)
		if err != nil {
			return nil, false, err
		}

		for _, envelope := range listResp.Data {
			if envelope.ExternalID != correlationKey {
				continue
			}

			detail, err := a.fetchEnvelope(ctx, envelope.ID)
			if err != nil {
				return nil, false, err
			}
			return detail, true, nil
		}

		if page >= listResp.Pagination.TotalPages || len(listResp.Data) == 0 {
			return nil, false, nil
		}
	}
}

func (a *Adapter) buildProviderDocumentResultFromEnvelope(env *envelopeDetailResponse, correlationKey string) *port.ProviderDocumentResult {
	result := &port.ProviderDocumentResult{
		Found:              true,
		Usable:             true,
		ProviderDocumentID: env.ID,
		ProviderName:       providerName,
		CorrelationKey:     correlationKey,
		Status:             MapEnvelopeStatus(env.Status),
		RawStatus:          env.Status,
		Recipients:         make([]port.RecipientResult, 0, len(env.Recipients)),
	}
	if result.Status == "" {
		result.Status = entity.SigningAttemptStatusSigningReady
	}

	for _, recipient := range env.Recipients {
		if strings.TrimSpace(recipient.ExternalID) == "" || strings.TrimSpace(recipient.Token) == "" {
			result.Usable = false
			result.Reason = "documenso envelope is missing recipient externalId or signing token"
			continue
		}

		result.Recipients = append(result.Recipients, port.RecipientResult{
			RoleID:               recipient.ExternalID,
			ProviderRecipientID:  strconv.Itoa(recipient.ID),
			ProviderSigningToken: recipient.Token,
			SigningURL:           fmt.Sprintf("%s/sign/%s", a.config.SigningBaseURL, recipient.Token),
			Status:               MapRecipientStatus(recipient.Status),
		})
	}

	if len(result.Recipients) == 0 {
		result.Usable = false
		if result.Reason == "" {
			result.Reason = "documenso envelope has no usable recipients"
		}
	}

	return result
}

func (a *Adapter) buildProviderSubmissionSnapshotFromEnvelope(env *envelopeDetailResponse, correlationKey string) *port.ProviderSubmissionSnapshot {
	snapshot := &port.ProviderSubmissionSnapshot{
		ProviderDocumentID: env.ID,
		ProviderName:       providerName,
		CorrelationKey:     correlationKey,
		RawStatus:          env.Status,
		HasDocument:        true,
		HasRecipients:      len(env.Recipients) > 0,
		HasFields:          countEnvelopeSignatureFields(env) > 0,
		IsDistributed:      isDistributedStatus(env.Status),
		Recipients:         a.buildRecipientResultsFromEnvelope(env, nil, true),
	}

	switch {
	case !snapshot.HasRecipients:
		snapshot.Reason = "documenso envelope has no recipients"
	case !snapshot.HasFields:
		snapshot.Reason = "documenso envelope has no fields"
	case !snapshot.IsDistributed:
		snapshot.Reason = "documenso envelope is still draft"
	case len(snapshot.Recipients) == 0:
		snapshot.Reason = "documenso envelope has no usable recipients"
	}

	return snapshot
}

func (a *Adapter) buildRecipientResultsFromEnvelope(env *envelopeDetailResponse, expected []port.SigningRecipient, requireSigningToken bool) []port.RecipientResult {
	roleByEmail := recipientRoleByEmail(expected)
	results := make([]port.RecipientResult, 0, len(env.Recipients))
	for _, recipient := range env.Recipients {
		result, ok := a.recipientResultFromEnvelope(recipient, roleByEmail, requireSigningToken)
		if ok {
			results = append(results, result)
		}
	}

	return results
}

func recipientRoleByEmail(expected []port.SigningRecipient) map[string]string {
	roleByEmail := make(map[string]string, len(expected))
	for _, recipient := range expected {
		if strings.TrimSpace(recipient.Email) == "" || strings.TrimSpace(recipient.RoleID) == "" {
			continue
		}
		roleByEmail[strings.ToLower(strings.TrimSpace(recipient.Email))] = recipient.RoleID
	}
	return roleByEmail
}

func (a *Adapter) recipientResultFromEnvelope(recipient recipientResponse, roleByEmail map[string]string, requireSigningToken bool) (port.RecipientResult, bool) {
	providerRecipientID := strconv.Itoa(recipient.ID)
	if recipient.ID == 0 {
		providerRecipientID = ""
	}
	if strings.TrimSpace(providerRecipientID) == "" {
		return port.RecipientResult{}, false
	}

	roleID := strings.TrimSpace(recipient.ExternalID)
	if roleID == "" {
		roleID = roleByEmail[strings.ToLower(strings.TrimSpace(recipient.Email))]
	}
	if roleID == "" {
		return port.RecipientResult{}, false
	}

	token := strings.TrimSpace(recipient.Token)
	if requireSigningToken && token == "" {
		return port.RecipientResult{}, false
	}

	result := port.RecipientResult{
		RoleID:               roleID,
		ProviderRecipientID:  providerRecipientID,
		ProviderSigningToken: token,
		Status:               MapRecipientStatus(recipient.Status),
	}
	if token != "" {
		result.SigningURL = fmt.Sprintf("%s/sign/%s", a.config.SigningBaseURL, token)
	}
	return result, true
}

func missingSigningRecipientRoles(env *envelopeDetailResponse, expected []port.SigningRecipient) []string {
	roleByEmail := recipientRoleByEmail(expected)
	present := make(map[string]struct{}, len(env.Recipients))
	for _, recipient := range env.Recipients {
		roleID := strings.TrimSpace(recipient.ExternalID)
		if roleID == "" {
			roleID = roleByEmail[strings.ToLower(strings.TrimSpace(recipient.Email))]
		}
		if roleID != "" {
			present[roleID] = struct{}{}
		}
	}

	missing := make([]string, 0)
	for _, recipient := range expected {
		roleID := strings.TrimSpace(recipient.RoleID)
		if roleID == "" {
			continue
		}
		if _, ok := present[roleID]; !ok {
			missing = append(missing, roleID)
		}
	}

	return missing
}

func buildMissingSignatureFieldPayloads(env *envelopeDetailResponse, signatureFields []port.SignatureFieldPosition, recipients []port.RecipientResult) []fieldPayload {
	recipientIDByRole := make(map[string]int, len(recipients))
	for _, recipient := range recipients {
		roleID := strings.TrimSpace(recipient.RoleID)
		if roleID == "" {
			continue
		}
		providerRecipientID, err := strconv.Atoi(recipient.ProviderRecipientID)
		if err != nil || providerRecipientID == 0 {
			continue
		}
		recipientIDByRole[roleID] = providerRecipientID
	}

	existingByRecipient := countExistingSignatureFieldsByRecipient(env)
	fieldPayloads := make([]fieldPayload, 0, len(signatureFields))
	for _, sf := range signatureFields {
		providerRecipientID, ok := recipientIDByRole[sf.RoleID]
		if !ok {
			continue
		}
		if existingByRecipient[providerRecipientID] > 0 {
			existingByRecipient[providerRecipientID]--
			continue
		}

		fieldPayloads = append(fieldPayloads, fieldPayload{
			RecipientID: providerRecipientID,
			Type:        "SIGNATURE",
			Page:        sf.Page,
			PositionX:   sf.PositionX,
			PositionY:   sf.PositionY,
			Width:       sf.Width,
			Height:      sf.Height,
		})
	}

	return fieldPayloads
}

func countRequiredSignatureFields(signatureFields []port.SignatureFieldPosition, recipients []port.RecipientResult) int {
	recipientIDByRole := make(map[string]struct{}, len(recipients))
	for _, recipient := range recipients {
		if strings.TrimSpace(recipient.RoleID) == "" || strings.TrimSpace(recipient.ProviderRecipientID) == "" {
			continue
		}
		recipientIDByRole[recipient.RoleID] = struct{}{}
	}

	count := 0
	for _, sf := range signatureFields {
		if _, ok := recipientIDByRole[sf.RoleID]; ok {
			count++
		}
	}

	return count
}

func countEnvelopeSignatureFields(env *envelopeDetailResponse) int {
	if env == nil {
		return 0
	}

	count := 0
	for _, field := range env.Fields {
		if isSignatureField(field) {
			count++
		}
	}
	for _, recipient := range env.Recipients {
		for _, field := range recipient.Fields {
			if isSignatureField(field) {
				count++
			}
		}
	}

	return count
}

func countExistingSignatureFieldsByRecipient(env *envelopeDetailResponse) map[int]int {
	counts := make(map[int]int)
	if env == nil {
		return counts
	}

	for _, field := range env.Fields {
		if isSignatureField(field) && field.RecipientID != 0 {
			counts[field.RecipientID]++
		}
	}
	for _, recipient := range env.Recipients {
		for _, field := range recipient.Fields {
			incrementSignatureFieldCount(counts, field, recipient.ID)
		}
	}

	return counts
}

func incrementSignatureFieldCount(counts map[int]int, field fieldResponse, fallbackRecipientID int) {
	if !isSignatureField(field) {
		return
	}
	recipientID := field.RecipientID
	if recipientID == 0 {
		recipientID = fallbackRecipientID
	}
	if recipientID != 0 {
		counts[recipientID]++
	}
}

func isSignatureField(field fieldResponse) bool {
	return strings.EqualFold(strings.TrimSpace(field.Type), "SIGNATURE")
}

func isDistributedStatus(status string) bool {
	return !strings.EqualFold(strings.TrimSpace(status), "DRAFT")
}

func validateEnvelopeCorrelation(env *envelopeDetailResponse, correlationKey string, phase entity.ProviderSubmitPhase) error {
	if env == nil {
		return nil
	}

	if env.ExternalID == correlationKey {
		return nil
	}

	return newDocumensoProviderError(
		entity.ProviderErrorClassConflictStale,
		phase,
		env.ID,
		fmt.Sprintf("documenso envelope externalId %q does not match attempt correlation key %q", env.ExternalID, correlationKey),
	)
}

func newDocumensoProviderError(class entity.ProviderErrorClass, phase entity.ProviderSubmitPhase, providerDocumentID string, message string) *port.ProviderError {
	return &port.ProviderError{
		Class:              class,
		Phase:              phase,
		ProviderName:       providerName,
		ProviderDocumentID: &providerDocumentID,
		Retryable:          false,
		SafeToResubmit:     false,
		Message:            message,
	}
}

// processRecipients converts recipient responses to status results and determines signing states.
func processRecipients(recipients []recipientResponse) ([]port.RecipientStatusResult, bool, bool) {
	results := make([]port.RecipientStatusResult, len(recipients))
	allSigned := true
	anyDeclined := false

	for i, r := range recipients {
		recipientStatus := MapRecipientStatus(r.Status)

		var signedAt *time.Time
		if r.SignedAt != "" {
			if t, err := time.Parse(time.RFC3339, r.SignedAt); err == nil {
				signedAt = &t
			}
		}
		if signedAt != nil {
			recipientStatus = entity.RecipientStatusSigned
		}

		results[i] = port.RecipientStatusResult{
			ProviderRecipientID: strconv.Itoa(r.ID),
			Status:              recipientStatus,
			SignedAt:            signedAt,
			ProviderStatus:      r.Status,
		}

		if recipientStatus != entity.RecipientStatusSigned {
			allSigned = false
		}
		if recipientStatus == entity.RecipientStatusDeclined {
			anyDeclined = true
		}
	}

	return results, allSigned, anyDeclined
}

// determineFinalStatus determines the final document status based on envelope status and recipient states.
func determineFinalStatus(envStatus string, allSigned, anyDeclined bool, recipientCount int, recipientResults []port.RecipientStatusResult) entity.SigningAttemptStatus {
	if anyDeclined {
		return entity.SigningAttemptStatusDeclined
	}

	if allSigned && recipientCount > 0 {
		return entity.SigningAttemptStatusCompleted
	}

	baseStatus := MapEnvelopeStatus(envStatus)
	if baseStatus != entity.SigningAttemptStatusSigningReady {
		return baseStatus
	}

	for _, r := range recipientResults {
		if r.Status == entity.RecipientStatusDelivered || r.Status == entity.RecipientStatusSigned {
			return entity.SigningAttemptStatusSigning
		}
	}

	return baseStatus
}

// CleanupProviderDocument cancels/deprecates a provider document by deleting the Documenso envelope.
func (a *Adapter) CleanupProviderDocument(ctx context.Context, req *port.CleanupProviderDocumentRequest) (*port.CleanupProviderDocumentResult, error) {
	if err := a.postEnvelopeAction(
		ctx,
		req.ProviderDocumentID,
		"/envelope/delete",
		"marshaling delete request",
		"creating request",
		"executing request",
		"documenso API error",
	); err != nil {
		return nil, err
	}
	return &port.CleanupProviderDocumentResult{Action: "CANCEL", Status: "SUCCEEDED"}, nil
}

func (a *Adapter) postEnvelopeAction(
	ctx context.Context,
	envelopeID string,
	endpoint string,
	marshalErrMsg string,
	createReqErrMsg string,
	execReqErrMsg string,
	apiErrMsg string,
) error {
	reqBody, err := json.Marshal(map[string]string{
		"envelopeId": envelopeID,
	})
	if err != nil {
		return fmt.Errorf("%s: %w", marshalErrMsg, err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.config.BaseURL+endpoint, bytes.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("%s: %w", createReqErrMsg, err)
	}

	a.setAuthHeader(httpReq)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := a.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("%s: %w", execReqErrMsg, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%s (status %d): %s", apiErrMsg, resp.StatusCode, string(respBody))
	}

	return nil
}

// ParseWebhook parses and validates an incoming webhook request.
func (a *Adapter) ParseWebhook(ctx context.Context, req *port.ParseWebhookRequest) (*port.WebhookEvent, error) {
	if a.config.WebhookSecret != "" {
		if !a.validateSignature(req.Body, req.Signature) {
			return nil, entity.ErrInvalidWebhookSignature
		}
	}

	var payload webhookPayload
	if err := json.Unmarshal(req.Body, &payload); err != nil {
		return nil, fmt.Errorf("parsing webhook payload: %w", err)
	}

	document := payload.Document()
	correlationKey := ""
	if document.ExternalID != nil {
		correlationKey = *document.ExternalID
	}

	event := &port.WebhookEvent{
		EventType:              payload.Event,
		ProviderName:           providerName,
		ProviderCorrelationKey: correlationKey,
		Timestamp:              time.Now(),
		RawPayload:             req.Body,
	}

	// Map the event type to status changes
	mapping := MapWebhookEvent(payload.Event)
	event.DocumentStatus = mapping.DocumentStatus
	event.RecipientStatus = mapping.RecipientStatus

	// Extract the recipient who signed from the webhook payload
	for _, r := range document.Recipients {
		if r.SigningStatus == "SIGNED" && r.SignedAt != nil {
			event.ProviderRecipientID = strconv.Itoa(r.ID)
			break
		}
	}

	return event, nil
}

// validateSignature validates the webhook secret.
// Documenso sends the raw secret in the X-Documenso-Secret header (not HMAC).
func (a *Adapter) validateSignature(_ []byte, signature string) bool {
	if signature == "" {
		return false
	}

	return subtle.ConstantTimeCompare([]byte(signature), []byte(a.config.WebhookSecret)) == 1
}

// Ensure Adapter implements the interfaces
var (
	_ port.SigningProvider = (*Adapter)(nil)
	_ port.WebhookHandler  = (*Adapter)(nil)
)

// API request/response types

type envelopeResponse struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

type recipientPayload struct {
	Email        string `json:"email"`
	Name         string `json:"name"`
	Role         string `json:"role"`
	SigningOrder int    `json:"signingOrder,omitempty"`
	ExternalID   string `json:"externalId,omitempty"`
}

type recipientsRequest struct {
	EnvelopeID string             `json:"envelopeId"`
	Data       []recipientPayload `json:"data"`
}

type recipientResponse struct {
	ID         int             `json:"id"`
	Email      string          `json:"email"`
	Name       string          `json:"name"`
	Status     string          `json:"status"`
	Token      string          `json:"token,omitempty"`
	SignedAt   string          `json:"signedAt,omitempty"`
	ExternalID string          `json:"externalId,omitempty"`
	Fields     []fieldResponse `json:"fields,omitempty"`
}

type recipientsResponse struct {
	Data []recipientData `json:"data"`
}

type recipientData struct {
	ID         int    `json:"id"`
	EnvelopeID string `json:"envelopeId"`
	Email      string `json:"email"`
	Name       string `json:"name"`
	Token      string `json:"token"`
}

type envelopeDetailResponse struct {
	ID                   string              `json:"id"`
	ExternalID           string              `json:"externalId"`
	Status               string              `json:"status"`
	Title                string              `json:"title"`
	Recipients           []recipientResponse `json:"recipients"`
	Fields               []fieldResponse     `json:"fields,omitempty"`
	EnvelopeItems        []envelopeItem      `json:"envelopeItems,omitempty"`
	CompletedDocumentURL string              `json:"completedDocumentUrl,omitempty"`
	CreatedAt            string              `json:"createdAt"`
	UpdatedAt            string              `json:"updatedAt"`
}

type envelopeItem struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Order int    `json:"order"`
}

type envelopeListResponse struct {
	Data       []envelopeSummary `json:"data"`
	Pagination pagination        `json:"pagination"`
}

type envelopeSummary struct {
	ID         string `json:"id"`
	ExternalID string `json:"externalId"`
	Status     string `json:"status"`
}

type pagination struct {
	Page       int `json:"page"`
	PerPage    int `json:"perPage"`
	TotalPages int `json:"totalPages"`
	TotalItems int `json:"totalItems"`
}

type webhookPayload struct {
	Event   string          `json:"event"`
	Payload webhookDocument `json:"payload"`
	Data    webhookDocument `json:"data"`
}

func (p webhookPayload) Document() webhookDocument {
	if p.Payload.ExternalID != nil || p.Payload.ID != 0 || len(p.Payload.Recipients) > 0 {
		return p.Payload
	}
	return p.Data
}

type webhookDocument struct {
	ID         int                `json:"id"`
	ExternalID *string            `json:"externalId"`
	Title      string             `json:"title"`
	Status     string             `json:"status"`
	Recipients []webhookRecipient `json:"recipients"`
}

type webhookRecipient struct {
	ID            int     `json:"id"`
	Name          string  `json:"name"`
	Email         string  `json:"email"`
	SigningStatus string  `json:"signingStatus"`
	SignedAt      *string `json:"signedAt"`
}

// Field creation types for Documenso API

type fieldPayload struct {
	RecipientID int     `json:"recipientId"`
	Type        string  `json:"type"` // "SIGNATURE", "TEXT", "DATE", etc.
	Page        int     `json:"page"` // 1-indexed page number
	PositionX   float64 `json:"positionX"`
	PositionY   float64 `json:"positionY"`
	Width       float64 `json:"width"`
	Height      float64 `json:"height"`
}

type fieldsRequest struct {
	EnvelopeID string         `json:"envelopeId"`
	Data       []fieldPayload `json:"data"`
}

type fieldResponse struct {
	ID          int    `json:"id"`
	RecipientID int    `json:"recipientId"`
	Type        string `json:"type"`
}
