package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/dto"
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	templatesvc "github.com/TetherEducation/doc-assembly/core/internal/core/service/template"
	templateuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/template"
)

const (
	testVersionUUID  = "11111111-1111-1111-1111-111111111111"
	testTemplateUUID = "22222222-2222-2222-2222-222222222222"
)

type fakeCommentUseCase struct {
	addCmd    templateuc.AddCommentCommand
	listQuery templateuc.ListCommentsQuery
	comments  []*entity.TemplateComment
	err       error
}

func (f *fakeCommentUseCase) AddComment(
	_ context.Context, cmd templateuc.AddCommentCommand,
) (*entity.TemplateComment, error) {
	f.addCmd = cmd
	if f.err != nil {
		return nil, f.err
	}
	return &entity.TemplateComment{
		ID: "c-1", TemplateID: testTemplateUUID, TemplateVersionID: cmd.TemplateVersionID,
		VersionNumber: 2, Audience: cmd.Audience, Body: cmd.Body,
		AuthorEmail: cmd.Email, AuthorCampus: cmd.Campus,
		CreatedAt: time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC),
	}, nil
}

func (f *fakeCommentUseCase) ListComments(
	_ context.Context, query templateuc.ListCommentsQuery,
) ([]*entity.TemplateComment, error) {
	f.listQuery = query
	if f.err != nil {
		return nil, f.err
	}
	return f.comments, nil
}

// newInternalRouter registers every internal controller on one engine, the way
// registerInternalRoutes does. gin panics on conflicting routes, so building it
// proves the comment routes coexist with /internal/templates/resolve,
// /internal/templates/:templateId/versions and the approval routes.
func newInternalRouter(t *testing.T, uc templateuc.TemplateCommentUseCase) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	api := engine.Group("/api/v1")
	noAuth := func(c *gin.Context) { c.Next() }

	NewInternalTemplateController(nil, &fakeVersionUseCase{}, fakeVersionMapper{}).RegisterRoutes(api, noAuth)
	NewInternalApprovalController(nil).RegisterRoutes(api, noAuth)
	NewInternalCommentController(uc).RegisterRoutes(api, noAuth)
	return engine
}

func doCommentRequest(engine *gin.Engine, method, target, body, workspace string) *httptest.ResponseRecorder {
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, target, nil)
	} else {
		req = httptest.NewRequest(method, target, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}
	if workspace != "" {
		req.Header.Set(HeaderWorkspaceCode, workspace)
	}
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	return recorder
}

func decodeError(t *testing.T, recorder *httptest.ResponseRecorder) dto.InternalErrorResponse {
	t.Helper()
	var resp dto.InternalErrorResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decoding error body %q: %v", recorder.Body.String(), err)
	}
	return resp
}

const validCommentBody = `{"audience":"AYELEN","body":"Revisar cláusula 3","email":"d@ayelen.cl","name":"D","campus":"AYELEN"}`

func TestAddCommentReturns201WithComment(t *testing.T) {
	uc := &fakeCommentUseCase{}
	engine := newInternalRouter(t, uc)

	recorder := doCommentRequest(engine, http.MethodPost,
		"/api/v1/internal/template-versions/"+testVersionUUID+"/comments", validCommentBody, "AYELEN")

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if uc.addCmd.TemplateVersionID != testVersionUUID || uc.addCmd.WorkspaceCode != "AYELEN" {
		t.Fatalf("version or workspace did not reach the use case: %+v", uc.addCmd)
	}

	var raw map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{
		"id", "templateId", "templateVersionId", "versionNumber", "audience", "body",
		"authorEmail", "authorName", "authorCampus", "createdAt",
	} {
		if _, ok := raw[key]; !ok {
			t.Errorf("response is missing %q: %s", key, recorder.Body.String())
		}
	}
}

// The template is derived from the version: a templateId in the body is ignored.
func TestAddCommentIgnoresTemplateIDInBody(t *testing.T) {
	uc := &fakeCommentUseCase{}
	engine := newInternalRouter(t, uc)

	body := `{"templateId":"someone-else","audience":"A","body":"x","email":"e","campus":"c"}`
	recorder := doCommentRequest(engine, http.MethodPost,
		"/api/v1/internal/template-versions/"+testVersionUUID+"/comments", body, "AYELEN")

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "someone-else") {
		t.Fatalf("templateId from the body leaked into the comment: %s", recorder.Body.String())
	}
}

