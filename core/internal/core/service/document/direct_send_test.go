package document

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
	documentuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/document"
)

// --- fakes -------------------------------------------------------------------
//
// Each fake embeds its port interface so only the methods the creation flow
// actually calls need an implementation; anything else panics loudly.

type directSendDocRepo struct {
	port.DocumentRepository
	docs       map[string]*entity.Document
	recipients map[string][]*entity.DocumentRecipient
	replayID   string
}

func newDirectSendDocRepo() *directSendDocRepo {
	return &directSendDocRepo{
		docs:       map[string]*entity.Document{},
		recipients: map[string][]*entity.DocumentRecipient{},
	}
}

func (r *directSendDocRepo) Create(_ context.Context, doc *entity.Document) (string, error) {
	cp := *doc
	r.docs[doc.ID] = &cp
	return doc.ID, nil
}

func (r *directSendDocRepo) Update(_ context.Context, doc *entity.Document) error {
	cp := *doc
	r.docs[doc.ID] = &cp
	return nil
}

func (r *directSendDocRepo) FindByID(_ context.Context, id string) (*entity.Document, error) {
	doc, ok := r.docs[id]
	if !ok {
		return nil, entity.ErrDocumentNotFound
	}
	cp := *doc
	return &cp, nil
}

func (r *directSendDocRepo) FindByIDWithRecipients(ctx context.Context, id string) (*entity.DocumentWithRecipients, error) {
	doc, err := r.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return &entity.DocumentWithRecipients{Document: *doc, Recipients: r.recipients[id]}, nil
}

func (r *directSendDocRepo) FindInternalCreateReplay(context.Context, string, string, string, string) (string, bool, error) {
	if r.replayID == "" {
		return "", false, nil
	}
	return r.replayID, true, nil
}

type directSendRecipientRepo struct {
	port.DocumentRecipientRepository
	docs *directSendDocRepo
}

func (r *directSendRecipientRepo) CreateBatch(_ context.Context, recipients []*entity.DocumentRecipient) error {
	for _, rec := range recipients {
		r.docs.recipients[rec.DocumentID] = append(r.docs.recipients[rec.DocumentID], rec)
	}
	return nil
}

func (r *directSendRecipientRepo) FindByDocumentID(_ context.Context, documentID string) ([]*entity.DocumentRecipient, error) {
	return r.docs.recipients[documentID], nil
}

type directSendTemplateRepo struct {
	port.TemplateRepository
}

func (directSendTemplateRepo) FindTemplateWorkspaceByTemplateID(context.Context, string) (*port.TemplateWorkspace, error) {
	docType := "doctype-1"
	return &port.TemplateWorkspace{
		Template:  &entity.Template{ID: "tpl-1", DocumentTypeID: &docType},
		Workspace: &entity.Workspace{ID: "ws-1", Type: entity.WorkspaceTypeClient},
	}, nil
}

type directSendVersionRepo struct {
	port.TemplateVersionRepository
	content json.RawMessage
}

func (r *directSendVersionRepo) FindByID(_ context.Context, id string) (*entity.TemplateVersion, error) {
	return &entity.TemplateVersion{
		ID:               id,
		TemplateID:       "tpl-1",
		Status:           entity.VersionStatusPublished,
		ContentStructure: r.content,
	}, nil
}

type directSendSignerRoleRepo struct {
	port.TemplateVersionSignerRoleRepository
}

func (directSendSignerRoleRepo) FindByVersionID(_ context.Context, versionID string) ([]*entity.TemplateVersionSignerRole, error) {
	return []*entity.TemplateVersionSignerRole{
		{ID: "role-client", TemplateVersionID: versionID, RoleName: "Client", SignerOrder: 2},
		{ID: "role-tether", TemplateVersionID: versionID, RoleName: "Tether", SignerOrder: 1},
	}, nil
}

type attemptCall struct {
	documentID   string
	recipients   []*entity.DocumentRecipient
	signerOrders map[string]int
}

// directSendUOW mimics riverqueue.SigningExecutionUnitOfWork: on success the
// document gets the attempt projection (PREPARING_SIGNATURE + active attempt).
type directSendUOW struct {
	port.SigningExecutionUnitOfWork
	docs  *directSendDocRepo
	calls []attemptCall
	err   error
}

