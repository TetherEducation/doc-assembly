package template

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
	templateuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/template"
)

type approvalRepoFake struct {
	byID        map[string]*entity.TemplateVersionApproval
	pending     *entity.TemplateVersionApproval
	latest      *entity.TemplateVersionApproval
	created     []*entity.TemplateVersionApproval
	decisionErr error
	withdrawn   bool
	withdrawErr error
}

func (f *approvalRepoFake) Create(
	_ context.Context, a *entity.TemplateVersionApproval,
) (string, error) {
	a.ID = "approval-1"
	f.created = append(f.created, a)
	return a.ID, nil
}

func (f *approvalRepoFake) FindByID(
	_ context.Context, id string,
) (*entity.TemplateVersionApproval, error) {
	if a, ok := f.byID[id]; ok {
		return a, nil
	}
	return nil, entity.ErrApprovalNotFound
}

func (f *approvalRepoFake) FindLatestForVersion(
	_ context.Context, _ string,
) (*entity.TemplateVersionApproval, error) {
	if f.latest == nil {
		return nil, entity.ErrApprovalNotFound
	}
	return f.latest, nil
}

func (f *approvalRepoFake) FindPendingForVersion(
	_ context.Context, _ string,
) (*entity.TemplateVersionApproval, error) {
	if f.pending == nil {
		return nil, entity.ErrApprovalNotFound
	}
	return f.pending, nil
}

func (f *approvalRepoFake) ListForVersion(
	_ context.Context, _ string,
) ([]*entity.TemplateVersionApproval, error) {
	return nil, nil
}

func (f *approvalRepoFake) Withdraw(_ context.Context, _ string) error {
	f.withdrawn = true
	return f.withdrawErr
}

func (f *approvalRepoFake) RecordDecision(
	_ context.Context, _ *entity.TemplateVersionApproval,
) error {
	return f.decisionErr
}

// versionRepoStub satisfies versionContentReader - one method, because that is all
// the service asked for.
type versionRepoStub struct {
	content       any
	workspaceCode string
}

func newVersionRepoStub(content any) *versionRepoStub {
	return &versionRepoStub{content: content, workspaceCode: testWorkspace}
}

// The workspace a version really belongs to. Scoping checks the caller's claim
// against this rather than believing the header.
func (v *versionRepoStub) FindByIDWithDetailsAndTemplateWorkspace(
	_ context.Context, id string,
) (*port.TemplateVersionContext, error) {
	return &port.TemplateVersionContext{
		Workspace: &entity.Workspace{Code: v.workspaceCode},
	}, nil
}

func (v *versionRepoStub) FindByID(
	_ context.Context, id string,
) (*entity.TemplateVersion, error) {
	// ContentStructure is json.RawMessage in the real row, so the stub stores it
	// that way too - a stub that used a friendlier type would hide the raw-JSON
	// path the service actually takes.
	encoded, err := json.Marshal(v.content)
	if err != nil {
		return nil, err
	}
	return &entity.TemplateVersion{ID: id, ContentStructure: encoded}, nil
}

const testWorkspace = "AYELEN"

func newService(approvals *approvalRepoFake, content any) *TemplateApprovalService {
	return &TemplateApprovalService{
		approvalRepo: approvals,
		versionRepo:  newVersionRepoStub(content),
	}
}

func TestProposeHashesCurrentContent(t *testing.T) {
	t.Parallel()

	content := map[string]any{"clause": "original"}
	repo := &approvalRepoFake{}
	svc := newService(repo, content)

	approval, err := svc.Propose(context.Background(), templateuc.ProposeApprovalCommand{
		TemplateVersionID: "v1", ProposedBy: "chris@tether.education", WorkspaceCode: testWorkspace,
	})
	require.NoError(t, err)

	expected, err := entity.ChecksumOfContent(content)
	require.NoError(t, err)
	assert.Equal(t, expected, approval.ContentChecksum,
		"the proposal must pin the text the school is being shown")
	assert.Equal(t, entity.ApprovalStatusPending, approval.Status)
}

