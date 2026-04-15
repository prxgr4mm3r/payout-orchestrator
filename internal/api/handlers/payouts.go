package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	apiauth "github.com/prxgr4mm3r/payout-orchestrator/internal/api/auth"
	payoutservice "github.com/prxgr4mm3r/payout-orchestrator/internal/api/services/payouts"
)

const (
	defaultPayoutListLimit = int32(50)
	maxPayoutListLimit     = int32(100)
)

type PayoutReadService interface {
	GetPayout(ctx context.Context, input payoutservice.GetPayoutInput) (payoutservice.Payout, error)
	ListPayouts(ctx context.Context, input payoutservice.ListPayoutsInput) ([]payoutservice.Payout, error)
}

type PayoutsHandler struct {
	service PayoutReadService
}

type payoutResponse struct {
	ID              string `json:"id"`
	ClientID        string `json:"client_id"`
	FundingSourceID string `json:"funding_source_id"`
	Amount          string `json:"amount"`
	Currency        string `json:"currency"`
	Status          string `json:"status"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
}

func NewPayoutsHandler(service PayoutReadService) *PayoutsHandler {
	return &PayoutsHandler{service: service}
}

func (h PayoutsHandler) GetPayout(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		http.Error(w, "payout handler is not configured", http.StatusInternalServerError)
		return
	}

	client, ok := apiauth.ClientFromContext(r.Context())
	if !ok {
		http.Error(w, "client not found in context", http.StatusInternalServerError)
		return
	}

	payout, err := h.service.GetPayout(r.Context(), payoutservice.GetPayoutInput{
		ClientID: client.ID,
		ID:       r.PathValue("id"),
	})
	if err != nil {
		switch {
		case errors.Is(err, payoutservice.ErrPayoutNotFound):
			http.Error(w, "payout not found", http.StatusNotFound)
		case errors.Is(err, payoutservice.ErrInvalidPayoutID):
			http.Error(w, "invalid payout id", http.StatusBadRequest)
		case errors.Is(err, payoutservice.ErrInvalidClientID):
			http.Error(w, "client not found in context", http.StatusInternalServerError)
		default:
			http.Error(w, "internal server error", http.StatusInternalServerError)
		}

		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(payoutResponseFromService(payout)); err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
	}
}

func (h PayoutsHandler) ListPayouts(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		http.Error(w, "payout handler is not configured", http.StatusInternalServerError)
		return
	}

	client, ok := apiauth.ClientFromContext(r.Context())
	if !ok {
		http.Error(w, "client not found in context", http.StatusInternalServerError)
		return
	}

	limit, offset, err := payoutPagination(r)
	if err != nil {
		http.Error(w, "invalid pagination", http.StatusBadRequest)
		return
	}

	payouts, err := h.service.ListPayouts(r.Context(), payoutservice.ListPayoutsInput{
		ClientID: client.ID,
		Limit:    limit,
		Offset:   offset,
	})
	if err != nil {
		switch {
		case errors.Is(err, payoutservice.ErrInvalidPagination):
			http.Error(w, "invalid pagination", http.StatusBadRequest)
		case errors.Is(err, payoutservice.ErrInvalidClientID):
			http.Error(w, "client not found in context", http.StatusInternalServerError)
		default:
			http.Error(w, "internal server error", http.StatusInternalServerError)
		}

		return
	}

	response := make([]payoutResponse, 0, len(payouts))
	for _, payout := range payouts {
		response = append(response, payoutResponseFromService(payout))
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
	}
}

func payoutPagination(r *http.Request) (int32, int32, error) {
	limit, err := int32QueryParam(r, "limit", defaultPayoutListLimit)
	if err != nil {
		return 0, 0, err
	}
	if limit <= 0 || limit > maxPayoutListLimit {
		return 0, 0, payoutservice.ErrInvalidPagination
	}

	offset, err := int32QueryParam(r, "offset", 0)
	if err != nil {
		return 0, 0, err
	}
	if offset < 0 {
		return 0, 0, payoutservice.ErrInvalidPagination
	}

	return limit, offset, nil
}

func payoutResponseFromService(payout payoutservice.Payout) payoutResponse {
	return payoutResponse{
		ID:              payout.ID,
		ClientID:        payout.ClientID,
		FundingSourceID: payout.FundingSourceID,
		Amount:          payout.Amount,
		Currency:        payout.Currency,
		Status:          payout.Status,
		CreatedAt:       payout.CreatedAt.Format(time.RFC3339Nano),
		UpdatedAt:       payout.UpdatedAt.Format(time.RFC3339Nano),
	}
}
