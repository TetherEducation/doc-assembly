package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	organizationuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/organization"
)

// fakeWorkspaceMemberUseCase records the commands it receives. Methods the
// tests don't call fall through to the nil embedded interface and panic.
type fakeWorkspaceMemberUseCase struct {
	organizationuc.WorkspaceMemberUseCase
	invited []organizationuc.InviteMemberCommand
}

func (f *fakeWorkspaceMemberUseCase) InviteMember(_ context.Context, cmd organizationuc.InviteMemberCommand) (*entity.MemberWithUser, error) {
	f.invited = append(f.invited, cmd)
	return &entity.MemberWithUser{
		WorkspaceMember: entity.WorkspaceMember{ID: "member-1", WorkspaceID: cmd.WorkspaceID, Role: cmd.Role},
		User:            &entity.User{ID: "invitee-1", Email: cmd.Email},
	}, nil
}

// The invite stores InvitedBy in workspace_members.invited_by, a foreign key
// to identity.users(id). It must be the caller's internal user id, not the
// IdP subject from the token, or inviting anyone who has never signed in
// fails with a foreign-key violation.
func TestInviteMember_InvitedByIsInternalUserID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	members := &fakeWorkspaceMemberUseCase{}
	ctrl := &WorkspaceController{memberUC: members}

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/workspace/members",
		strings.NewReader(`{"email":"agent@example.com","fullName":"Agent","role":"OPERATOR"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("workspace_id", "ws-1")
	ctx.Set("user_id", "idp-subject-1")
	ctx.Set("internal_user_id", "internal-user-1")

	ctrl.InviteMember(ctx)

	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	require.Len(t, members.invited, 1)
	assert.Equal(t, "internal-user-1", members.invited[0].InvitedBy)
	assert.Equal(t, "ws-1", members.invited[0].WorkspaceID)
}
