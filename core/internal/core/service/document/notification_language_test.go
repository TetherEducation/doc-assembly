package document

import (
	"context"
	"strings"
	"testing"

	"github.com/rendis/doc-assembly/core/internal/core/entity"
	"github.com/rendis/doc-assembly/core/internal/core/port"
)

type capturingNotificationProvider struct {
	req *port.NotificationRequest
}

func (p *capturingNotificationProvider) Send(_ context.Context, req *port.NotificationRequest) error {
	p.req = req
	return nil
}

func TestSendAccessLinkPreservesExplicitLanguage(t *testing.T) {
	provider := &capturingNotificationProvider{}
	service := NewNotificationService(provider, nil, nil, nil, "https://docs.example.test")
	title := "Contract"

	service.SendAccessLink(
		context.Background(),
		&entity.DocumentRecipient{Email: "alice@example.com", Name: "Alice"},
		&entity.Document{ID: "doc-1", Title: &title},
		"public-token",
		"en",
	)

	if provider.req == nil {
		t.Fatal("expected notification to be sent")
	}
	if !strings.Contains(provider.req.HTMLBody, "https://docs.example.test/public/sign/public-token?language=en") {
		t.Fatalf("expected email body to preserve language, got %q", provider.req.HTMLBody)
	}
}
