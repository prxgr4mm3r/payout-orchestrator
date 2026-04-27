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

	apiauth "github.com/prxgr4mm3r/payout-orchestrator/internal/apps/api/auth"
	fundingservice "github.com/prxgr4mm3r/payout-orchestrator/internal/apps/api/services/fundingsources"
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

func TestCreateFundingSourceCreatesForAuthenticatedClient(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC)
	expectedClientID := "2c97a4da-38a7-46a8-9205-6482d0cfc6fb"

	handler := NewFundingSourcesHandler(fakeFundingSourceCreator{
		create: func(_ context.Context, input fundingservice.CreateFundingSourceInput) (fundingservice.FundingSource, error) {
			if input.ClientID != expectedClientID {
				t.Fatalf("expected client id %s, got %s", expectedClientID, input.ClientID)
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

			return serviceFundingSource("efb98fe4-b75f-4f1d-b9c7-794e66da2abb", input.ClientID, input.Name, "acct_123", now), nil
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
		create: func(context.Context, fundingservice.CreateFundingSourceInput) (fundingservice.FundingSource, error) {
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
		create: func(context.Context, fundingservice.CreateFundingSourceInput) (fundingservice.FundingSource, error) {
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
		create: func(context.Context, fundingservice.CreateFundingSourceInput) (fundingservice.FundingSource, error) {
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
		create: func(context.Context, fundingservice.CreateFundingSourceInput) (fundingservice.FundingSource, error) {
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

func TestGetFundingSourceFetchesAuthenticatedClientSource(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC)
	expectedClientID := "2c97a4da-38a7-46a8-9205-6482d0cfc6fb"
	expectedSourceID := "efb98fe4-b75f-4f1d-b9c7-794e66da2abb"

	handler := NewFundingSourcesHandler(fakeFundingSourceCreator{
		get: func(_ context.Context, input fundingservice.GetFundingSourceInput) (fundingservice.FundingSource, error) {
			if input.ClientID != expectedClientID {
				t.Fatalf("expected client id %s, got %s", expectedClientID, input.ClientID)
			}
			if input.ID != expectedSourceID {
				t.Fatalf("expected source id %s, got %s", expectedSourceID, input.ID)
			}

			return serviceFundingSource(input.ID, input.ClientID, "Main account", "acct_123", now), nil
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/funding-sources/"+expectedSourceID, nil)
	req.SetPathValue("id", expectedSourceID)
	req = req.WithContext(apiauth.WithClient(req.Context(), apiauth.Client{
		ID:   expectedClientID,
		Name: "acme",
	}))
	rec := httptest.NewRecorder()

	handler.GetFundingSource(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var response fundingSourceResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.ID != expectedSourceID {
		t.Fatalf("expected source id %s, got %s", expectedSourceID, response.ID)
	}
	if response.ClientID != expectedClientID {
		t.Fatalf("expected client id %s, got %s", expectedClientID, response.ClientID)
	}
}

func TestGetFundingSourceMapsNotFound(t *testing.T) {
	t.Parallel()

	handler := NewFundingSourcesHandler(fakeFundingSourceCreator{
		get: func(context.Context, fundingservice.GetFundingSourceInput) (fundingservice.FundingSource, error) {
			return fundingservice.FundingSource{}, fundingservice.ErrFundingSourceNotFound
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/funding-sources/efb98fe4-b75f-4f1d-b9c7-794e66da2abb", nil)
	req.SetPathValue("id", "efb98fe4-b75f-4f1d-b9c7-794e66da2abb")
	req = req.WithContext(apiauth.WithClient(req.Context(), apiauth.Client{
		ID:   "2c97a4da-38a7-46a8-9205-6482d0cfc6fb",
		Name: "acme",
	}))
	rec := httptest.NewRecorder()

	handler.GetFundingSource(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected %d, got %d", http.StatusNotFound, rec.Code)
	}
}

func TestListFundingSourcesFetchesAuthenticatedClientSources(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC)
	expectedClientID := "2c97a4da-38a7-46a8-9205-6482d0cfc6fb"

	handler := NewFundingSourcesHandler(fakeFundingSourceCreator{
		list: func(_ context.Context, input fundingservice.ListFundingSourcesInput) ([]fundingservice.FundingSource, error) {
			if input.ClientID != expectedClientID {
				t.Fatalf("expected client id %s, got %s", expectedClientID, input.ClientID)
			}
			if input.Limit != 25 {
				t.Fatalf("expected limit 25, got %d", input.Limit)
			}
			if input.Offset != 10 {
				t.Fatalf("expected offset 10, got %d", input.Offset)
			}

			return []fundingservice.FundingSource{
				serviceFundingSource("efb98fe4-b75f-4f1d-b9c7-794e66da2abb", input.ClientID, "Main account", "acct_123", now),
			}, nil
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/funding-sources?limit=25&offset=10", nil)
	req = req.WithContext(apiauth.WithClient(req.Context(), apiauth.Client{
		ID:   expectedClientID,
		Name: "acme",
	}))
	rec := httptest.NewRecorder()

	handler.ListFundingSources(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var response []fundingSourceResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response) != 1 {
		t.Fatalf("expected 1 funding source, got %d", len(response))
	}
	if response[0].ClientID != expectedClientID {
		t.Fatalf("expected client id %s, got %s", expectedClientID, response[0].ClientID)
	}
}

func TestListFundingSourcesRejectsInvalidPagination(t *testing.T) {
	t.Parallel()

	handler := NewFundingSourcesHandler(fakeFundingSourceCreator{
		list: func(context.Context, fundingservice.ListFundingSourcesInput) ([]fundingservice.FundingSource, error) {
			t.Fatal("service should not be called for invalid pagination")
			return nil, nil
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/funding-sources?limit=0", nil)
	req = req.WithContext(apiauth.WithClient(req.Context(), apiauth.Client{
		ID:   "2c97a4da-38a7-46a8-9205-6482d0cfc6fb",
		Name: "acme",
	}))
	rec := httptest.NewRecorder()

	handler.ListFundingSources(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

func serviceFundingSource(id, clientID, name, paymentAccountID string, at time.Time) fundingservice.FundingSource {
	return fundingservice.FundingSource{
		ID:               id,
		ClientID:         clientID,
		Name:             name,
		Type:             "bank_account",
		PaymentAccountID: paymentAccountID,
		Status:           "active",
		CreatedAt:        at,
		UpdatedAt:        at,
	}
}