func (u *directSendUOW) CreateAttemptAndEnqueueRender(
	_ context.Context,
	documentID string,
	recipients []*entity.DocumentRecipient,
	signerOrders map[string]int,
) (*entity.SigningAttempt, error) {
	u.calls = append(u.calls, attemptCall{documentID: documentID, recipients: recipients, signerOrders: signerOrders})
	if u.err != nil {
		return nil, u.err
	}
	attempt := &entity.SigningAttempt{
		ID:         fmt.Sprintf("attempt-%d", len(u.calls)),
		DocumentID: documentID,
		Sequence:   1,
		Status:     entity.SigningAttemptStatusCreated,
	}
	doc := u.docs.docs[documentID]
	doc.ActiveAttemptID = &attempt.ID
	doc.Status = entity.ProjectDocumentStatusFromAttempt(attempt.Status)
	return attempt, nil
}

type directSendEventRepo struct {
	port.DocumentEventRepository
	events []*entity.DocumentEvent
}

func (r *directSendEventRepo) Create(_ context.Context, event *entity.DocumentEvent) error {
	r.events = append(r.events, event)
	return nil
}

type directSendNotifier struct {
	sent []*port.NotificationRequest
}

func (n *directSendNotifier) Send(_ context.Context, req *port.NotificationRequest) error {
	n.sent = append(n.sent, req)
	return nil
}

// --- fixtures ----------------------------------------------------------------

const (
	contentWithoutFields = `{"version":"1","content":{"type":"doc","content":[
		{"type":"paragraph","content":[{"type":"text","text":"Mutual NDA"}]}
	]}}`
	// The interactive field is nested to prove detection is recursive.
	contentWithFields = `{"version":"1","content":{"type":"doc","content":[
		{"type":"paragraph","content":[{"type":"text","text":"Pick one"}]},
		{"type":"blockquote","content":[
			{"type":"interactiveField","attrs":{"id":"f1","fieldType":"checkbox","roleId":"role-client","label":"Agree?"}}
		]}
	]}}`
)

type directSendEnv struct {
	svc      *DocumentService
	docs     *directSendDocRepo
	uow      *directSendUOW
	events   *directSendEventRepo
	notifier *directSendNotifier
}

func newDirectSendEnv(content string, directSend bool) *directSendEnv {
	docs := newDirectSendDocRepo()
	recipients := &directSendRecipientRepo{docs: docs}
	uow := &directSendUOW{docs: docs}
	events := &directSendEventRepo{}
	notifier := &directSendNotifier{}
	signerRoles := directSendSignerRoleRepo{}

	var opts []DocumentServiceOption
	if directSend {
		opts = append(opts, WithDirectSend(NewDirectSender(true, uow, signerRoles)))
	}

	svc := NewDocumentService(
		docs, recipients, nil, uow,
		directSendTemplateRepo{}, &directSendVersionRepo{content: json.RawMessage(content)}, signerRoles,
		nil, nil, nil,
		NewEventEmitter(events),
		NewNotificationService(notifier, recipients, docs, nil, ""),
		30,
		nil, nil,
		false,
		opts...,
	)
	return &directSendEnv{svc: svc, docs: docs, uow: uow, events: events, notifier: notifier}
}

func directSendCreateCmd() documentuc.CreateDocumentCommand {
	return documentuc.CreateDocumentCommand{
		WorkspaceID:       "ws-1",
		TemplateVersionID: "ver-1",
		Title:             "Mutual NDA",
		Recipients: []documentuc.DocumentRecipientCommand{
			{RoleID: "role-tether", Name: "Zack", Email: "zack@example.com"},
			{RoleID: "role-client", Name: "Client", Email: "client@example.com"},
		},
	}
}

func eventTypes(events []*entity.DocumentEvent) []string {
	types := make([]string, len(events))
	for i, e := range events {
		types[i] = e.EventType
	}
	return types
}

// --- DocumentService.CreateAndSendDocument -------------------------------------

