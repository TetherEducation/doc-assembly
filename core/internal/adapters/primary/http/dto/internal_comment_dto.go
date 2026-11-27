package dto

import "time"

// InternalAddCommentRequest adds a CRM-only comment to the template a version
// belongs to. The template is derived from the version, never taken from the body.
type InternalAddCommentRequest struct {
	// Audience the comment is listed within, computed by crm-admission.
	Audience string `json:"audience"`

	// Body is trimmed, must not be empty and is at most 4000 characters.
	Body string `json:"body"`

	// Who wrote it, and from which campus.
	Email  string `json:"email"`
	Name   string `json:"name"`
	Campus string `json:"campus"`
}

// InternalCommentResponse is one template comment.
type InternalCommentResponse struct {
	ID                string    `json:"id"`
	TemplateID        string    `json:"templateId"`
	TemplateVersionID string    `json:"templateVersionId"`
	VersionNumber     int       `json:"versionNumber"`
	Audience          string    `json:"audience"`
	Body              string    `json:"body"`
	AuthorEmail       string    `json:"authorEmail"`
	AuthorName        *string   `json:"authorName"`
	AuthorCampus      string    `json:"authorCampus"`
	CreatedAt         time.Time `json:"createdAt"`
}

// InternalCommentListResponse lists one audience's comments, oldest first.
type InternalCommentListResponse struct {
	Items []InternalCommentResponse `json:"items"`
}