// Two outstanding proposals would make "the newest approval" ambiguous exactly when
// the publish gate consults it.
func TestProposeRefusesWhenOneIsOutstanding(t *testing.T) {
	t.Parallel()

	repo := &approvalRepoFake{
		pending: entity.NewTemplateVersionApproval("v1", "chris@tether.education", "sum"),
	}
	svc := newService(repo, map[string]any{"clause": "original"})

	_, err := svc.Propose(context.Background(), templateuc.ProposeApprovalCommand{
		TemplateVersionID: "v1", ProposedBy: "chris@tether.education", WorkspaceCode: testWorkspace,
	})
	require.Error(t, err)
	assert.Empty(t, repo.created, "nothing may be written when a proposal is outstanding")
}

func TestGetStateAuthorizesUnchangedContent(t *testing.T) {
	t.Parallel()

	content := map[string]any{"clause": "original"}
	sum, err := entity.ChecksumOfContent(content)
	require.NoError(t, err)

	approved := entity.NewTemplateVersionApproval("v1", "chris@tether.education", sum)
	require.NoError(t, approved.Decide(
		entity.ApprovalStatusApproved, "director@colegio.cl", "La Directora", "2036400001", nil))

	svc := newService(&approvalRepoFake{latest: approved}, content)

	state, err := svc.GetState(context.Background(), "v1")
	require.NoError(t, err)
	assert.True(t, state.Authorized)
	assert.False(t, state.Stale)
}

// The failure the whole feature exists to prevent.
func TestGetStateRefusesContentEditedAfterApproval(t *testing.T) {
	t.Parallel()

	approvedSum, err := entity.ChecksumOfContent(map[string]any{"clause": "original"})
	require.NoError(t, err)

	approved := entity.NewTemplateVersionApproval("v1", "chris@tether.education", approvedSum)
	require.NoError(t, approved.Decide(
		entity.ApprovalStatusApproved, "director@colegio.cl", "La Directora", "2036400001", nil))

	// The version now holds different content than the school signed off on.
	svc := newService(&approvalRepoFake{latest: approved}, map[string]any{"clause": "edited"})

	state, err := svc.GetState(context.Background(), "v1")
	require.NoError(t, err)
	assert.False(t, state.Authorized, "edited content must not publish on an old approval")
	assert.True(t, state.Stale, "and the reader must be told it went stale, not that it was never approved")
}

// Never proposed is a normal state, not an error - the page has to render it.
func TestGetStateOnAVersionNeverProposed(t *testing.T) {
	t.Parallel()

	svc := newService(&approvalRepoFake{}, map[string]any{"clause": "original"})

	state, err := svc.GetState(context.Background(), "v1")
	require.NoError(t, err)
	assert.Nil(t, state.Latest)
	assert.False(t, state.Authorized)
	assert.False(t, state.Stale, "never approved is not the same as stale")
}

func TestDecideRecordsApproval(t *testing.T) {
	t.Parallel()

	pending := entity.NewTemplateVersionApproval("v1", "chris@tether.education", "sum")
	pending.ID = "approval-1"
	repo := &approvalRepoFake{byID: map[string]*entity.TemplateVersionApproval{"approval-1": pending}}
	svc := newService(repo, map[string]any{"clause": "original"})

	decided, err := svc.Decide(context.Background(), templateuc.DecideApprovalCommand{
		ApprovalID: "approval-1", WorkspaceCode: testWorkspace, Approved: true,
		Email: "director@colegio.cl", Name: "La Directora", Campus: "2036400001",
	})
	require.NoError(t, err)
	assert.Equal(t, entity.ApprovalStatusApproved, decided.Status)
	assert.Equal(t, "2036400001", *decided.DecidedAtCampus)
}

func TestDecideRequiresACommentToRejectt(t *testing.T) {
	t.Parallel()

	pending := entity.NewTemplateVersionApproval("v1", "chris@tether.education", "sum")
	pending.ID = "approval-1"
	repo := &approvalRepoFake{byID: map[string]*entity.TemplateVersionApproval{"approval-1": pending}}
	svc := newService(repo, map[string]any{"clause": "original"})

	_, err := svc.Decide(context.Background(), templateuc.DecideApprovalCommand{
		ApprovalID: "approval-1", WorkspaceCode: testWorkspace, Approved: false,
		Email: "director@colegio.cl", Name: "La Directora", Campus: "2036400001",
	})
	require.ErrorIs(t, err, entity.ErrApprovalCommentRequired)
}

