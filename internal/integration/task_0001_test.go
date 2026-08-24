package integration_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/vance1852/community-family-doctor/internal/auth"
	"github.com/vance1852/community-family-doctor/internal/domain"
)

func TestExpiredSessionIsRejectedAfterRestartBoundary(t *testing.T) {
	f := newFixture(t)
	login, err := f.auth.Login(context.Background(), auth.LoginCommand{
		OrganizationID: "org-1", Email: "field@example.test", Password: "correct-horse-battery-staple",
	})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	expired := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)
	if _, err := f.store.DB().ExecContext(context.Background(), "UPDATE sessions SET expires_at = ? WHERE user_id = 'field'", expired); err != nil {
		t.Fatalf("expire session: %v", err)
	}
	if _, err := f.auth.Authenticate(context.Background(), login.Token); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("expired token was accepted: %v", err)
	}
	second, err := f.auth.Login(context.Background(), auth.LoginCommand{
		OrganizationID: "org-1", Email: "supervisor@example.test", Password: "correct-horse-battery-staple",
	})
	if err != nil {
		t.Fatalf("fresh login: %v", err)
	}
	if _, err := f.auth.Authenticate(context.Background(), second.Token); err != nil {
		t.Fatalf("fresh session was rejected: %v", err)
	}
}