func TestCreateAndSendDocument_DirectSendOff_EntersAwaitingInput(t *testing.T) {
	env := newDirectSendEnv(contentWithoutFields, false)

	got, err := env.svc.CreateAndSendDocument(context.Background(), directSendCreateCmd())
	require.NoError(t, err)

	assert.Equal(t, entity.DocumentStatusAwaitingInput, got.Status)
	assert.Nil(t, got.ActiveAttemptID)
	assert.Empty(t, env.uow.calls, "no signing attempt may be created when direct send is off")
	assert.Equal(t, entity.DocumentStatusAwaitingInput, env.docs.docs[got.ID].Status)
	assert.Equal(t, []string{entity.EventDocumentCreated}, eventTypes(env.events.events))
	assert.Len(t, env.notifier.sent, 2, "created notification goes to every recipient")
}

func TestCreateAndSendDocument_DirectSendOn_NoFillableFields_StartsSigning(t *testing.T) {
	env := newDirectSendEnv(contentWithoutFields, true)

	got, err := env.svc.CreateAndSendDocument(context.Background(), directSendCreateCmd())
	require.NoError(t, err)

	require.Len(t, env.uow.calls, 1, "exactly one signing attempt must be enqueued")
	call := env.uow.calls[0]
	assert.Equal(t, got.ID, call.documentID)
	require.Len(t, call.recipients, 2)
	assert.Equal(t, "role-tether", call.recipients[0].TemplateVersionRoleID)
	assert.NotEmpty(t, call.recipients[0].ID, "attempt recipients must link to persisted document recipients")
	assert.Equal(t, map[string]int{"role-tether": 1, "role-client": 2}, call.signerOrders)

	// The returned document reflects the attempt projection, like ProceedToSigning.
	assert.Equal(t, entity.DocumentStatusPreparingSignature, got.Status)
	require.NotNil(t, got.ActiveAttemptID)
	assert.Equal(t, "attempt-1", *got.ActiveAttemptID)
	assert.Len(t, got.Recipients, 2)

	// Same document timeline as the normal path: created (DRAFT -> AWAITING_INPUT).
	assert.Equal(t, []string{entity.EventDocumentCreated}, eventTypes(env.events.events))
	assert.Len(t, env.notifier.sent, 2)
}

func TestCreateAndSendDocument_DirectSendOn_WithFillableFields_EntersAwaitingInput(t *testing.T) {
	env := newDirectSendEnv(contentWithFields, true)

	got, err := env.svc.CreateAndSendDocument(context.Background(), directSendCreateCmd())
	require.NoError(t, err)

	assert.Equal(t, entity.DocumentStatusAwaitingInput, got.Status)
	assert.Nil(t, got.ActiveAttemptID)
	assert.Empty(t, env.uow.calls, "signer-fillable fields need the pre-signing form")
}

func TestCreateAndSendDocument_DirectSendOn_EnqueueFailure_ReturnsError(t *testing.T) {
	env := newDirectSendEnv(contentWithoutFields, true)
	env.uow.err = errors.New("river unavailable")

	got, err := env.svc.CreateAndSendDocument(context.Background(), directSendCreateCmd())
	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "river unavailable")

	// The attempt transaction rolled back, so the document stays in the
	// normal-path AWAITING_INPUT state (recoverable via the signer flow).
	require.Len(t, env.docs.docs, 1)
	for _, doc := range env.docs.docs {
		assert.Equal(t, entity.DocumentStatusAwaitingInput, doc.Status)
		assert.Nil(t, doc.ActiveAttemptID)
	}
}

func TestCreateDocumentsBatch_DirectSendOn_SendsEachDocument(t *testing.T) {
	env := newDirectSendEnv(contentWithoutFields, true)

	results, err := env.svc.CreateDocumentsBatch(context.Background(), []documentuc.CreateDocumentCommand{
		directSendCreateCmd(), directSendCreateCmd(), directSendCreateCmd(),
	})
	require.NoError(t, err)
	require.Len(t, results, 3)
	for _, r := range results {
		require.NoError(t, r.Error)
		assert.Equal(t, entity.DocumentStatusPreparingSignature, r.Document.Status)
	}
	assert.Len(t, env.uow.calls, 3)
}

