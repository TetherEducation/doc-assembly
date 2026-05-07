package document

import "context"

// SigningSessionUseCase defines the input port for authenticated signing
// session creation used by embedded CRM flows.
type SigningSessionUseCase interface {
	// CreateOrGetSession returns a reusable tokenized session URL for a
	// recipient authenticated by JWT/custom auth middleware.
	CreateOrGetSession(ctx context.Context, documentID string, principal *SigningSessionPrincipal) (*SigningSessionResponse, error)
}

// SigningSessionPrincipal describes the authenticated caller identity.
type SigningSessionPrincipal struct {
	Email    string         `json:"email"`
	Subject  string         `json:"subject,omitempty"`
	Provider string         `json:"provider,omitempty"`
	Language string         `json:"language,omitempty"`
	Extra    map[string]any `json:"extra,omitempty"`
}

// SigningSessionResponse contains session URL and summarized signing state.
type SigningSessionResponse struct {
	SessionURL  string `json:"sessionUrl"`
	Step        string `json:"step"`
	CanSign     bool   `json:"canSign"`
	CanDownload bool   `json:"canDownload"`
	DownloadURL string `json:"downloadUrl,omitempty"`
}
