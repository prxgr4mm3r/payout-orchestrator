package payouts

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/prxgr4mm3r/payout-orchestrator/internal/db"
	"github.com/prxgr4mm3r/payout-orchestrator/internal/platform/pgtypeutil"
)

var (
	ErrInvalidClientID        = errors.New("invalid client id")
	ErrInvalidFundingSourceID = errors.New("invalid funding source id")
	ErrInvalidIdempotencyKey  = errors.New("invalid idempotency key")
	ErrInvalidPagination      = errors.New("invalid pagination")
	ErrInvalidPayout          = errors.New("invalid payout")
	ErrInvalidPayoutID        = errors.New("invalid payout id")
	ErrFundingSourceNotFound  = errors.New("funding source not found")
	ErrIdempotencyConflict    = errors.New("idempotency conflict")
	ErrPayoutNotFound         = errors.New("payout not found")
	ErrUnsupportedNumeric     = errors.New("unsupported numeric value")
)

var errConcurrentIdempotencyKeyInsert = errors.New("concurrent idempotency key insert")

const payoutCreatedOutboxEventType = "process_payout"

type PayoutStore interface {
	CreateIdempotencyKey(ctx context.Context, arg db.CreateIdempotencyKeyParams) (db.IdempotencyKey, error)
	CreateOutboxEvent(ctx context.Context, arg db.CreateOutboxEventParams) (db.OutboxEvent, error)
	CreatePayout(ctx context.Context, arg db.CreatePayoutParams) (db.Payout, error)
	GetFundingSourceByClientID(ctx context.Context, arg db.GetFundingSourceByClientIDParams) (db.FundingSource, error)
	GetIdempotencyKey(ctx context.Context, arg db.GetIdempotencyKeyParams) (db.IdempotencyKey, error)
	GetPayoutByClientID(ctx context.Context, arg db.GetPayoutByClientIDParams) (db.Payout, error)
	ListPayoutsByClientID(ctx context.Context, arg db.ListPayoutsByClientIDParams) ([]db.Payout, error)
}

type TxRunner interface {
	WithinTx(ctx context.Context, fn func(store PayoutStore) error) error
}

type Service struct {
	store    PayoutStore
	txRunner TxRunner
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

type CreatePayoutInput struct {
	ClientID        string
	FundingSourceID string
	IdempotencyKey  string
	Amount          string
	Currency        string
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

type payoutCreatedOutboxPayload struct {
	PayoutID string `json:"payout_id"`
	ClientID string `json:"client_id"`
}

func NewService(store PayoutStore) *Service {
	return &Service{
		store:    store,
		txRunner: passthroughTxRunner{store: store},
	}
}

func NewServiceWithTx(store PayoutStore, txRunner TxRunner) *Service {
	return &Service{
		store:    store,
		txRunner: txRunner,
	}
}

func (s *Service) CreatePayout(ctx context.Context, input CreatePayoutInput) (Payout, error) {
	if s == nil || s.store == nil || s.txRunner == nil {
		return Payout{}, errors.New("payout service is not configured")
	}

	clientID, err := pgtypeutil.ParseUUID(input.ClientID)
	if err != nil {
		return Payout{}, ErrInvalidClientID
	}

	fundingSourceID, err := pgtypeutil.ParseUUID(input.FundingSourceID)
	if err != nil {
		return Payout{}, ErrInvalidFundingSourceID
	}

	amount, err := parsePositiveNumeric(input.Amount)
	if err != nil {
		return Payout{}, ErrInvalidPayout
	}

	currency := strings.TrimSpace(input.Currency)
	if currency == "" || len(currency) > 12 {
		return Payout{}, ErrInvalidPayout
	}

	idempotencyKey := strings.TrimSpace(input.IdempotencyKey)
	if idempotencyKey == "" {
		return Payout{}, ErrInvalidIdempotencyKey
	}

	amountHashValue, err := numericString(amount)
	if err != nil {
		return Payout{}, err
	}
	requestHash := createPayoutRequestHash(clientID.String(), fundingSourceID.String(), amountHashValue, currency)

	var payout db.Payout

	err = s.txRunner.WithinTx(ctx, func(store PayoutStore) error {
		existing, found, err := loadIdempotentPayout(ctx, store, clientID, idempotencyKey, requestHash)
		if err != nil {
			return err
		}
		if found {
			payout = existing
			return nil
		}

		if _, err := store.GetFundingSourceByClientID(ctx, db.GetFundingSourceByClientIDParams{
			ClientID: clientID,
			ID:       fundingSourceID,
		}); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrFundingSourceNotFound
			}

			return err
		}

		created, err := store.CreatePayout(ctx, db.CreatePayoutParams{
			ClientID:        clientID,
			FundingSourceID: fundingSourceID,
			Amount:          amount,
			Currency:        currency,
		})
		if err != nil {
			return err
		}

		if _, err := store.CreateIdempotencyKey(ctx, db.CreateIdempotencyKeyParams{
			Key:         idempotencyKey,
			ClientID:    clientID,
			RequestHash: requestHash,
			PayoutID:    created.ID,
		}); err != nil {
			if isUniqueViolation(err) {
				return errConcurrentIdempotencyKeyInsert
			}

			return err
		}

		payload, err := marshalPayoutCreatedOutboxPayload(created)
		if err != nil {
			return err
		}

		if _, err := store.CreateOutboxEvent(ctx, db.CreateOutboxEventParams{
			EventType: payoutCreatedOutboxEventType,
			EntityID:  created.ID,
			Payload:   payload,
		}); err != nil {
			return err
		}

		payout = created
		return nil
	})
	if errors.Is(err, errConcurrentIdempotencyKeyInsert) {
		payout, _, err = loadIdempotentPayout(ctx, s.store, clientID, idempotencyKey, requestHash)
	}
	if err != nil {
		return Payout{}, err
	}

	return payoutFromDB(payout)
}

