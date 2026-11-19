package document

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity/portabledoc"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
	documentuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/document"
)

// A signer with no questions of their own must not reach the PDF preview, nor
// start the signing attempt, while another signer's required questions are
// unanswered.
func TestPreSigningAnswers_SignerWithoutQuestionsWaitsForRequiredAnswers(t *testing.T) {
	env := newAnswersEnv(answersContent(answersField("q1", answersGuardianRole, true)), entity.TokenTypePreSigning)
	ctx := context.Background()

	page, err := env.svc.GetPublicSigningPage(ctx, "token-school")
	require.NoError(t, err)
	require.Equal(t, documentuc.StepWaiting, page.Step)
	require.True(t, page.WaitingForPrevious)
	require.Equal(t, 2, page.SigningPosition)
	require.Equal(t, 2, page.TotalSigners)
	require.Nil(t, page.Form)
	require.Empty(t, page.PdfURL)

	page, err = env.svc.ProceedToSigning(ctx, "token-school")
	require.NoError(t, err)
	require.Equal(t, documentuc.StepWaiting, page.Step)
	require.Zero(t, env.uow.calls, "no signing attempt may start before the required answers")

	page, err = env.svc.GetPublicSigningPage(ctx, "token-guardian")
	require.NoError(t, err)
	require.Equal(t, documentuc.StepPreview, page.Step)
	require.NotNil(t, page.Form)
	require.Equal(t, answersGuardianRole, page.Form.RoleID)
	require.Len(t, page.Form.Fields, 1)

	page, err = env.svc.ProceedToSigning(ctx, "token-guardian")
	require.NoError(t, err)
	require.NotNil(t, page.Form, "proceeding before answering returns the signer's questions")
	require.Zero(t, env.uow.calls)
}

func TestPreSigningAnswers_AnsweredQuestionsLetEitherSignerStart(t *testing.T) {
	env := newAnswersEnv(answersContent(answersField("q1", answersGuardianRole, true)), entity.TokenTypePreSigning)
	env.answer("q1")
	ctx := context.Background()

	for _, token := range []string{"token-guardian", "token-school"} {
		page, err := env.svc.GetPublicSigningPage(ctx, token)
		require.NoError(t, err, token)
		require.Equal(t, documentuc.StepPreview, page.Step, token)
		require.Nil(t, page.Form, token)
		require.NotEmpty(t, page.PdfURL, token)
	}

	page, err := env.svc.ProceedToSigning(ctx, "token-school")
	require.NoError(t, err)
	require.Equal(t, documentuc.StepProcessing, page.Step)
	require.Equal(t, 1, env.uow.calls)
}

// Only required questions hold the other signers; optional ones stay optional.
func TestPreSigningAnswers_OptionalQuestionsDoNotHoldOtherSigners(t *testing.T) {
	env := newAnswersEnv(answersContent(answersField("q1", answersGuardianRole, false)), entity.TokenTypePreSigning)
	ctx := context.Background()

	page, err := env.svc.GetPublicSigningPage(ctx, "token-school")
	require.NoError(t, err)
	require.Equal(t, documentuc.StepPreview, page.Step)
	require.Nil(t, page.Form)

	page, err = env.svc.ProceedToSigning(ctx, "token-school")
	require.NoError(t, err)
	require.Equal(t, documentuc.StepProcessing, page.Step)
	require.Equal(t, 1, env.uow.calls)
}

// When both signers have questions, each is shown their own until they answer,
// and signing starts only once both have.
func TestPreSigningAnswers_EachSignerAnswersTheirOwnQuestions(t *testing.T) {
	env := newAnswersEnv(answersContent(
		answersField("q1", answersGuardianRole, true),
		answersField("q2", answersSchoolRole, true),
	), entity.TokenTypePreSigning)
	env.answer("q1")
	ctx := context.Background()

	page, err := env.svc.GetPublicSigningPage(ctx, "token-guardian")
	require.NoError(t, err)
	require.Equal(t, documentuc.StepWaiting, page.Step)
	require.Equal(t, 1, page.SigningPosition)

	page, err = env.svc.GetPublicSigningPage(ctx, "token-school")
	require.NoError(t, err)
	require.Equal(t, documentuc.StepPreview, page.Step)
	require.NotNil(t, page.Form)
	require.Equal(t, answersSchoolRole, page.Form.RoleID)
	require.Len(t, page.Form.Fields, 1)
	require.Equal(t, "q2", page.Form.Fields[0].ID)

	page, err = env.svc.ProceedToSigning(ctx, "token-guardian")
	require.NoError(t, err)
	require.Equal(t, documentuc.StepWaiting, page.Step)
	require.Zero(t, env.uow.calls)

	env.answer("q2")
	page, err = env.svc.ProceedToSigning(ctx, "token-guardian")
	require.NoError(t, err)
	require.Equal(t, documentuc.StepProcessing, page.Step)
	require.Equal(t, 1, env.uow.calls)
}

