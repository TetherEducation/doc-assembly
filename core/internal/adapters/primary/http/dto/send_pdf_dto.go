package dto

// SendPDFRequest is the JSON "payload" part of POST /documents/from-pdf; the
// PDF itself is the "file" part.
type SendPDFRequest struct {
	Title        string               `json:"title"`
	Signers      []SendPDFSigner      `json:"signers"`
	SigningEmail *SigningEmailRequest `json:"signingEmail,omitempty"`
}

// SendPDFSigner is one signer, their place in the signing order and their boxes.
type SendPDFSigner struct {
	Name   string                     `json:"name"`
	Email  string                     `json:"email"`
	Order  int                        `json:"order"`
	Fields []AutomationFieldPlacement `json:"fields"`
}
