package fundingsources

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/prxgr4mm3r/payout-orchestrator/internal/db"
)

var (
	ErrInvalidClientID       = errors.New("invalid client id")
	ErrInvalidFundingSource  = errors.New("invalid funding source")
	ErrInvalidPagination     = errors.New("invalid pagination")
	ErrInvalidSourceID       = errors.New("invalid funding source id")
	ErrFundingSourceNotFound = errors.New("funding source not found")
)

type FundingSourceStore interface {
	CreateFundingSource(ctx context.Context, arg db.CreateFundingSourceParams) (db.FundingSource, error)
	GetFundingSourceByClientID(ctx context.Context, arg db.GetFundingSourceByClientIDParams) (db.FundingSource, error)
	ListFundingSourcesByClientID(ctx context.Context, arg db.ListFundingSourcesByClientIDParams) ([]db.FundingSource, error)
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

type GetFundingSourceInput struct {
	ClientID string
	ID       string
}

type ListFundingSourcesInput struct {
	ClientID string
	Limit    int32
	Offset   int32
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

	return fundingSourceFromDB(source), nil
}

func (s *Service) GetFundingSource(ctx context.Context, input GetFundingSourceInput) (FundingSource, error) {
	if s == nil || s.store == nil {
		return FundingSource{}, errors.New("funding source service is not configured")
	}

	clientID, err := parseUUID(input.ClientID, ErrInvalidClientID)
	if err != nil {
		return FundingSource{}, err
	}

	sourceID, err := parseUUID(input.ID, ErrInvalidSourceID)
	if err != nil {
		return FundingSource{}, err
	}

	source, err := s.store.GetFundingSourceByClientID(ctx, db.GetFundingSourceByClientIDParams{
		ClientID: clientID,
		ID:       sourceID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return FundingSource{}, ErrFundingSourceNotFound
		}

		return FundingSource{}, err
	}

	return fundingSourceFromDB(source), nil
}

func (s *Service) ListFundingSources(ctx context.Context, input ListFundingSourcesInput) ([]FundingSource, error) {
	if s == nil || s.store == nil {
		return nil, errors.New("funding source service is not configured")
	}

	clientID, err := parseUUID(input.ClientID, ErrInvalidClientID)
	if err != nil {
		return nil, err
	}
	if input.Limit <= 0 || input.Offset < 0 {
		return nil, ErrInvalidPagination
	}

	sources, err := s.store.ListFundingSourcesByClientID(ctx, db.ListFundingSourcesByClientIDParams{
		ClientID: clientID,
		Limit:    input.Limit,
		Offset:   input.Offset,
	})
	if err != nil {
		return nil, err
	}

	result := make([]FundingSource, 0, len(sources))
	for _, source := range sources {
		result = append(result, fundingSourceFromDB(source))
	}

	return result, nil
}

func parseUUID(raw string, invalidErr error) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(raw); err != nil {
		return pgtype.UUID{}, invalidErr
	}

	return id, nil
}

func fundingSourceFromDB(source db.FundingSource) FundingSource {
	return FundingSource{
		ID:               source.ID.String(),
		ClientID:         source.ClientID.String(),
		Name:             source.Name,
		Type:             source.Type,
		PaymentAccountID: source.PaymentAccountID,
		Status:           source.Status,
		CreatedAt:        source.CreatedAt.Time,
		UpdatedAt:        source.UpdatedAt.Time,
	}
}
