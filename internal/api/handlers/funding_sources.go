package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	apiauth "github.com/prxgr4mm3r/payout-orchestrator/internal/api/auth"
	fundingservice "github.com/prxgr4mm3r/payout-orchestrator/internal/api/services/fundingsources"
)

type FundingSourceCreator interface {
	CreateFundingSource(ctx context.Context, clientID string, input fundingservice.CreateFundingSourceInput) (fundingservice.FundingSource, error)
}

type FundingSourcesHandler struct {
	creator FundingSourceCreator
}

type createFundingSourceRequest struct {
	Name             string `json:"name"`
	Type             string `json:"type"`
	PaymentAccountID string `json:"payment_account_id"`
}

type fundingSourceResponse struct {
	ID               string `json:"id"`
	ClientID         string `json:"client_id"`
	Name             string `json:"name"`
	Type             string `json:"type"`
	PaymentAccountID string `json:"payment_account_id"`
	Status           string `json:"status"`
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
}

func NewFundingSourcesHandler(creator FundingSourceCreator) *FundingSourcesHandler {
	return &FundingSourcesHandler{creator: creator}
}

func (h FundingSourcesHandler) CreateFundingSource(w http.ResponseWriter, r *http.Request) {
	if h.creator == nil {
		http.Error(w, "funding source handler is not configured", http.StatusInternalServerError)
		return
	}

	client, ok := apiauth.ClientFromContext(r.Context())
	if !ok {
		http.Error(w, "client not found in context", http.StatusInternalServerError)
		return
	}

	var req createFundingSourceRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	source, err := h.creator.CreateFundingSource(r.Context(), client.ID, fundingservice.CreateFundingSourceInput{
		Name:             req.Name,
		Type:             req.Type,
		PaymentAccountID: req.PaymentAccountID,
	})
	if err != nil {
		switch {
		case errors.Is(err, fundingservice.ErrInvalidFundingSource):
			http.Error(w, "invalid funding source", http.StatusBadRequest)
		case errors.Is(err, fundingservice.ErrInvalidClientID):
			http.Error(w, "client not found in context", http.StatusInternalServerError)
		default:
			http.Error(w, "internal server error", http.StatusInternalServerError)
		}

		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(fundingSourceResponse{
		ID:               source.ID,
		ClientID:         source.ClientID,
		Name:             source.Name,
		Type:             source.Type,
		PaymentAccountID: source.PaymentAccountID,
		Status:           source.Status,
		CreatedAt:        source.CreatedAt.Format(time.RFC3339Nano),
		UpdatedAt:        source.UpdatedAt.Format(time.RFC3339Nano),
	}); err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
	}
}
