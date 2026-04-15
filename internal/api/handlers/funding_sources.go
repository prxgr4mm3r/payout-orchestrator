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

const (
	defaultFundingSourceListLimit = int32(50)
	maxFundingSourceListLimit     = int32(100)
)

type FundingSourceService interface {
	CreateFundingSource(ctx context.Context, input fundingservice.CreateFundingSourceInput) (fundingservice.FundingSource, error)
	GetFundingSource(ctx context.Context, input fundingservice.GetFundingSourceInput) (fundingservice.FundingSource, error)
	ListFundingSources(ctx context.Context, input fundingservice.ListFundingSourcesInput) ([]fundingservice.FundingSource, error)
}

type FundingSourcesHandler struct {
	service FundingSourceService
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

func NewFundingSourcesHandler(service FundingSourceService) *FundingSourcesHandler {
	return &FundingSourcesHandler{service: service}
}

func (h FundingSourcesHandler) CreateFundingSource(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
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

	source, err := h.service.CreateFundingSource(r.Context(), fundingservice.CreateFundingSourceInput{
		ClientID:         client.ID,
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
	if err := json.NewEncoder(w).Encode(fundingSourceResponseFromService(source)); err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
	}
}

func (h FundingSourcesHandler) GetFundingSource(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		http.Error(w, "funding source handler is not configured", http.StatusInternalServerError)
		return
	}

	client, ok := apiauth.ClientFromContext(r.Context())
	if !ok {
		http.Error(w, "client not found in context", http.StatusInternalServerError)
		return
	}

	source, err := h.service.GetFundingSource(r.Context(), fundingservice.GetFundingSourceInput{
		ClientID: client.ID,
		ID:       r.PathValue("id"),
	})
	if err != nil {
		switch {
		case errors.Is(err, fundingservice.ErrFundingSourceNotFound):
			http.Error(w, "funding source not found", http.StatusNotFound)
		case errors.Is(err, fundingservice.ErrInvalidSourceID):
			http.Error(w, "invalid funding source id", http.StatusBadRequest)
		case errors.Is(err, fundingservice.ErrInvalidClientID):
			http.Error(w, "client not found in context", http.StatusInternalServerError)
		default:
			http.Error(w, "internal server error", http.StatusInternalServerError)
		}

		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(fundingSourceResponseFromService(source)); err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
	}
}

func (h FundingSourcesHandler) ListFundingSources(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		http.Error(w, "funding source handler is not configured", http.StatusInternalServerError)
		return
	}

	client, ok := apiauth.ClientFromContext(r.Context())
	if !ok {
		http.Error(w, "client not found in context", http.StatusInternalServerError)
		return
	}

	limit, offset, err := fundingSourcePagination(r)
	if err != nil {
		http.Error(w, "invalid pagination", http.StatusBadRequest)
		return
	}

	sources, err := h.service.ListFundingSources(r.Context(), fundingservice.ListFundingSourcesInput{
		ClientID: client.ID,
		Limit:    limit,
		Offset:   offset,
	})
	if err != nil {
		switch {
		case errors.Is(err, fundingservice.ErrInvalidPagination):
			http.Error(w, "invalid pagination", http.StatusBadRequest)
		case errors.Is(err, fundingservice.ErrInvalidClientID):
			http.Error(w, "client not found in context", http.StatusInternalServerError)
		default:
			http.Error(w, "internal server error", http.StatusInternalServerError)
		}

		return
	}

	response := make([]fundingSourceResponse, 0, len(sources))
	for _, source := range sources {
		response = append(response, fundingSourceResponseFromService(source))
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
	}
}

func fundingSourcePagination(r *http.Request) (int32, int32, error) {
	limit, offset, err := paginationParams(r, defaultFundingSourceListLimit, maxFundingSourceListLimit)
	if err != nil {
		return 0, 0, fundingservice.ErrInvalidPagination
	}

	return limit, offset, nil
}

func fundingSourceResponseFromService(source fundingservice.FundingSource) fundingSourceResponse {
	return fundingSourceResponse{
		ID:               source.ID,
		ClientID:         source.ClientID,
		Name:             source.Name,
		Type:             source.Type,
		PaymentAccountID: source.PaymentAccountID,
		Status:           source.Status,
		CreatedAt:        source.CreatedAt.Format(time.RFC3339Nano),
		UpdatedAt:        source.UpdatedAt.Format(time.RFC3339Nano),
	}
}
