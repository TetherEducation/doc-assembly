package document

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	documentuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/document"
)

func signingEmailCreateCommand() documentuc.CreateDocumentCommand {
	return documentuc.CreateDocumentCommand{WorkspaceID: "ws-1", TemplateVersionID: "ver-1", Title: "Anexo"}
}

func TestNewDocument_StoresTheSigningEmail(t *testing.T) {
	cmd := signingEmailCreateCommand()
	cmd.SigningEmail = &entity.SigningEmail{Subject: " Anexo de Renovación ", Message: "Para su firma"}

	doc, err := newDocument(cmd, "type-1")

	require.NoError(t, err)
	assert.Equal(t, entity.SigningEmail{Subject: "Anexo de Renovación", Message: "Para su firma"}, doc.SigningEmail())
}

// Without an email the document is built exactly as before: no metadata.
func TestNewDocument_NoSigningEmailLeavesMetadataEmpty(t *testing.T) {
	for _, email := range []*entity.SigningEmail{nil, {}} {
		cmd := signingEmailCreateCommand()
		cmd.SigningEmail = email

		doc, err := newDocument(cmd, "type-1")

		require.NoError(t, err)
		assert.Nil(t, doc.Metadata)
	}
}

func TestNewDocument_RejectsATooLongSigningEmail(t *testing.T) {
	cmd := signingEmailCreateCommand()
	cmd.SigningEmail = &entity.SigningEmail{Message: strings.Repeat("x", entity.MaxSigningEmailMessageLength+1)}

	_, err := newDocument(cmd, "type-1")

	require.ErrorIs(t, err, entity.ErrFieldTooLong)
}
