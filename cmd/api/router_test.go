package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apiauth "github.com/prxgr4mm3r/payout-orchestrator/internal/api/auth"
	"github.com/prxgr4mm3r/payout-orchestrator/internal/api/handlers"
	fundingservice "github.com/prxgr4mm3r/payout-orchestrator/internal/api/services/fundingsources"
)

type fakeFundingSourceCreator struct {
	create func(ctx context.Context, input fundingservice.CreateFundingSourceInput) (fundingservice.FundingSource, error)
	get    func(ctx context.Context, input fundingservice.GetFundingSourceInput) (fundingservice.FundingSource, error)
	list   func(ctx context.Context, input fundingservice.ListFundingSourcesInput) ([]fundingservice.FundingSource, error)
}

func (f fakeFundingSourceCreator) CreateFundingSource(ctx context.Context, input fundingservice.CreateFundingSourceInput) (fundingservice.FundingSource, error) {
	return f.create(ctx, input)
}

func (f fakeFundingSourceCreator) GetFundingSource(ctx context.Context, input fundingservice.GetFundingSourceInput) (fundingservice.FundingSource, error) {
	return f.get(ctx, input)
}

func (f fakeFundingSourceCreator) ListFundingSources(ctx context.Context, input fundingservice.ListFundingSourcesInput) ([]fundingservice.FundingSource, error) {
	return f.list(ctx, input)
}

func TestNewRouterLeavesHealthzPublic(t *testing.T) {
	t.Parallel()

	authCalled := false
	router := NewRouter(&handlers.ClientsHandler{}, handlers.NewFundingSourcesHandler(fakeFundingSourceCreator{}), func(next http.Handler) http.Handler {
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
	router := NewRouter(&handlers.ClientsHandler{}, handlers.NewFundingSourcesHandler(fakeFundingSourceCreator{}), func(next http.Handler) http.Handler {
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

func TestNewRouterProtectsFundingSourceRoutes(t *testing.T) {
	t.Parallel()

	expectedClient := apiauth.Client{
		ID:   "2c97a4da-38a7-46a8-9205-6482d0cfc6fb",
		Name: "acme",
	}

	authCalled := false
	serviceCalled := false
	router := NewRouter(&handlers.ClientsHandler{}, handlers.NewFundingSourcesHandler(fakeFundingSourceCreator{
		create: func(_ context.Context, input fundingservice.CreateFundingSourceInput) (fundingservice.FundingSource, error) {
			serviceCalled = true
			if input.ClientID != expectedClient.ID {
				t.Fatalf("expected client id %s, got %s", expectedClient.ID, input.ClientID)
			}
			if input.Name != "Main account" {
				t.Fatalf("expected funding source name, got %q", input.Name)
			}

			return fundingservice.FundingSource{}, nil
		},
	}), func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authCalled = true
			ctx := apiauth.WithClient(r.Context(), expectedClient)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})

	req := httptest.NewRequest(http.MethodPost, "/funding-sources", strings.NewReader(`{
		"name": "Main account",
		"type": "bank_account",
		"payment_account_id": "acct_123"
	}`))
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected %d, got %d", http.StatusCreated, rec.Code)
	}
	if !authCalled {
		t.Fatal("expected funding source route to pass through auth middleware")
	}
	if !serviceCalled {
		t.Fatal("expected funding source handler to call service")
	}
}

func TestNewRouterProtectsFundingSourceReadRoutes(t *testing.T) {
	t.Parallel()

	expectedClient := apiauth.Client{
		ID:   "2c97a4da-38a7-46a8-9205-6482d0cfc6fb",
		Name: "acme",
	}
	expectedSourceID := "efb98fe4-b75f-4f1d-b9c7-794e66da2abb"

	authCalled := false
	getCalled := false
	listCalled := false
	router := NewRouter(&handlers.ClientsHandler{}, handlers.NewFundingSourcesHandler(fakeFundingSourceCreator{
		get: func(_ context.Context, input fundingservice.GetFundingSourceInput) (fundingservice.FundingSource, error) {
			getCalled = true
			if input.ClientID != expectedClient.ID {
				t.Fatalf("expected client id %s, got %s", expectedClient.ID, input.ClientID)
			}
			if input.ID != expectedSourceID {
				t.Fatalf("expected source id %s, got %s", expectedSourceID, input.ID)
			}

			return fundingservice.FundingSource{}, nil
		},
		list: func(_ context.Context, input fundingservice.ListFundingSourcesInput) ([]fundingservice.FundingSource, error) {
			listCalled = true
			if input.ClientID != expectedClient.ID {
				t.Fatalf("expected client id %s, got %s", expectedClient.ID, input.ClientID)
			}

			return nil, nil
		},
	}), func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authCalled = true
			ctx := apiauth.WithClient(r.Context(), expectedClient)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})

	listReq := httptest.NewRequest(http.MethodGet, "/funding-sources", nil)
	listRec := httptest.NewRecorder()
	router.ServeHTTP(listRec, listReq)

	if listRec.Code != http.StatusOK {
		t.Fatalf("expected list status %d, got %d", http.StatusOK, listRec.Code)
	}
	if !listCalled {
		t.Fatal("expected list handler to call service")
	}

	getReq := httptest.NewRequest(http.MethodGet, "/funding-sources/"+expectedSourceID, nil)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("expected get status %d, got %d", http.StatusOK, getRec.Code)
	}
	if !getCalled {
		t.Fatal("expected get handler to call service")
	}
	if !authCalled {
		t.Fatal("expected funding source read routes to pass through auth middleware")
	}
}
