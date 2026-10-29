package document

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	catalogcuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/catalog"
	documentuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/document"
	templateuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/template"
)

// pdfSendFakes records every step PDFSendService takes, in order.
type pdfSendFakes struct {
	steps []string

	createTemplate templateuc.CreateTemplateCommand
	storedPDF      []byte
	placements     templateuc.ReplacePlacementsCommand
	assigned       templateuc.AssignDocumentTypeCommand
	publishedBy    string
	document       documentuc.CreateDocumentCommand
	deleted        []string

	existingType *entity.DocumentType
	createType   func() (*entity.DocumentType, error)
	failAt       string
}

func (f *pdfSendFakes) step(name string) error {
	f.steps = append(f.steps, name)
	if f.failAt == name {
		return fmt.Errorf("%s failed", name)
	}
	return nil
}

func (f *pdfSendFakes) CreateTemplate(_ context.Context, cmd templateuc.CreateTemplateCommand) (*entity.Template, *entity.TemplateVersion, error) {
	f.createTemplate = cmd
	if err := f.step("create template"); err != nil {
		return nil, nil, err
	}
	return &entity.Template{ID: "tpl-1"}, &entity.TemplateVersion{ID: "ver-1"}, nil
}

func (f *pdfSendFakes) AssignDocumentType(_ context.Context, cmd templateuc.AssignDocumentTypeCommand) (*templateuc.AssignDocumentTypeResult, error) {
	f.assigned = cmd
	if err := f.step("assign type"); err != nil {
		return nil, err
	}
	return &templateuc.AssignDocumentTypeResult{Template: &entity.Template{ID: cmd.TemplateID}}, nil
}

func (f *pdfSendFakes) DeleteTemplate(_ context.Context, id string) error {
	f.deleted = append(f.deleted, id)
	return nil
}

func (f *pdfSendFakes) PublishVersion(_ context.Context, _ string, userID string) error {
	f.publishedBy = userID
	return f.step("publish")
}

func (f *pdfSendFakes) StorePDF(_ context.Context, cmd templateuc.StoreUploadedPDFCommand) (*templateuc.StoreUploadedPDFResult, error) {
	f.storedPDF = cmd.PDF
	if err := f.step("store PDF"); err != nil {
		return nil, err
	}
	return &templateuc.StoreUploadedPDFResult{PageCount: 1}, nil
}

func (f *pdfSendFakes) ReplacePlacements(_ context.Context, cmd templateuc.ReplacePlacementsCommand) (*templateuc.ReplacePlacementsResult, error) {
	f.placements = cmd
	if err := f.step("place boxes"); err != nil {
		return nil, err
	}
	result := &templateuc.ReplacePlacementsResult{}
	for _, signer := range cmd.Signers {
		result.Signers = append(result.Signers, templateuc.PlacedSigner{RoleID: fmt.Sprintf("role-%d", signer.Order), Name: signer.Name, Order: signer.Order})
	}
	return result, nil
}

func (f *pdfSendFakes) FindByCode(_ context.Context, tenantID, code string) (*entity.DocumentType, error) {
	f.steps = append(f.steps, "find type")
	if f.existingType != nil && f.existingType.TenantID == tenantID && f.existingType.Code == code {
		return f.existingType, nil
	}
	return nil, entity.ErrDocumentTypeNotFound
}

func (f *pdfSendFakes) CreateDocumentType(_ context.Context, cmd catalogcuc.CreateDocumentTypeCommand) (*entity.DocumentType, error) {
	f.steps = append(f.steps, "create type")
	if f.createType != nil {
		return f.createType()
	}
	created := &entity.DocumentType{ID: "type-new", TenantID: cmd.TenantID, Code: cmd.Code}
	f.existingType = created
	return created, nil
}

func (f *pdfSendFakes) FindByID(context.Context, string) (*entity.Workspace, error) {
	tenant := "tenant-1"
	return &entity.Workspace{ID: "ws-1", TenantID: &tenant}, nil
}

func (f *pdfSendFakes) CreateAndSendDocument(_ context.Context, cmd documentuc.CreateDocumentCommand) (*entity.DocumentWithRecipients, error) {
	f.document = cmd
	if err := f.step("create document"); err != nil {
		return nil, err
	}
	return &entity.DocumentWithRecipients{Document: entity.Document{ID: "doc-1"}}, nil
}

func newPDFSendFixture() (*PDFSendService, *pdfSendFakes) {
	f := &pdfSendFakes{}
	return NewPDFSendService(f, f, f, f, f, f, f), f
}

func sendPDFCommand() documentuc.SendPDFCommand {
	sig := func(y float64) entity.PlacedField {
		return entity.PlacedField{Type: "SIGNATURE", Page: 1, X: 72, Y: y, Width: 200, Height: 50}
	}
	return documentuc.SendPDFCommand{
		WorkspaceID: "ws-1",
		UserID:      "user-1",
		Title:       "  Mutual NDA  ",
		PDF:         []byte("%PDF-1.4 …"),
		Signers: []documentuc.PDFSigner{
			{Name: " Ana ", Email: "ana@example.test", Order: 1, Fields: []entity.PlacedField{sig(100)}},
			{Name: "Ben", Email: " ben@example.test ", Order: 2, Fields: []entity.PlacedField{sig(200), {Type: "DATE", Page: 1, X: 300, Y: 200, Width: 100, Height: 20}}},
		},
	}
}