func TestCreateDocumentsBatch_DirectSendOff_Unchanged(t *testing.T) {
	env := newDirectSendEnv(contentWithoutFields, false)

	results, err := env.svc.CreateDocumentsBatch(context.Background(), []documentuc.CreateDocumentCommand{
		directSendCreateCmd(), directSendCreateCmd(),
	})
	require.NoError(t, err)
	for _, r := range results {
		require.NoError(t, r.Error)
		assert.Equal(t, entity.DocumentStatusAwaitingInput, r.Document.Status)
	}
	assert.Empty(t, env.uow.calls)
}

func TestNewDirectSender_DisabledIsInert(t *testing.T) {
	env := newDirectSendEnv(contentWithoutFields, false)
	disabled := NewDirectSender(false, env.uow, directSendSignerRoleRepo{})
	env.svc = NewDocumentService(
		env.docs, &directSendRecipientRepo{docs: env.docs}, nil, env.uow,
		directSendTemplateRepo{}, &directSendVersionRepo{content: json.RawMessage(contentWithoutFields)}, directSendSignerRoleRepo{},
		nil, nil, nil,
		NewEventEmitter(env.events),
		NewNotificationService(env.notifier, &directSendRecipientRepo{docs: env.docs}, env.docs, nil, ""),
		30, nil, nil, false,
		WithDirectSend(disabled),
	)

	got, err := env.svc.CreateAndSendDocument(context.Background(), directSendCreateCmd())
	require.NoError(t, err)
	assert.Equal(t, entity.DocumentStatusAwaitingInput, got.Status)
	assert.Empty(t, env.uow.calls)
}

// --- field detection ---------------------------------------------------------

func TestHasSignerFillableFields(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    bool
	}{
		{"no fields", contentWithoutFields, false},
		{"nested interactive field", contentWithFields, true},
		{"empty content", ``, false},
		{"unparseable content fails safe", `{not json`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, hasSignerFillableFields(json.RawMessage(tc.content)))
		})
	}
}

// --- InternalDocumentService (internal API) ----------------------------------

func newInternalDirectSendEnv(content string, directSend bool) (*InternalDocumentService, *directSendDocRepo, *directSendUOW) {
	docs := newDirectSendDocRepo()
	uow := &directSendUOW{docs: docs}
	var opts []InternalDocumentServiceOption
	if directSend {
		opts = append(opts, WithInternalDirectSend(NewDirectSender(true, uow, directSendSignerRoleRepo{})))
	}
	svc := NewInternalDocumentService(
		nil, docs, nil, nil, nil, nil,
		&directSendVersionRepo{content: json.RawMessage(content)},
		nil,
		opts...,
	).(*InternalDocumentService)
	return svc, docs, uow
}

// seedInternalAwaitingDoc stores a document the way InternalCreateOrReplay
// leaves it: AWAITING_INPUT with persisted recipients and no attempt.
func seedInternalAwaitingDoc(docs *directSendDocRepo) *entity.DocumentWithRecipients {
	doc := entity.NewDocument("ws-1", "ver-1")
	doc.ID = "doc-internal"
	doc.DocumentTypeID = "doctype-1"
	_ = doc.MarkAsAwaitingInput()
	docs.docs[doc.ID] = doc
	docs.recipients[doc.ID] = []*entity.DocumentRecipient{
		{ID: "rec-1", DocumentID: doc.ID, TemplateVersionRoleID: "role-tether", Name: "Zack", Email: "zack@example.com"},
		{ID: "rec-2", DocumentID: doc.ID, TemplateVersionRoleID: "role-client", Name: "Client", Email: "client@example.com"},
	}
	stored, _ := docs.FindByIDWithRecipients(context.Background(), doc.ID)
	return stored
}

func TestInternalDirectSend_Off_LeavesAwaitingInput(t *testing.T) {
	svc, docs, uow := newInternalDirectSendEnv(contentWithoutFields, false)
	stored := seedInternalAwaitingDoc(docs)

	got, err := svc.applyDirectSend(context.Background(), stored, json.RawMessage(contentWithoutFields))
	require.NoError(t, err)
	assert.Same(t, stored, got)
	assert.Equal(t, entity.DocumentStatusAwaitingInput, got.Status)
	assert.Empty(t, uow.calls)
}

