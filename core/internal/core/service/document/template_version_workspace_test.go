package document

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

// A template version whose template is in another workspace answers exactly as
// a missing version does (see findTemplateVersion), before the recipients are
// checked against its signer roles, and nothing is created or started. The
// direct-send fakes keep the template in ws-1.
func TestCreateAndSendDocument_TemplateVersionFromAnotherWorkspace(t *testing.T) {
	for name, directSend := range map[string]bool{"direct send off": false, "direct send on": true} {
		t.Run(name, func(t *testing.T) {
			env := newDirectSendEnv(contentWithoutFields, directSend)
			cmd := directSendCreateCmd()
			cmd.WorkspaceID = "ws-2"
			cmd.Recipients = cmd.Recipients[:1] // fewer than the version's two signer roles

			got, err := env.svc.CreateAndSendDocument(context.Background(), cmd)

			require.ErrorIs(t, err, entity.ErrVersionNotFound)
			assert.EqualError(t, err, "finding template version: "+entity.ErrVersionNotFound.Error(),
				"the answer must be the one a missing version gets")
			assert.Nil(t, got)
			assert.Empty(t, env.docs.docs, "no document may be created")
			assert.Empty(t, env.uow.calls, "no signing attempt may be started")
			assert.Empty(t, env.events.events)
		})
	}
}