func TestPreSigningAnswers_DocumentWithoutQuestionsStartsSigning(t *testing.T) {
	env := newAnswersEnv(answersContent(), entity.TokenTypeSigning)

	page, err := env.svc.ProceedToSigning(context.Background(), "token-school")
	require.NoError(t, err)
	require.Equal(t, documentuc.StepProcessing, page.Step)
	require.Equal(t, 1, env.uow.calls)
}

// --- fixtures ---

const (
	answersGuardianRole = "role-guardian"
	answersSchoolRole   = "role-school"
)

type answersEnv struct {
	svc       *PreSigningService
	uow       *answersUOW
	responses *answersFieldResponseRepo
}

// newAnswersEnv builds a document awaiting input with two signers: the guardian
// signs first, the school second.
func newAnswersEnv(content json.RawMessage, tokenType string) *answersEnv {
	doc := &entity.Document{ID: "doc-1", TemplateVersionID: "version-1", Status: entity.DocumentStatusAwaitingInput}
	recipients := []*entity.DocumentRecipient{
		{ID: "recipient-guardian", DocumentID: doc.ID, TemplateVersionRoleID: "db-guardian", Name: "Guardian", Email: "guardian@example.com", Status: entity.RecipientStatusPending},
		{ID: "recipient-school", DocumentID: doc.ID, TemplateVersionRoleID: "db-school", Name: "School", Email: "school@example.com", Status: entity.RecipientStatusPending},
	}
	roles := []*entity.TemplateVersionSignerRole{
		{ID: "db-guardian", TemplateVersionID: doc.TemplateVersionID, RoleName: "Apoderado", AnchorString: portabledoc.GenerateAnchorString("Apoderado"), SignerOrder: 1},
		{ID: "db-school", TemplateVersionID: doc.TemplateVersionID, RoleName: "Colegio", AnchorString: portabledoc.GenerateAnchorString("Colegio"), SignerOrder: 2},
	}
	expires := time.Now().Add(time.Hour)
	tokens := map[string]*entity.DocumentAccessToken{
		"token-guardian": {ID: "access-guardian", DocumentID: doc.ID, RecipientID: "recipient-guardian", Token: "token-guardian", TokenType: tokenType, ExpiresAt: expires},
		"token-school":   {ID: "access-school", DocumentID: doc.ID, RecipientID: "recipient-school", Token: "token-school", TokenType: tokenType, ExpiresAt: expires},
	}
	responses := &answersFieldResponseRepo{}
	uow := &answersUOW{doc: doc}
	svc := &PreSigningService{
		accessTokenRepo:   &answersTokenRepo{tokens: tokens},
		fieldResponseRepo: responses,
		documentRepo:      &answersDocumentRepo{doc: doc},
		recipientRepo:     &answersRecipientRepo{recipients: recipients},
		attemptRepo:       &answersAttemptRepo{uow: uow},
		signingUOW:        uow,
		versionRepo:       &answersVersionRepo{version: &entity.TemplateVersion{ID: doc.TemplateVersionID, ContentStructure: content}},
		signerRoleRepo:    &answersSignerRoleRepo{roles: roles},
	}
	return &answersEnv{svc: svc, uow: uow, responses: responses}
}

func (e *answersEnv) answer(fieldIDs ...string) {
	for _, id := range fieldIDs {
		e.responses.responses = append(e.responses.responses, entity.DocumentFieldResponse{
			ID:         "response-" + id,
			DocumentID: "doc-1",
			FieldID:    id,
			FieldType:  portabledoc.InteractiveFieldTypeCheckbox,
			Response:   json.RawMessage(`{"selectedOptionIds":["` + id + `-yes"]}`),
		})
	}
}

func answersField(id, roleID string, required bool) map[string]any {
	return map[string]any{
		"id":        id,
		"fieldType": portabledoc.InteractiveFieldTypeCheckbox,
		"roleId":    roleID,
		"label":     "Question " + id,
		"required":  required,
		"options": []any{
			map[string]any{"id": id + "-yes", "label": "Yes"},
			map[string]any{"id": id + "-no", "label": "No"},
		},
	}
}

