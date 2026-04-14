package auth

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	apiauth "github.com/prxgr4mm3r/payout-orchestrator/internal/api/auth"
	"github.com/prxgr4mm3r/payout-orchestrator/internal/db"
)

var (
	ErrInvalidAPIKey = errors.New("invalid api key")
	ErrUnauthorized  = errors.New("unauthorized")
)

type ClientStore interface {
	GetClientByApiKey(ctx context.Context, apiKey pgtype.UUID) (db.Client, error)
}

type Service struct {
	clients ClientStore
}

func NewService(clients ClientStore) *Service {
	return &Service{clients: clients}
}

func (s *Service) AuthenticateAPIKey(ctx context.Context, rawAPIKey string) (apiauth.Client, error) {
	if s == nil || s.clients == nil {
		return apiauth.Client{}, errors.New("auth service is not configured")
	}

	var apiKey pgtype.UUID
	if err := apiKey.Scan(rawAPIKey); err != nil {
		return apiauth.Client{}, ErrInvalidAPIKey
	}

	client, err := s.clients.GetClientByApiKey(ctx, apiKey)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return apiauth.Client{}, ErrUnauthorized
		}

		return apiauth.Client{}, err
	}

	return apiauth.Client{
		ID:   client.ID.String(),
		Name: client.Name,
	}, nil
}
