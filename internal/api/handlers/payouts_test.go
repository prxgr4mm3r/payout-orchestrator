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
	payoutservice "github.com/prxgr4mm3r/payout-orchestrator/internal/api/services/payouts"
)

type fakePayoutReadService struct {
	create func(ctx context.Context, input payoutservice.CreatePayoutInput) (payoutservice.Payout, error)
	get    func(ctx context.Context, input payoutservice.GetPayoutInput) (payoutservice.Payout, error)
	list   func(ctx context.Context, input payoutservice.ListPayoutsInput) ([]payoutservice.Payout, error)
}

func (f fakePayoutReadService) CreatePayout(ctx context.Context, input payoutservice.CreatePayoutInput) (payoutservice.Payout, error) {
	return f.create(ctx, input)
}

func (f fakePayoutReadService) GetPayout(ctx context.Context, input payoutservice.GetPayoutInput) (payoutservice.Payout, error) {
	return f.get(ctx, input)
}

func (f fakePayoutReadService) ListPayouts(ctx context.Context, input payoutservice.ListPayoutsInput) ([]payoutservice.Payout, error) {
	return f.list(ctx, input)
}

func TestCreatePayoutCreatesForAuthenticatedClient(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC)
	expectedClientID := "2c97a4da-38a7-46a8-9205-6482d0cfc6fb"
	expectedFundingSourceID := "b76e34c6-d2da-45b1-a0c1-307bc76918bd"

	handler := NewPayoutsHandler(fakePayoutReadService{
		create: func(_ context.Context, input payoutservice.CreatePayoutInput) (payoutservice.Payout, error) {
			if input.ClientID != expectedClientID {
				t.Fatalf("expected client id %s, got %s", expectedClientID, input.ClientID)
			}
			if input.FundingSourceID != expectedFundingSourceID {
				t.Fatalf("expected funding source id %s, got %s", expectedFundingSourceID, input.FundingSourceID)
			}
			if input.Amount != "125.50" {
				t.Fatalf("expected amount 125.50, got %s", input.Amount)
			}
			if input.Currency != "USDC" {
				t.Fatalf("expected currency USDC, got %s", input.Currency)
			}
			if input.IdempotencyKey != "payout-1" {
				t.Fatalf("expected idempotency key payout-1, got %s", input.IdempotencyKey)
			}

			return servicePayout("efb98fe4-b75f-4f1d-b9c7-794e66da2abb", input.ClientID, "125.50", "USDC", "pending", "", now), nil
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/payouts", strings.NewReader(`{
		"funding_source_id": "b76e34c6-d2da-45b1-a0c1-307bc76918bd",
		"amount": "125.50",
		"currency": "USDC"
	}`))
	req.Header.Set(idempotencyKeyHeader, "payout-1")
	req = req.WithContext(apiauth.WithClient(req.Context(), apiauth.Client{
		ID:   expectedClientID,
		Name: "acme",
	}))
	rec := httptest.NewRecorder()

	handler.CreatePayout(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected %d, got %d: %s", http.StatusCreated, rec.Code, rec.Body.String())
	}

	var response payoutResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.ClientID != expectedClientID {
		t.Fatalf("expected client id %s, got %s", expectedClientID, response.ClientID)
	}
	if response.Status != "pending" {
		t.Fatalf("expected status pending, got %s", response.Status)
	}
}

func TestCreatePayoutMapsFundingSourceNotFound(t *testing.T) {
	t.Parallel()

	handler := NewPayoutsHandler(fakePayoutReadService{
		create: func(context.Context, payoutservice.CreatePayoutInput) (payoutservice.Payout, error) {
			return payoutservice.Payout{}, payoutservice.ErrFundingSourceNotFound
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/payouts", strings.NewReader(`{
		"funding_source_id": "b76e34c6-d2da-45b1-a0c1-307bc76918bd",
		"amount": "125.50",
		"currency": "USDC"
	}`))
	req.Header.Set(idempotencyKeyHeader, "payout-1")
	req = req.WithContext(apiauth.WithClient(req.Context(), apiauth.Client{
		ID:   "2c97a4da-38a7-46a8-9205-6482d0cfc6fb",
		Name: "acme",
	}))
	rec := httptest.NewRecorder()

	handler.CreatePayout(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected %d, got %d", http.StatusNotFound, rec.Code)
	}
}

func TestCreatePayoutMapsIdempotencyConflict(t *testing.T) {
	t.Parallel()

	handler := NewPayoutsHandler(fakePayoutReadService{
		create: func(context.Context, payoutservice.CreatePayoutInput) (payoutservice.Payout, error) {
			return payoutservice.Payout{}, payoutservice.ErrIdempotencyConflict
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/payouts", strings.NewReader(`{
		"funding_source_id": "b76e34c6-d2da-45b1-a0c1-307bc76918bd",
		"amount": "125.50",
		"currency": "USDC"
	}`))
	req.Header.Set(idempotencyKeyHeader, "payout-1")
	req = req.WithContext(apiauth.WithClient(req.Context(), apiauth.Client{
		ID:   "2c97a4da-38a7-46a8-9205-6482d0cfc6fb",
		Name: "acme",
	}))
	rec := httptest.NewRecorder()

	handler.CreatePayout(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected %d, got %d", http.StatusConflict, rec.Code)
	}
}

func TestCreatePayoutRejectsMissingIdempotencyKey(t *testing.T) {
	t.Parallel()

	handler := NewPayoutsHandler(fakePayoutReadService{
		create: func(context.Context, payoutservice.CreatePayoutInput) (payoutservice.Payout, error) {
			t.Fatal("service should not be called without idempotency key")
			return payoutservice.Payout{}, nil
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/payouts", strings.NewReader(`{
		"funding_source_id": "b76e34c6-d2da-45b1-a0c1-307bc76918bd",
		"amount": "125.50",
		"currency": "USDC"
	}`))
	req = req.WithContext(apiauth.WithClient(req.Context(), apiauth.Client{
		ID:   "2c97a4da-38a7-46a8-9205-6482d0cfc6fb",
		Name: "acme",
	}))
	rec := httptest.NewRecorder()

	handler.CreatePayout(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestCreatePayoutRejectsInvalidJSON(t *testing.T) {
	t.Parallel()

	handler := NewPayoutsHandler(fakePayoutReadService{
		create: func(context.Context, payoutservice.CreatePayoutInput) (payoutservice.Payout, error) {
			t.Fatal("service should not be called for invalid json")
			return payoutservice.Payout{}, nil
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/payouts", strings.NewReader(`{`))
	req.Header.Set(idempotencyKeyHeader, "payout-1")
	req = req.WithContext(apiauth.WithClient(req.Context(), apiauth.Client{
		ID:   "2c97a4da-38a7-46a8-9205-6482d0cfc6fb",
		Name: "acme",
	}))
	rec := httptest.NewRecorder()

	handler.CreatePayout(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestGetPayoutFetchesAuthenticatedClientPayout(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC)
	expectedClientID := "2c97a4da-38a7-46a8-9205-6482d0cfc6fb"
	expectedPayoutID := "efb98fe4-b75f-4f1d-b9c7-794e66da2abb"

	handler := NewPayoutsHandler(fakePayoutReadService{
		get: func(_ context.Context, input payoutservice.GetPayoutInput) (payoutservice.Payout, error) {
			if input.ClientID != expectedClientID {
				t.Fatalf("expected client id %s, got %s", expectedClientID, input.ClientID)
			}
			if input.ID != expectedPayoutID {
				t.Fatalf("expected payout id %s, got %s", expectedPayoutID, input.ID)
			}

			return servicePayout(input.ID, input.ClientID, "125.50", "USDC", "pending", "", now), nil
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/payouts/"+expectedPayoutID, nil)
	req.SetPathValue("id", expectedPayoutID)
	req = req.WithContext(apiauth.WithClient(req.Context(), apiauth.Client{
		ID:   expectedClientID,
		Name: "acme",
	}))
	rec := httptest.NewRecorder()

	handler.GetPayout(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var response payoutResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.ID != expectedPayoutID {
		t.Fatalf("expected payout id %s, got %s", expectedPayoutID, response.ID)
	}
	if response.ClientID != expectedClientID {
		t.Fatalf("expected client id %s, got %s", expectedClientID, response.ClientID)
	}
	if response.Amount != "125.50" {
		t.Fatalf("expected amount 125.50, got %s", response.Amount)
	}
}

func TestGetPayoutMapsNotFound(t *testing.T) {
	t.Parallel()

	handler := NewPayoutsHandler(fakePayoutReadService{
		get: func(context.Context, payoutservice.GetPayoutInput) (payoutservice.Payout, error) {
			return payoutservice.Payout{}, payoutservice.ErrPayoutNotFound
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/payouts/efb98fe4-b75f-4f1d-b9c7-794e66da2abb", nil)
	req.SetPathValue("id", "efb98fe4-b75f-4f1d-b9c7-794e66da2abb")
	req = req.WithContext(apiauth.WithClient(req.Context(), apiauth.Client{
		ID:   "2c97a4da-38a7-46a8-9205-6482d0cfc6fb",
		Name: "acme",
	}))
	rec := httptest.NewRecorder()

	handler.GetPayout(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected %d, got %d", http.StatusNotFound, rec.Code)
	}
}

func TestListPayoutsFetchesAuthenticatedClientPayouts(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC)
	expectedClientID := "2c97a4da-38a7-46a8-9205-6482d0cfc6fb"

	handler := NewPayoutsHandler(fakePayoutReadService{
		list: func(_ context.Context, input payoutservice.ListPayoutsInput) ([]payoutservice.Payout, error) {
			if input.ClientID != expectedClientID {
				t.Fatalf("expected client id %s, got %s", expectedClientID, input.ClientID)
			}
			if input.Limit != 25 {
				t.Fatalf("expected limit 25, got %d", input.Limit)
			}
			if input.Offset != 10 {
				t.Fatalf("expected offset 10, got %d", input.Offset)
			}

			return []payoutservice.Payout{
				servicePayout("efb98fe4-b75f-4f1d-b9c7-794e66da2abb", input.ClientID, "125.50", "USDC", "pending", "", now),
			}, nil
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/payouts?limit=25&offset=10", nil)
	req = req.WithContext(apiauth.WithClient(req.Context(), apiauth.Client{
		ID:   expectedClientID,
		Name: "acme",
	}))
	rec := httptest.NewRecorder()

	handler.ListPayouts(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var response []payoutResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response) != 1 {
		t.Fatalf("expected 1 payout, got %d", len(response))
	}
	if response[0].ClientID != expectedClientID {
		t.Fatalf("expected client id %s, got %s", expectedClientID, response[0].ClientID)
	}
}

func TestGetPayoutIncludesFailureReason(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC)
	handler := NewPayoutsHandler(fakePayoutReadService{
		get: func(_ context.Context, input payoutservice.GetPayoutInput) (payoutservice.Payout, error) {
			return servicePayout(input.ID, input.ClientID, "125.50", "USDC", "failed", "provider rejected payout", now), nil
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/payouts/efb98fe4-b75f-4f1d-b9c7-794e66da2abb", nil)
	req.SetPathValue("id", "efb98fe4-b75f-4f1d-b9c7-794e66da2abb")
	req = req.WithContext(apiauth.WithClient(req.Context(), apiauth.Client{
		ID:   "2c97a4da-38a7-46a8-9205-6482d0cfc6fb",
		Name: "acme",
	}))
	rec := httptest.NewRecorder()

	handler.GetPayout(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var response payoutResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.FailureReason != "provider rejected payout" {
		t.Fatalf("expected failure reason to be returned, got %q", response.FailureReason)
	}
}

func TestListPayoutsRejectsInvalidPagination(t *testing.T) {
	t.Parallel()

	handler := NewPayoutsHandler(fakePayoutReadService{
		list: func(context.Context, payoutservice.ListPayoutsInput) ([]payoutservice.Payout, error) {
			t.Fatal("service should not be called for invalid pagination")
			return nil, nil
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/payouts?limit=0", nil)
	req = req.WithContext(apiauth.WithClient(req.Context(), apiauth.Client{
		ID:   "2c97a4da-38a7-46a8-9205-6482d0cfc6fb",
		Name: "acme",
	}))
	rec := httptest.NewRecorder()

	handler.ListPayouts(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestGetPayoutMapsUnexpectedErrors(t *testing.T) {
	t.Parallel()

	handler := NewPayoutsHandler(fakePayoutReadService{
		get: func(context.Context, payoutservice.GetPayoutInput) (payoutservice.Payout, error) {
			return payoutservice.Payout{}, errors.New("database unavailable")
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/payouts/efb98fe4-b75f-4f1d-b9c7-794e66da2abb", nil)
	req.SetPathValue("id", "efb98fe4-b75f-4f1d-b9c7-794e66da2abb")
	req = req.WithContext(apiauth.WithClient(req.Context(), apiauth.Client{
		ID:   "2c97a4da-38a7-46a8-9205-6482d0cfc6fb",
		Name: "acme",
	}))
	rec := httptest.NewRecorder()

	handler.GetPayout(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected %d, got %d", http.StatusInternalServerError, rec.Code)
	}
}

func servicePayout(id, clientID, amount, currency, status, failureReason string, at time.Time) payoutservice.Payout {
	return payoutservice.Payout{
		ID:              id,
		ClientID:        clientID,
		FundingSourceID: "b76e34c6-d2da-45b1-a0c1-307bc76918bd",
		Amount:          amount,
		Currency:        currency,
		Status:          status,
		FailureReason:   failureReason,
		CreatedAt:       at,
		UpdatedAt:       at,
	}
}