func answersContent(fields ...map[string]any) json.RawMessage {
	nodes := []any{map[string]any{
		"type":    "paragraph",
		"content": []any{map[string]any{"type": "text", "text": "Agreement"}},
	}}
	for _, field := range fields {
		nodes = append(nodes, map[string]any{"type": "interactiveField", "attrs": field})
	}
	nodes = append(nodes, map[string]any{
		"type": "signature",
		"attrs": map[string]any{
			"count": 2, "layout": "dual-sides", "lineWidth": "md",
			"signatures": []any{
				map[string]any{"id": "signature-guardian", "roleId": answersGuardianRole, "label": "Apoderado"},
				map[string]any{"id": "signature-school", "roleId": answersSchoolRole, "label": "Colegio"},
			},
		},
	})
	raw, err := json.Marshal(map[string]any{
		"version":     "1.1.0",
		"meta":        map[string]any{"title": "Agreement", "language": "es"},
		"variableIds": []any{},
		"signerRoles": []any{
			map[string]any{"id": answersGuardianRole, "label": "Apoderado", "order": 1,
				"name": map[string]any{"type": "text", "value": "Guardian"}, "email": map[string]any{"type": "text", "value": "guardian@example.com"}},
			map[string]any{"id": answersSchoolRole, "label": "Colegio", "order": 2,
				"name": map[string]any{"type": "text", "value": "School"}, "email": map[string]any{"type": "text", "value": "school@example.com"}},
		},
		"content": map[string]any{"type": "doc", "content": nodes},
	})
	if err != nil {
		panic(err)
	}
	return raw
}

type answersTokenRepo struct {
	port.DocumentAccessTokenRepository
	tokens map[string]*entity.DocumentAccessToken
}

func (r *answersTokenRepo) FindByToken(_ context.Context, token string) (*entity.DocumentAccessToken, error) {
	found, ok := r.tokens[token]
	if !ok {
		return nil, entity.ErrRecordNotFound
	}
	clone := *found
	return &clone, nil
}

type answersDocumentRepo struct {
	port.DocumentRepository
	doc *entity.Document
}

func (r *answersDocumentRepo) FindByID(_ context.Context, _ string) (*entity.Document, error) {
	clone := *r.doc
	return &clone, nil
}

type answersRecipientRepo struct {
	port.DocumentRecipientRepository
	recipients []*entity.DocumentRecipient
}

func (r *answersRecipientRepo) FindByID(_ context.Context, id string) (*entity.DocumentRecipient, error) {
	for _, recipient := range r.recipients {
		if recipient.ID == id {
			clone := *recipient
			return &clone, nil
		}
	}
	return nil, entity.ErrRecordNotFound
}

func (r *answersRecipientRepo) FindByDocumentID(_ context.Context, _ string) ([]*entity.DocumentRecipient, error) {
	return r.recipients, nil
}

type answersVersionRepo struct {
	port.TemplateVersionRepository
	version *entity.TemplateVersion
}

func (r *answersVersionRepo) FindByID(_ context.Context, _ string) (*entity.TemplateVersion, error) {
	return r.version, nil
}

type answersSignerRoleRepo struct {
	port.TemplateVersionSignerRoleRepository
	roles []*entity.TemplateVersionSignerRole
}

func (r *answersSignerRoleRepo) FindByVersionID(_ context.Context, _ string) ([]*entity.TemplateVersionSignerRole, error) {
	return r.roles, nil
}

type answersFieldResponseRepo struct {
	port.DocumentFieldResponseRepository
	responses []entity.DocumentFieldResponse
}

func (r *answersFieldResponseRepo) FindByDocumentID(_ context.Context, _ string) ([]entity.DocumentFieldResponse, error) {
	return r.responses, nil
}

// answersUOW records attempt creation and points the document at the new attempt.
type answersUOW struct {
	port.SigningExecutionUnitOfWork
	doc     *entity.Document
	attempt *entity.SigningAttempt
	calls   int
}

func (u *answersUOW) CreateAttemptAndEnqueueRender(
	_ context.Context,
	documentID string,
	_ []*entity.DocumentRecipient,
	_ map[string]int,
) (*entity.SigningAttempt, error) {
	u.calls++
	u.attempt = &entity.SigningAttempt{ID: "attempt-1", DocumentID: documentID, Status: entity.SigningAttemptStatusCreated, CreatedAt: time.Now()}
	u.doc.ActiveAttemptID = &u.attempt.ID
	u.doc.Status = entity.DocumentStatusPreparingSignature
	return u.attempt, nil
}

type answersAttemptRepo struct {
	port.SigningAttemptRepository
	uow *answersUOW
}

func (r *answersAttemptRepo) BindTokenToAttempt(_ context.Context, _, _ string) error { return nil }

func (r *answersAttemptRepo) FindByID(_ context.Context, _ string) (*entity.SigningAttempt, error) {
	if r.uow.attempt == nil {
		return nil, entity.ErrRecordNotFound
	}
	return r.uow.attempt, nil
}