func TestInternalDirectSend_On_NoFillableFields_StartsSigning(t *testing.T) {
	svc, docs, uow := newInternalDirectSendEnv(contentWithoutFields, true)
	stored := seedInternalAwaitingDoc(docs)

	got, err := svc.applyDirectSend(context.Background(), stored, json.RawMessage(contentWithoutFields))
	require.NoError(t, err)
	require.Len(t, uow.calls, 1)
	assert.Equal(t, "doc-internal", uow.calls[0].documentID)
	assert.Equal(t, map[string]int{"role-tether": 1, "role-client": 2}, uow.calls[0].signerOrders)
	assert.Equal(t, entity.DocumentStatusPreparingSignature, got.Status)
	assert.Len(t, got.Recipients, 2)
}

func TestInternalDirectSend_On_WithFillableFields_LeavesAwaitingInput(t *testing.T) {
	svc, docs, uow := newInternalDirectSendEnv(contentWithFields, true)
	stored := seedInternalAwaitingDoc(docs)

	got, err := svc.applyDirectSend(context.Background(), stored, json.RawMessage(contentWithFields))
	require.NoError(t, err)
	assert.Equal(t, entity.DocumentStatusAwaitingInput, got.Status)
	assert.Empty(t, uow.calls)
}

func TestInternalDirectSend_On_EnqueueFailure_ReturnsError(t *testing.T) {
	svc, docs, uow := newInternalDirectSendEnv(contentWithoutFields, true)
	uow.err = errors.New("river unavailable")
	stored := seedInternalAwaitingDoc(docs)

	_, err := svc.applyDirectSend(context.Background(), stored, json.RawMessage(contentWithoutFields))
	require.Error(t, err)
	assert.Equal(t, entity.DocumentStatusAwaitingInput, docs.docs["doc-internal"].Status)
}

// A failed enqueue followed by the caller's idempotent retry must not leave the
// document stuck in AWAITING_INPUT: the replay finishes the send.
func TestInternalDirectSend_On_ReplayRecoversStrandedDocument(t *testing.T) {
	svc, docs, uow := newInternalDirectSendEnv(contentWithoutFields, true)
	seedInternalAwaitingDoc(docs)
	docs.replayID = "doc-internal"

	result, ok, err := svc.findInternalCreateReplay(
		context.Background(),
		documentuc.InternalCreateCommand{ExternalID: "ext-1", TransactionalID: "tx-1"},
		&internalResolvedContext{workspace: &entity.Workspace{ID: "ws-1"}, documentType: &entity.DocumentType{ID: "doctype-1"}},
	)
	require.NoError(t, err)
	require.True(t, ok)
	assert.True(t, result.IdempotentReplay)
	require.Len(t, uow.calls, 1)
	assert.Equal(t, entity.DocumentStatusPreparingSignature, result.Document.Status)

	// A second replay sees the active attempt and does nothing more.
	result, _, err = svc.findInternalCreateReplay(
		context.Background(),
		documentuc.InternalCreateCommand{ExternalID: "ext-1", TransactionalID: "tx-1"},
		&internalResolvedContext{workspace: &entity.Workspace{ID: "ws-1"}, documentType: &entity.DocumentType{ID: "doctype-1"}},
	)
	require.NoError(t, err)
	assert.Len(t, uow.calls, 1)
	assert.Equal(t, entity.DocumentStatusPreparingSignature, result.Document.Status)
}

func TestInternalDirectSend_Off_ReplayUnchanged(t *testing.T) {
	svc, docs, uow := newInternalDirectSendEnv(contentWithoutFields, false)
	seedInternalAwaitingDoc(docs)
	docs.replayID = "doc-internal"

	result, ok, err := svc.findInternalCreateReplay(
		context.Background(),
		documentuc.InternalCreateCommand{ExternalID: "ext-1", TransactionalID: "tx-1"},
		&internalResolvedContext{workspace: &entity.Workspace{ID: "ws-1"}, documentType: &entity.DocumentType{ID: "doctype-1"}},
	)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, entity.DocumentStatusAwaitingInput, result.Document.Status)
	assert.Empty(t, uow.calls)
}
