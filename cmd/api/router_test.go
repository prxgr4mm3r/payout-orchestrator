package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	apiauth "github.com/prxgr4mm3r/payout-orchestrator/internal/api/auth"
	"github.com/prxgr4mm3r/payout-orchestrator/internal/api/handlers"
)

func TestNewRouterLeavesHealthzPublic(t *testing.T) {
	t.Parallel()

	authCalled := false
	router := NewRouter(&handlers.ClientsHandler{}, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authCalled = true
			next.ServeHTTP(w, r)
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d", http.StatusOK, rec.Code)
	}

	if authCalled {
		t.Fatal("expected healthz to bypass auth middleware")
	}
}

func TestNewRouterProtectsClientRoutes(t *testing.T) {
	t.Parallel()

	expectedClient := apiauth.Client{
		ID:   "2c97a4da-38a7-46a8-9205-6482d0cfc6fb",
		Name: "acme",
	}

	authCalled := false
	router := NewRouter(&handlers.ClientsHandler{}, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authCalled = true
			ctx := apiauth.WithClient(r.Context(), expectedClient)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/clients/me", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d", http.StatusOK, rec.Code)
	}

	if !authCalled {
		t.Fatal("expected clients route to pass through auth middleware")
	}
}
