package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/rendis/doc-assembly/core/internal/adapters/primary/http/middleware"
	"github.com/rendis/doc-assembly/core/internal/core/port"
	documentuc "github.com/rendis/doc-assembly/core/internal/core/usecase/document"
)

type signingSessionUCStub struct {
	principal *documentuc.SigningSessionPrincipal
}

func (s *signingSessionUCStub) CreateOrGetSession(_ context.Context, _ string, principal *documentuc.SigningSessionPrincipal) (*documentuc.SigningSessionResponse, error) {
	s.principal = principal
	return &documentuc.SigningSessionResponse{SessionURL: "/public/sign/token"}, nil
}

type signingSessionAuthStub struct{}

func (signingSessionAuthStub) Authenticate(_ *gin.Context, _ *port.SigningSessionAuthenticateRequest) (*port.SigningSessionAuthClaims, error) {
	return &port.SigningSessionAuthClaims{Email: "alice@example.com"}, nil
}

func TestSigningSessionControllerPassesExplicitLanguage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	uc := &signingSessionUCStub{}
	router := gin.New()
	NewSigningSessionController(uc).RegisterRoutes(router, middleware.SigningSessionCustomAuth(signingSessionAuthStub{}))

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/signing-sessions/doc-1?language=en", nil)
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if uc.principal == nil || uc.principal.Language != "en" {
		t.Fatalf("expected principal language en, got %#v", uc.principal)
	}
}

func TestSigningSessionControllerFallsBackUnsupportedExplicitLanguage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	uc := &signingSessionUCStub{}
	router := gin.New()
	NewSigningSessionController(uc).RegisterRoutes(router, middleware.SigningSessionCustomAuth(signingSessionAuthStub{}))

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/signing-sessions/doc-1?language=pt", nil)
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if uc.principal == nil || uc.principal.Language != "en" {
		t.Fatalf("expected unsupported explicit language to fallback to en, got %#v", uc.principal)
	}
}

type publicAccessUCStub struct {
	requestLanguage string
	tokenLanguage   string
}

func (publicAccessUCStub) GetPublicDocumentInfo(context.Context, string) (*documentuc.PublicDocumentInfoResponse, error) {
	return &documentuc.PublicDocumentInfoResponse{DocumentID: "doc-1", DocumentTitle: "Contract", Status: "active"}, nil
}
func (s *publicAccessUCStub) RequestAccess(_ context.Context, _, _, language string) error {
	s.requestLanguage = language
	return nil
}
func (s *publicAccessUCStub) RequestAccessByToken(_ context.Context, _, _, language string) error {
	s.tokenLanguage = language
	return nil
}
func (*publicAccessUCStub) RequestDirectAccess(context.Context, string, string) (string, error) {
	return "/public/sign/token", nil
}

type publicDocAuthStub struct{}

func (publicDocAuthStub) Authenticate(_ *gin.Context, _ *port.AuthenticateRequest) (*port.PublicDocumentAccessClaims, error) {
	return &port.PublicDocumentAccessClaims{Email: "alice@example.com"}, nil
}

func TestPublicDocumentAccessRedirectPreservesExplicitLanguage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewPublicDocumentAccessController(&publicAccessUCStub{}).RegisterRoutes(router, middleware.CustomPublicDocumentAccess(publicDocAuthStub{}))

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/public/doc/doc-1?language=es", nil)
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("Location"); got != "/public/sign/token?language=es" {
		t.Fatalf("expected language-preserving redirect, got %q", got)
	}
}

func TestPublicDocumentAccessDirectJSONPreservesExplicitLanguage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewPublicDocumentAccessController(&publicAccessUCStub{}).RegisterRoutes(router, middleware.CustomPublicDocumentAccess(publicDocAuthStub{}))

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/public/doc/doc-1/request-access?language=en", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var body struct {
		SigningURL string `json:"signingUrl"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if body.SigningURL != "/public/sign/token?language=en" {
		t.Fatalf("expected language-preserving signingUrl, got %q", body.SigningURL)
	}
}

func TestPublicDocumentAccessEmailGatePassesExplicitLanguage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	uc := &publicAccessUCStub{}
	router := gin.New()
	NewPublicDocumentAccessController(uc).RegisterRoutes(router)

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/public/doc/doc-1/request-access?language=es", bytes.NewBufferString(`{"email":"alice@example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if uc.requestLanguage != "es" {
		t.Fatalf("expected email gate language es, got %q", uc.requestLanguage)
	}
}

func TestPublicSigningExpiredTokenRecoveryPassesExplicitLanguage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	uc := &publicAccessUCStub{}
	router := gin.New()
	NewPublicSigningController(nil, uc, "").RegisterRoutes(router)

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/public/sign/expired-token/request-access?language=en", bytes.NewBufferString(`{"email":"alice@example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if uc.tokenLanguage != "en" {
		t.Fatalf("expected expired token recovery language en, got %q", uc.tokenLanguage)
	}
}
