package entity

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// TemplateCommentMaxLength is the longest comment body accepted, in characters.
// Matches the template_comments_body_valid check constraint.
const TemplateCommentMaxLength = 4000

// ErrTemplateCommentInvalid is returned when a comment is missing a required field
// or its body is empty or too long. Wrapped with the specific reason.
var ErrTemplateCommentInvalid = errors.New("invalid template comment")

// TemplateComment is a school's remark on a contract template, shown only in the CRM.
//
// It hangs off the template so a thread survives new versions, and records the
// version it was written against. Comments are append-only: never edited or deleted.
type TemplateComment struct {
	ID                string
	TemplateID        string
	TemplateVersionID string

	// VersionNumber of TemplateVersionID. Read from template_versions, not stored.
	VersionNumber int

	// Audience is the boundary the comment is listed within. Computed by the caller
	// (crm-admission), never by the browser.
	Audience string
	Body     string

	AuthorEmail  string
	AuthorName   *string
	AuthorCampus string

	CreatedAt time.Time
}

// NewTemplateCommentInput carries what a new comment is made of.
type NewTemplateCommentInput struct {
	TemplateID        string
	TemplateVersionID string
	Audience          string
	Body              string
	AuthorEmail       string
	AuthorName        string
	AuthorCampus      string
}

// NewTemplateComment validates and trims a new comment.
//
// The body is stored trimmed, must not be empty and must be at most
// TemplateCommentMaxLength characters. Audience, author email and author campus
// are required; an empty author name is stored as nil.
func NewTemplateComment(in NewTemplateCommentInput) (*TemplateComment, error) {
	audience := strings.TrimSpace(in.Audience)
	body := strings.TrimSpace(in.Body)
	email := strings.TrimSpace(in.AuthorEmail)
	campus := strings.TrimSpace(in.AuthorCampus)

	switch {
	case audience == "":
		return nil, fmt.Errorf("%w: audience is required", ErrTemplateCommentInvalid)
	case body == "":
		return nil, fmt.Errorf("%w: body is required", ErrTemplateCommentInvalid)
	case utf8.RuneCountInString(body) > TemplateCommentMaxLength:
		return nil, fmt.Errorf("%w: body exceeds %d characters", ErrTemplateCommentInvalid, TemplateCommentMaxLength)
	case email == "":
		return nil, fmt.Errorf("%w: email is required", ErrTemplateCommentInvalid)
	case campus == "":
		return nil, fmt.Errorf("%w: campus is required", ErrTemplateCommentInvalid)
	}

	comment := &TemplateComment{
		TemplateID:        in.TemplateID,
		TemplateVersionID: in.TemplateVersionID,
		Audience:          audience,
		Body:              body,
		AuthorEmail:       email,
		AuthorCampus:      campus,
	}
	if name := strings.TrimSpace(in.AuthorName); name != "" {
		comment.AuthorName = &name
	}
	return comment, nil
}