// The repository guards on status inside the UPDATE, so a second decider matches no
// row. That must surface as "already decided" rather than a silent success.
func TestDecideSurfacesALostRace(t *testing.T) {
	t.Parallel()

	pending := entity.NewTemplateVersionApproval("v1", "chris@tether.education", "sum")
	pending.ID = "approval-1"
	repo := &approvalRepoFake{
		byID:        map[string]*entity.TemplateVersionApproval{"approval-1": pending},
		decisionErr: entity.ErrApprovalAlreadyDecided,
	}
	svc := newService(repo, map[string]any{"clause": "original"})

	_, err := svc.Decide(context.Background(), templateuc.DecideApprovalCommand{
		ApprovalID: "approval-1", WorkspaceCode: testWorkspace, Approved: true,
		Email: "director@colegio.cl", Name: "La Directora", Campus: "2036400001",
	})
	require.ErrorIs(t, err, entity.ErrApprovalAlreadyDecided)
}

// ---------------------------------------------------------------------------
// Workspace scoping. The internal API key proves "a Tether service is calling",
// not which school it is calling for. Without these, a caller could record that
// one school's director approved another school's contract.
// ---------------------------------------------------------------------------

func TestProposeRefusesAVersionInAnotherWorkspace(t *testing.T) {
	t.Parallel()

	repo := &approvalRepoFake{}
	svc := newService(repo, map[string]any{"clause": "original"})

	_, err := svc.Propose(context.Background(), templateuc.ProposeApprovalCommand{
		TemplateVersionID: "v1", ProposedBy: "chris@tether.education",
		WorkspaceCode: "SOME_OTHER_SCHOOL",
	})
	require.ErrorIs(t, err, ErrWorkspaceMismatch)
	assert.Empty(t, repo.created, "nothing may be written for a workspace the caller does not own")
}

func TestDecideRefusesAnApprovalInAnotherWorkspace(t *testing.T) {
	t.Parallel()

	pending := entity.NewTemplateVersionApproval("v1", "chris@tether.education", "sum")
	pending.ID = "approval-1"
	repo := &approvalRepoFake{byID: map[string]*entity.TemplateVersionApproval{"approval-1": pending}}
	svc := newService(repo, map[string]any{"clause": "original"})

	_, err := svc.Decide(context.Background(), templateuc.DecideApprovalCommand{
		ApprovalID: "approval-1", WorkspaceCode: "SOME_OTHER_SCHOOL", Approved: true,
		Email: "impostor@elsewhere.cl", Name: "Someone", Campus: "9999999999",
	})
	require.ErrorIs(t, err, ErrWorkspaceMismatch)
	assert.Equal(t, entity.ApprovalStatusPending, pending.Status, "the proposal must be untouched")
}

// A missing header must not be a way to opt out of the check.
func TestAnEmptyWorkspaceClaimIsRefused(t *testing.T) {
	t.Parallel()

	svc := newService(&approvalRepoFake{}, map[string]any{"clause": "original"})

	_, err := svc.Propose(context.Background(), templateuc.ProposeApprovalCommand{
		TemplateVersionID: "v1", ProposedBy: "chris@tether.education", WorkspaceCode: "  ",
	})
	require.ErrorIs(t, err, ErrWorkspaceScopeRequired)
}