func (s *Service) GetPayout(ctx context.Context, input GetPayoutInput) (Payout, error) {
	if s == nil || s.store == nil {
		return Payout{}, errors.New("payout service is not configured")
	}

	clientID, err := pgtypeutil.ParseUUID(input.ClientID)
	if err != nil {
		return Payout{}, ErrInvalidClientID
	}

	payoutID, err := pgtypeutil.ParseUUID(input.ID)
	if err != nil {
		return Payout{}, ErrInvalidPayoutID
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

	clientID, err := pgtypeutil.ParseUUID(input.ClientID)
	if err != nil {
		return nil, ErrInvalidClientID
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

func parsePositiveNumeric(raw string) (pgtype.Numeric, error) {
	var amount pgtype.Numeric
	if err := amount.Scan(strings.TrimSpace(raw)); err != nil {
		return pgtype.Numeric{}, err
	}
	if !amount.Valid || amount.Int == nil || amount.NaN || amount.InfinityModifier != pgtype.Finite || amount.Int.Sign() <= 0 {
		return pgtype.Numeric{}, ErrInvalidPayout
	}

	return amount, nil
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

func marshalPayoutCreatedOutboxPayload(payout db.Payout) ([]byte, error) {
	return json.Marshal(payoutCreatedOutboxPayload{
		PayoutID: payout.ID.String(),
		ClientID: payout.ClientID.String(),
	})
}

func createPayoutRequestHash(clientID, fundingSourceID, amount, currency string) string {
	hash := sha256.New()
	for _, part := range []string{clientID, fundingSourceID, amount, currency} {
		_, _ = hash.Write([]byte(part))
		_, _ = hash.Write([]byte{0})
	}

	return hex.EncodeToString(hash.Sum(nil))
}

func loadIdempotentPayout(ctx context.Context, store PayoutStore, clientID pgtype.UUID, idempotencyKey, requestHash string) (db.Payout, bool, error) {
	existingKey, err := store.GetIdempotencyKey(ctx, db.GetIdempotencyKeyParams{
		ClientID: clientID,
		Key:      idempotencyKey,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Payout{}, false, nil
	}
	if err != nil {
		return db.Payout{}, false, err
	}
	if existingKey.RequestHash != requestHash {
		return db.Payout{}, false, ErrIdempotencyConflict
	}

	payout, err := store.GetPayoutByClientID(ctx, db.GetPayoutByClientIDParams{
		ClientID: clientID,
		ID:       existingKey.PayoutID,
	})
	if err != nil {
		return db.Payout{}, false, err
	}

	return payout, true, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
