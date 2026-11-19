//go:build integration

package controller_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

// A signer with no questions of their own waits while another signer's required
// questions are unanswered, and cannot start the signing attempt before they are.
func TestPublicSigningController_SignerWithoutQuestionsWaitsForRequiredAnswers(t *testing.T) {
	env := setupDocumentEnv(t)
	// field-1 is required and belongs to alice's role; bob has no questions.
	setTemplateVersionInteractiveContent(t, env, "rol 1", "rol 2")
	doc := env.createDocument(t, "Waits For Required Answers")

	type signingPage struct {
		Step               string                  `json:"step"`
		Form               *publicSigningFormState `json:"form,omitempty"`
		PdfURL             string                  `json:"pdfUrl"`
		WaitingForPrevious bool                    `json:"waitingForPrevious"`
	}
	decode := func(resp *http.Response, body []byte) signingPage {
		t.Helper()
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		var page signingPage
		require.NoError(t, json.Unmarshal(body, &page))
		return page
	}
	token := func(email string) string {
		t.Helper()
		resp, _ := env.client.POST("/public/doc/"+doc.ID+"/request-access", map[string]string{"email": email})
		require.Equal(t, http.StatusOK, resp.StatusCode)
		tok := findLatestAccessToken(t, env, doc.ID, email)
		require.NotEmpty(t, tok)
		return tok
	}
	attempts := func() int {
		t.Helper()
		var n int
		require.NoError(t, env.ts.Pool.QueryRow(context.Background(),
			`SELECT COUNT(*) FROM execution.signing_attempts WHERE document_id = $1`, doc.ID).Scan(&n))
		return n
	}
	alice, bob := token("alice@test.com"), token("bob@test.com")

	page := decode(env.client.GET("/public/sign/" + bob))
	require.Equal(t, "waiting", page.Step)
	require.True(t, page.WaitingForPrevious)
	require.Nil(t, page.Form)

	page = decode(env.client.POST("/public/sign/"+bob+"/proceed", nil))
	require.Equal(t, "waiting", page.Step)
	require.Zero(t, attempts(), "no signing attempt before the required answers")

	page = decode(env.client.GET("/public/sign/" + alice))
	require.Equal(t, "preview", page.Step)
	require.NotNil(t, page.Form)
	require.Len(t, page.Form.Fields, 1)

	resp, body := env.client.POST("/public/sign/"+alice, map[string]any{
		"responses": []map[string]any{{
			"fieldId":  "field-1",
			"response": map[string]any{"selectedOptionIds": []string{"opt-1"}},
		}},
	})
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))

	page = decode(env.client.GET("/public/sign/" + bob))
	require.Equal(t, "preview", page.Step)
	require.Nil(t, page.Form)
	require.NotEmpty(t, page.PdfURL)

	page = decode(env.client.POST("/public/sign/"+bob+"/proceed", nil))
	require.Equal(t, "processing", page.Step)
	require.Equal(t, 1, attempts())
}