// The workspace code is a business code, not a secret; case should not decide
// whether a school can approve its own contract.
func TestWorkspaceClaimIsCaseInsensitive(t *testing.T) {
	t.Parallel()

	svc := newService(&approvalRepoFake{}, map[string]any{"clause": "original"})

	_, err := svc.Propose(context.Background(), templateuc.ProposeApprovalCommand{
		TemplateVersionID: "v1", ProposedBy: "chris@tether.education",
		WorkspaceCode: strings.ToLower(testWorkspace),
	})
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// Re-proposing unchanged content would move the latest approval back to PENDING
// and silently revoke a standing approval of the identical text.
// ---------------------------------------------------------------------------

func TestProposeRefusesContentAlreadyApproved(t *testing.T) {
	t.Parallel()

	content := map[string]any{"clause": "original"}
	encoded, err := json.Marshal(content)
	require.NoError(t, err)
	sum, err := entity.ChecksumOfContent(json.RawMessage(encoded))
	require.NoError(t, err)

	approved := entity.NewTemplateVersionApproval("v1", "chris@tether.education", sum)
	require.NoError(t, approved.Decide(
		entity.ApprovalStatusApproved, "director@colegio.cl", "La Directora", "2036400001", nil))

	repo := &approvalRepoFake{latest: approved}
	svc := newService(repo, content)

	_, err = svc.Propose(context.Background(), templateuc.ProposeApprovalCommand{
		TemplateVersionID: "v1", ProposedBy: "chris@tether.education", WorkspaceCode: testWorkspace,
	})
	require.ErrorIs(t, err, entity.ErrApprovalContentUnchanged)
	assert.Empty(t, repo.created, "a standing approval must not be revoked by an accidental re-proposal")
}

func TestProposeAllowsEditedContentAfterApproval(t *testing.T) {
	t.Parallel()

	approvedSum, err := entity.ChecksumOfContent(map[string]any{"clause": "original"})
	require.NoError(t, err)
	approved := entity.NewTemplateVersionApproval("v1", "chris@tether.education", approvedSum)
	require.NoError(t, approved.Decide(
		entity.ApprovalStatusApproved, "director@colegio.cl", "La Directora", "2036400001", nil))

	// The version has since been edited, so a fresh decision is exactly right.
	svc := newService(&approvalRepoFake{latest: approved}, map[string]any{"clause": "edited"})

	_, err = svc.Propose(context.Background(), templateuc.ProposeApprovalCommand{
		TemplateVersionID: "v1", ProposedBy: "chris@tether.education", WorkspaceCode: testWorkspace,
	})
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// Withdraw: a proposal sent by mistake must not have to be answered by a school.
// ---------------------------------------------------------------------------

func TestWithdrawRetractsAPendingProposal(t *testing.T) {
	t.Parallel()

	pending := entity.NewTemplateVersionApproval("v1", "chris@tether.education", "sum")
	pending.ID = "approval-1"
	repo := &approvalRepoFake{byID: map[string]*entity.TemplateVersionApproval{"approval-1": pending}}
	svc := newService(repo, map[string]any{"clause": "original"})

	require.NoError(t, svc.Withdraw(context.Background(), "approval-1", testWorkspace))
	assert.True(t, repo.withdrawn)
}

func TestWithdrawRefusesAnotherWorkspace(t *testing.T) {
	t.Parallel()

	pending := entity.NewTemplateVersionApproval("v1", "chris@tether.education", "sum")
	pending.ID = "approval-1"
	repo := &approvalRepoFake{byID: map[string]*entity.TemplateVersionApproval{"approval-1": pending}}
	svc := newService(repo, map[string]any{"clause": "original"})

	err := svc.Withdraw(context.Background(), "approval-1", "SOME_OTHER_SCHOOL")
	require.ErrorIs(t, err, ErrWorkspaceMismatch)
	assert.False(t, repo.withdrawn, "a caller must not retract another school's proposal")
}

// A decision is a historical fact and is never retracted - the repository guards
// on status, and that refusal must reach the caller intact.
func TestWithdrawRefusesSomethingAlreadyDecided(t *testing.T) {
	t.Parallel()

	decided := entity.NewTemplateVersionApproval("v1", "chris@tether.education", "sum")
	decided.ID = "approval-1"
	repo := &approvalRepoFake{
		byID:        map[string]*entity.TemplateVersionApproval{"approval-1": decided},
		withdrawErr: entity.ErrApprovalNotPending,
	}
	svc := newService(repo, map[string]any{"clause": "original"})

	err := svc.Withdraw(context.Background(), "approval-1", testWorkspace)
	require.ErrorIs(t, err, entity.ErrApprovalNotPending)
}

// A decision naming nobody fails at the one question this table exists to answer.
func TestDecideRefusesAnAnonymousDecider(t *testing.T) {
	t.Parallel()

	pending := entity.NewTemplateVersionApproval("v1", "chris@tether.education", "sum")
	pending.ID = "approval-1"
	repo := &approvalRepoFake{byID: map[string]*entity.TemplateVersionApproval{"approval-1": pending}}
	svc := newService(repo, map[string]any{"clause": "original"})

	_, err := svc.Decide(context.Background(), templateuc.DecideApprovalCommand{
		ApprovalID: "approval-1", WorkspaceCode: testWorkspace, Approved: true,
		Email: "   ", Name: "La Directora", Campus: "2036400001",
	})
	require.ErrorIs(t, err, entity.ErrApprovalDeciderRequired)
}
