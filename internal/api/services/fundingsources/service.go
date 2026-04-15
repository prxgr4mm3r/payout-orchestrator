package fundingsources

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/prxgr4mm3r/payout-orchestrator/internal/db"
)

var (
	ErrInvalidClientID      = errors.New("invalid client id")
	ErrInvalidFundingSource = errors.New("invalid funding source")
)

type FundingSourceStore interface {
	CreateFundingSource(ctx context.Context, arg db.CreateFundingSourceParams) (db.FundingSource, error)
}

type Service struct {
	store FundingSourceStore
}

type CreateFundingSourceInput struct {
	ClientID         string
	Name             string
	Type             string
	PaymentAccountID string
}

type FundingSource struct {
	ID               string
	ClientID         string
	Name             string
	Type             string
	PaymentAccountID string
	Status           string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func NewService(store FundingSourceStore) *Service {
	return &Service{store: store}
}

func (s *Service) CreateFundingSource(ctx context.Context, input CreateFundingSourceInput) (FundingSource, error) {
	if s == nil || s.store == nil {
		return FundingSource{}, errors.New("funding source service is not configured")
	}

	name := strings.TrimSpace(input.Name)
	sourceType := strings.TrimSpace(input.Type)
	paymentAccountID := strings.TrimSpace(input.PaymentAccountID)
	if name == "" || sourceType == "" || paymentAccountID == "" {
		return FundingSource{}, ErrInvalidFundingSource
	}

	var parsedClientID pgtype.UUID
	if err := parsedClientID.Scan(input.ClientID); err != nil {
		return FundingSource{}, ErrInvalidClientID
	}

	source, err := s.store.CreateFundingSource(ctx, db.CreateFundingSourceParams{
		ClientID:         parsedClientID,
		Name:             name,
		Type:             sourceType,
		PaymentAccountID: paymentAccountID,
	})
	if err != nil {
		return FundingSource{}, err
	}

	return FundingSource{
		ID:               source.ID.String(),
		ClientID:         source.ClientID.String(),
		Name:             source.Name,
		Type:             source.Type,
		PaymentAccountID: source.PaymentAccountID,
		Status:           source.Status,
		CreatedAt:        source.CreatedAt.Time,
		UpdatedAt:        source.UpdatedAt.Time,
	}, nil
}
