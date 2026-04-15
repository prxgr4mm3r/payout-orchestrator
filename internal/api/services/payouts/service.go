package payouts

import (
	"context"
	"errors"
	"math/big"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/prxgr4mm3r/payout-orchestrator/internal/db"
)

var (
	ErrInvalidClientID    = errors.New("invalid client id")
	ErrInvalidPagination  = errors.New("invalid pagination")
	ErrInvalidPayoutID    = errors.New("invalid payout id")
	ErrPayoutNotFound     = errors.New("payout not found")
	ErrUnsupportedNumeric = errors.New("unsupported numeric value")
)

type PayoutStore interface {
	GetPayoutByClientID(ctx context.Context, arg db.GetPayoutByClientIDParams) (db.Payout, error)
	ListPayoutsByClientID(ctx context.Context, arg db.ListPayoutsByClientIDParams) ([]db.Payout, error)
}

type Service struct {
	store PayoutStore
}

type GetPayoutInput struct {
	ClientID string
	ID       string
}

type ListPayoutsInput struct {
	ClientID string
	Limit    int32
	Offset   int32
}

type Payout struct {
	ID              string
	ClientID        string
	FundingSourceID string
	Amount          string
	Currency        string
	Status          string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func NewService(store PayoutStore) *Service {
	return &Service{store: store}
}

func (s *Service) GetPayout(ctx context.Context, input GetPayoutInput) (Payout, error) {
	if s == nil || s.store == nil {
		return Payout{}, errors.New("payout service is not configured")
	}

	clientID, err := parseUUID(input.ClientID, ErrInvalidClientID)
	if err != nil {
		return Payout{}, err
	}

	payoutID, err := parseUUID(input.ID, ErrInvalidPayoutID)
	if err != nil {
		return Payout{}, err
	}

	payout, err := s.store.GetPayoutByClientID(ctx, db.GetPayoutByClientIDParams{
		ClientID: clientID,
		ID:       payoutID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Payout{}, ErrPayoutNotFound
		}

		return Payout{}, err
	}

	return payoutFromDB(payout)
}

func (s *Service) ListPayouts(ctx context.Context, input ListPayoutsInput) ([]Payout, error) {
	if s == nil || s.store == nil {
		return nil, errors.New("payout service is not configured")
	}

	clientID, err := parseUUID(input.ClientID, ErrInvalidClientID)
	if err != nil {
		return nil, err
	}
	if input.Limit <= 0 || input.Offset < 0 {
		return nil, ErrInvalidPagination
	}

	payouts, err := s.store.ListPayoutsByClientID(ctx, db.ListPayoutsByClientIDParams{
		ClientID: clientID,
		Limit:    input.Limit,
		Offset:   input.Offset,
	})
	if err != nil {
		return nil, err
	}

	result := make([]Payout, 0, len(payouts))
	for _, payout := range payouts {
		mapped, err := payoutFromDB(payout)
		if err != nil {
			return nil, err
		}

		result = append(result, mapped)
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

func payoutFromDB(payout db.Payout) (Payout, error) {
	amount, err := numericString(payout.Amount)
	if err != nil {
		return Payout{}, err
	}

	return Payout{
		ID:              payout.ID.String(),
		ClientID:        payout.ClientID.String(),
		FundingSourceID: payout.FundingSourceID.String(),
		Amount:          amount,
		Currency:        payout.Currency,
		Status:          payout.Status,
		CreatedAt:       payout.CreatedAt.Time,
		UpdatedAt:       payout.UpdatedAt.Time,
	}, nil
}

func numericString(value pgtype.Numeric) (string, error) {
	if !value.Valid || value.Int == nil || value.NaN || value.InfinityModifier != pgtype.Finite {
		return "", ErrUnsupportedNumeric
	}

	sign := ""
	abs := new(big.Int).Set(value.Int)
	if abs.Sign() < 0 {
		sign = "-"
		abs.Abs(abs)
	}

	digits := abs.String()
	if value.Exp >= 0 {
		return sign + digits + strings.Repeat("0", int(value.Exp)), nil
	}

	scale := int(-value.Exp)
	if len(digits) <= scale {
		return sign + "0." + strings.Repeat("0", scale-len(digits)) + digits, nil
	}

	point := len(digits) - scale
	return sign + digits[:point] + "." + digits[point:], nil
}
