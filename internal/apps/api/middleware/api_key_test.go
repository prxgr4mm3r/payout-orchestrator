package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	apiauth "github.com/prxgr4mm3r/payout-orchestrator/internal/apps/api/auth"
	authservice "github.com/prxgr4mm3r/payout-orchestrator/internal/apps/api/services/auth"
)

type fakeAuthenticator struct {
	authenticate func(ctx context.Context, rawAPIKey string) (apiauth.Client, error)
}

func (f fakeAuthenticator) AuthenticateAPIKey(ctx context.Context, rawAPIKey string) (apiauth.Client, error) {
	return f.authenticate(ctx, rawAPIKey)
}

func TestAPIKeyRejectsMissingHeader(t *testing.T) {
	t.Parallel()

	middleware := APIKey(fakeAuthenticator{
		authenticate: func(_ context.Context, _ string) (apiauth.Client, error) {
			t.Fatal("authenticator should not be called when header is missing")
			return apiauth.Client{}, nil
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/clients/me", nil)
	rec := httptest.NewRecorder()

	middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("next handler should not be called")
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected %d, got %d", http.StatusUnauthorized, rec.Code)
	}
}

func TestAPIKeyRejectsUnauthorizedClient(t *testing.T) {
	t.Parallel()

	middleware := APIKey(fakeAuthenticator{
		authenticate: func(_ context.Context, _ string) (apiauth.Client, error) {
			return apiauth.Client{}, authservice.ErrUnauthorized
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/clients/me", nil)
	req.Header.Set(apiKeyHeader, "c7d8c5f8-f7cc-426a-b533-59f486bbf5ce")
	rec := httptest.NewRecorder()

	middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("next handler should not be called")
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected %d, got %d", http.StatusUnauthorized, rec.Code)
	}
}

func TestAPIKeyInjectsClientIntoContext(t *testing.T) {
	t.Parallel()

	expected := apiauth.Client{
		ID:   "8d8459db-985e-46ba-92e1-f80827966720",
		Name: "acme",
	}

	middleware := APIKey(fakeAuthenticator{
		authenticate: func(_ context.Context, rawAPIKey string) (apiauth.Client, error) {
			if rawAPIKey == "" {
				return apiauth.Client{}, errors.New("missing api key")
			}

			return expected, nil
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/clients/me", nil)
	req.Header.Set(apiKeyHeader, "c7d8c5f8-f7cc-426a-b533-59f486bbf5ce")
	rec := httptest.NewRecorder()

	middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		client, ok := apiauth.ClientFromContext(r.Context())
		if !ok {
			t.Fatal("expected client in request context")
		}

		if client != expected {
			t.Fatalf("expected %#v, got %#v", expected, client)
		}

		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected %d, got %d", http.StatusNoContent, rec.Code)
	}
}