func TestSendPDF_RunsEveryStepAndSends(t *testing.T) {
	svc, f := newPDFSendFixture()

	doc, err := svc.SendPDF(context.Background(), sendPDFCommand())

	require.NoError(t, err)
	assert.Equal(t, "doc-1", doc.ID)
	assert.Equal(t, []string{
		"find type", "create type", "create template", "store PDF", "place boxes", "assign type", "publish", "create document",
	}, f.steps)

	// A one-off template with a process of its own, so it can share the type.
	assert.Equal(t, "ws-1", f.createTemplate.WorkspaceID)
	assert.True(t, strings.HasPrefix(f.createTemplate.Title, "Mutual NDA · PDF "), f.createTemplate.Title)
	assert.True(t, strings.HasPrefix(f.createTemplate.Process, "pdf-send-"), f.createTemplate.Process)
	assert.Equal(t, string(entity.ProcessTypeID), f.createTemplate.ProcessType)
	assert.Equal(t, "user-1", f.createTemplate.CreatedBy)

	assert.Equal(t, []byte("%PDF-1.4 …"), f.storedPDF)
	require.Len(t, f.placements.Signers, 2)
	assert.Equal(t, "Ana", f.placements.Signers[0].Name)
	assert.Len(t, f.placements.Signers[1].Fields, 2)

	require.NotNil(t, f.assigned.DocumentTypeID)
	assert.Equal(t, "type-new", *f.assigned.DocumentTypeID)
	assert.Equal(t, "user-1", f.publishedBy)

	// One recipient per signer, on the role placed for them, in order.
	assert.Equal(t, "Mutual NDA", f.document.Title)
	assert.Equal(t, "ver-1", f.document.TemplateVersionID)
	assert.Equal(t, []documentuc.DocumentRecipientCommand{
		{RoleID: "role-1", Name: "Ana", Email: "ana@example.test"},
		{RoleID: "role-2", Name: "Ben", Email: "ben@example.test"},
	}, f.document.Recipients)
	assert.Empty(t, f.deleted)
}

func TestSendPDF_ReusesTheTenantsUploadedPDFType(t *testing.T) {
	svc, f := newPDFSendFixture()
	f.existingType = &entity.DocumentType{ID: "type-existing", TenantID: "tenant-1", Code: UploadedPDFDocumentTypeCode}

	_, err := svc.SendPDF(context.Background(), sendPDFCommand())

	require.NoError(t, err)
	assert.NotContains(t, f.steps, "create type")
	assert.Equal(t, "type-existing", *f.assigned.DocumentTypeID)
}

func TestSendPDF_TypeCreatedConcurrently_ReadsItBack(t *testing.T) {
	svc, f := newPDFSendFixture()
	f.createType = func() (*entity.DocumentType, error) {
		// Another first send created it between the lookup and the create.
		f.existingType = &entity.DocumentType{ID: "type-other", TenantID: "tenant-1", Code: UploadedPDFDocumentTypeCode}
		return nil, entity.ErrDocumentTypeCodeExists
	}

	_, err := svc.SendPDF(context.Background(), sendPDFCommand())

	require.NoError(t, err)
	assert.Equal(t, "type-other", *f.assigned.DocumentTypeID)
}

func TestSendPDF_EachSendGetsItsOwnProcess(t *testing.T) {
	svc, f := newPDFSendFixture()
	_, err := svc.SendPDF(context.Background(), sendPDFCommand())
	require.NoError(t, err)
	first := f.createTemplate

	_, err = svc.SendPDF(context.Background(), sendPDFCommand())
	require.NoError(t, err)

	assert.NotEqual(t, first.Process, f.createTemplate.Process)
	assert.NotEqual(t, first.Title, f.createTemplate.Title)
}

func TestSendPDF_RejectsBadInputBeforeCreatingAnything(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*documentuc.SendPDFCommand)
		want   error
	}{
		{"no title", func(c *documentuc.SendPDFCommand) { c.Title = "   " }, entity.ErrInvalidPlacedField},
		{"title too long", func(c *documentuc.SendPDFCommand) { c.Title = strings.Repeat("x", 201) }, entity.ErrInvalidPlacedField},
		{"no PDF", func(c *documentuc.SendPDFCommand) { c.PDF = nil }, entity.ErrInvalidPDF},
		{"no signers", func(c *documentuc.SendPDFCommand) { c.Signers = nil }, entity.ErrInvalidPlacedField},
		{"signer without a name", func(c *documentuc.SendPDFCommand) { c.Signers[0].Name = " " }, entity.ErrInvalidPlacedField},
		{"signer without an email", func(c *documentuc.SendPDFCommand) { c.Signers[1].Email = "" }, entity.ErrInvalidPlacedField},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, f := newPDFSendFixture()
			cmd := sendPDFCommand()
			tt.mutate(&cmd)

			_, err := svc.SendPDF(context.Background(), cmd)

			require.ErrorIs(t, err, tt.want)
			assert.Empty(t, f.steps, "nothing may be created for a request that fails validation")
		})
	}
}

func TestSendPDF_FailureAfterTheTemplateExists_DeletesIt(t *testing.T) {
	for _, failAt := range []string{"store PDF", "place boxes", "assign type", "publish", "create document"} {
		t.Run(failAt, func(t *testing.T) {
			svc, f := newPDFSendFixture()
			f.failAt = failAt

			doc, err := svc.SendPDF(context.Background(), sendPDFCommand())

			require.Error(t, err)
			assert.Nil(t, doc)
			assert.Equal(t, []string{"tpl-1"}, f.deleted)
			assert.Equal(t, failAt, f.steps[len(f.steps)-1], "no step may run after the one that failed")
		})
	}
}

func TestSendPDF_TemplateCreationFails_NothingToDelete(t *testing.T) {
	svc, f := newPDFSendFixture()
	f.failAt = "create template"

	_, err := svc.SendPDF(context.Background(), sendPDFCommand())

	require.Error(t, err)
	assert.Empty(t, f.deleted)
	assert.False(t, errors.Is(err, entity.ErrInvalidPlacedField))
}
