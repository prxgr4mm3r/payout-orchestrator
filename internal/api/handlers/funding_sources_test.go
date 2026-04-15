package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	apiauth "github.com/prxgr4mm3r/payout-orchestrator/internal/api/auth"
	fundingservice "github.com/prxgr4mm3r/payout-orchestrator/internal/api/services/fundingsources"
)

type fakeFundingSourceCreator struct {
	create func(ctx context.Context, clientID string, input fundingservice.CreateFundingSourceInput) (fundingservice.FundingSource, error)
}

func (f fakeFundingSourceCreator) CreateFundingSource(ctx context.Context, clientID string, input fundingservice.CreateFundingSourceInput) (fundingservice.FundingSource, error) {
	return f.create(ctx, clientID, input)
}

func TestCreateFundingSourceCreatesForAuthenticatedClient(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC)
	expectedClientID := "2c97a4da-38a7-46a8-9205-6482d0cfc6fb"

	handler := NewFundingSourcesHandler(fakeFundingSourceCreator{
		create: func(_ context.Context, clientID string, input fundingservice.CreateFundingSourceInput) (fundingservice.FundingSource, error) {
			if clientID != expectedClientID {
				t.Fatalf("expected client id %s, got %s", expectedClientID, clientID)
			}
			if input.Name != "Main account" {
				t.Fatalf("expected name Main account, got %q", input.Name)
			}
			if input.Type != "bank_account" {
				t.Fatalf("expected type bank_account, got %q", input.Type)
			}
			if input.PaymentAccountID != "acct_123" {
				t.Fatalf("expected payment account id acct_123, got %q", input.PaymentAccountID)
			}

			return fundingservice.FundingSource{
				ID:               "efb98fe4-b75f-4f1d-b9c7-794e66da2abb",
				ClientID:         clientID,
				Name:             input.Name,
				Type:             input.Type,
				PaymentAccountID: input.PaymentAccountID,
				Status:           "active",
				CreatedAt:        now,
				UpdatedAt:        now,
			}, nil
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/funding-sources", strings.NewReader(`{
		"name": "Main account",
		"type": "bank_account",
		"payment_account_id": "acct_123"
	}`))
	req = req.WithContext(apiauth.WithClient(req.Context(), apiauth.Client{
		ID:   expectedClientID,
		Name: "acme",
	}))
	rec := httptest.NewRecorder()

	handler.CreateFundingSource(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected %d, got %d: %s", http.StatusCreated, rec.Code, rec.Body.String())
	}
	if contentType := rec.Header().Get("Content-Type"); contentType != "application/json" {
		t.Fatalf("expected application/json, got %q", contentType)
	}

	var response fundingSourceResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.ClientID != expectedClientID {
		t.Fatalf("expected client id %s, got %s", expectedClientID, response.ClientID)
	}
	if response.Status != "active" {
		t.Fatalf("expected status active, got %q", response.Status)
	}
	if response.CreatedAt != now.Format(time.RFC3339Nano) {
		t.Fatalf("expected created at %s, got %s", now.Format(time.RFC3339Nano), response.CreatedAt)
	}
}

func TestCreateFundingSourceRejectsInvalidJSON(t *testing.T) {
	t.Parallel()

	handler := NewFundingSourcesHandler(fakeFundingSourceCreator{
		create: func(context.Context, string, fundingservice.CreateFundingSourceInput) (fundingservice.FundingSource, error) {
			t.Fatal("service should not be called for invalid json")
			return fundingservice.FundingSource{}, nil
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/funding-sources", strings.NewReader(`{`))
	req = req.WithContext(apiauth.WithClient(req.Context(), apiauth.Client{
		ID:   "2c97a4da-38a7-46a8-9205-6482d0cfc6fb",
		Name: "acme",
	}))
	rec := httptest.NewRecorder()

	handler.CreateFundingSource(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestCreateFundingSourceMapsValidationErrors(t *testing.T) {
	t.Parallel()

	handler := NewFundingSourcesHandler(fakeFundingSourceCreator{
		create: func(context.Context, string, fundingservice.CreateFundingSourceInput) (fundingservice.FundingSource, error) {
			return fundingservice.FundingSource{}, fundingservice.ErrInvalidFundingSource
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/funding-sources", strings.NewReader(`{
		"name": "",
		"type": "bank_account",
		"payment_account_id": "acct_123"
	}`))
	req = req.WithContext(apiauth.WithClient(req.Context(), apiauth.Client{
		ID:   "2c97a4da-38a7-46a8-9205-6482d0cfc6fb",
		Name: "acme",
	}))
	rec := httptest.NewRecorder()

	handler.CreateFundingSource(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestCreateFundingSourceRequiresAuthenticatedClient(t *testing.T) {
	t.Parallel()

	handler := NewFundingSourcesHandler(fakeFundingSourceCreator{
		create: func(context.Context, string, fundingservice.CreateFundingSourceInput) (fundingservice.FundingSource, error) {
			t.Fatal("service should not be called without authenticated client")
			return fundingservice.FundingSource{}, nil
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/funding-sources", strings.NewReader(`{
		"name": "Main account",
		"type": "bank_account",
		"payment_account_id": "acct_123"
	}`))
	rec := httptest.NewRecorder()

	handler.CreateFundingSource(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected %d, got %d", http.StatusInternalServerError, rec.Code)
	}
}

func TestCreateFundingSourceMapsUnexpectedErrors(t *testing.T) {
	t.Parallel()

	handler := NewFundingSourcesHandler(fakeFundingSourceCreator{
		create: func(context.Context, string, fundingservice.CreateFundingSourceInput) (fundingservice.FundingSource, error) {
			return fundingservice.FundingSource{}, errors.New("database unavailable")
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/funding-sources", strings.NewReader(`{
		"name": "Main account",
		"type": "bank_account",
		"payment_account_id": "acct_123"
	}`))
	req = req.WithContext(apiauth.WithClient(req.Context(), apiauth.Client{
		ID:   "2c97a4da-38a7-46a8-9205-6482d0cfc6fb",
		Name: "acme",
	}))
	rec := httptest.NewRecorder()

	handler.CreateFundingSource(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected %d, got %d", http.StatusInternalServerError, rec.Code)
	}
}