func TestAddCommentErrors(t *testing.T) {
	cases := []struct {
		name      string
		version   string
		body      string
		workspace string
		ucErr     error
		status    int
		code      string
	}{
		{"missing body", testVersionUUID, "", "AYELEN", nil, http.StatusBadRequest, "INVALID_BODY"},
		{"malformed body", testVersionUUID, "{", "AYELEN", nil, http.StatusBadRequest, "INVALID_BODY"},
		{"missing workspace header", testVersionUUID, validCommentBody, "", nil, http.StatusBadRequest, "MISSING_HEADERS"},
		{"invalid comment", testVersionUUID, validCommentBody, "AYELEN", entity.ErrTemplateCommentInvalid, http.StatusBadRequest, "INVALID_BODY"},
		{"workspace mismatch", testVersionUUID, validCommentBody, "OTHER", templatesvc.ErrWorkspaceMismatch, http.StatusForbidden, "WORKSPACE_SCOPE"},
		{"version not found", testVersionUUID, validCommentBody, "AYELEN", entity.ErrVersionNotFound, http.StatusNotFound, ""},
		{"version not a uuid", "not-a-uuid", validCommentBody, "AYELEN", nil, http.StatusNotFound, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			engine := newInternalRouter(t, &fakeCommentUseCase{err: tc.ucErr})
			recorder := doCommentRequest(engine, http.MethodPost,
				"/api/v1/internal/template-versions/"+tc.version+"/comments", tc.body, tc.workspace)

			if recorder.Code != tc.status {
				t.Fatalf("expected %d, got %d: %s", tc.status, recorder.Code, recorder.Body.String())
			}
			if tc.code != "" {
				if got := decodeError(t, recorder).Code; got != tc.code {
					t.Fatalf("expected code %s, got %s", tc.code, got)
				}
			}
		})
	}
}

func TestListCommentsReturnsItems(t *testing.T) {
	uc := &fakeCommentUseCase{comments: []*entity.TemplateComment{
		{ID: "first", VersionNumber: 1},
		{ID: "second", VersionNumber: 2},
	}}
	engine := newInternalRouter(t, uc)

	recorder := doCommentRequest(engine, http.MethodGet,
		"/api/v1/internal/templates/"+testTemplateUUID+"/comments?audience=CAMPUS_X", "", "AYELEN")

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if uc.listQuery.TemplateID != testTemplateUUID || uc.listQuery.Audience != "CAMPUS_X" ||
		uc.listQuery.WorkspaceCode != "AYELEN" {
		t.Fatalf("query did not reach the use case: %+v", uc.listQuery)
	}

	var resp dto.InternalCommentListResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Items) != 2 || resp.Items[0].ID != "first" || resp.Items[1].ID != "second" {
		t.Fatalf("expected the use case order to be preserved, got %+v", resp.Items)
	}
}

func TestListCommentsEmptyIsEmptyArray(t *testing.T) {
	engine := newInternalRouter(t, &fakeCommentUseCase{})

	recorder := doCommentRequest(engine, http.MethodGet,
		"/api/v1/internal/templates/"+testTemplateUUID+"/comments?audience=A", "", "AYELEN")

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), `"items":[]`) {
		t.Fatalf("expected an empty array, got %s", recorder.Body.String())
	}
}

func TestListCommentsErrors(t *testing.T) {
	cases := []struct {
		name      string
		template  string
		query     string
		workspace string
		ucErr     error
		status    int
		code      string
	}{
		{"missing audience", testTemplateUUID, "", "AYELEN", nil, http.StatusBadRequest, "INVALID_QUERY"},
		{"blank audience", testTemplateUUID, "?audience=%20", "AYELEN", nil, http.StatusBadRequest, "INVALID_QUERY"},
		{"missing workspace header", testTemplateUUID, "?audience=A", "", nil, http.StatusBadRequest, "MISSING_HEADERS"},
		{"workspace mismatch", testTemplateUUID, "?audience=A", "OTHER", templatesvc.ErrWorkspaceMismatch, http.StatusForbidden, "WORKSPACE_SCOPE"},
		{"template not found", testTemplateUUID, "?audience=A", "AYELEN", entity.ErrTemplateNotFound, http.StatusNotFound, ""},
		{"template not a uuid", "nope", "?audience=A", "AYELEN", nil, http.StatusNotFound, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			engine := newInternalRouter(t, &fakeCommentUseCase{err: tc.ucErr})
			recorder := doCommentRequest(engine, http.MethodGet,
				"/api/v1/internal/templates/"+tc.template+"/comments"+tc.query, "", tc.workspace)

			if recorder.Code != tc.status {
				t.Fatalf("expected %d, got %d: %s", tc.status, recorder.Code, recorder.Body.String())
			}
			if tc.code != "" {
				if got := decodeError(t, recorder).Code; got != tc.code {
					t.Fatalf("expected code %s, got %s", tc.code, got)
				}
			}
		})
	}
}

// The existing static route must still win over the new wildcard sibling.
func TestResolveRouteStillReachable(t *testing.T) {
	engine := newInternalRouter(t, &fakeCommentUseCase{})

	recorder := doCommentRequest(engine, http.MethodGet, "/api/v1/internal/templates/resolve", "", "")

	// ResolveTemplate answers 500 with a nil use case; what matters is that it, not
	// ListComments (which would answer 400 INVALID_QUERY), handled the request.
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("expected ResolveTemplate to handle /resolve, got %d: %s", recorder.Code, recorder.Body.String())
	}
}
