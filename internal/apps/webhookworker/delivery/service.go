package delivery

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/prxgr4mm3r/payout-orchestrator/internal/db"
	"github.com/prxgr4mm3r/payout-orchestrator/internal/outbox"
	"github.com/prxgr4mm3r/payout-orchestrator/internal/platform/pgtypeutil"
)

var (
	ErrInvalidWebhookPayload = errors.New("invalid webhook payload")
	ErrUnsupportedEvent      = errors.New("unsupported event")
)

type Store interface {
	CreateWebhookDelivery(ctx context.Context, arg db.CreateWebhookDeliveryParams) (db.WebhookDelivery, error)
	MarkWebhookDeliveryDelivered(ctx context.Context, id pgtype.UUID) (db.WebhookDelivery, error)
	MarkWebhookDeliveryFailed(ctx context.Context, id pgtype.UUID) (db.WebhookDelivery, error)
}

type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

type Service struct {
	store  Store
	client HTTPDoer
	logger *log.Logger
}

func NewService(store Store, client HTTPDoer, logger *log.Logger) *Service {
	if client == nil {
		client = http.DefaultClient
	}
	if logger == nil {
		logger = log.Default()
	}

	return &Service{
		store:  store,
		client: client,
		logger: logger,
	}
}

func (s *Service) HandleEvent(ctx context.Context, event outbox.Event) error {
	if s == nil || s.store == nil || s.client == nil {
		return errors.New("webhook delivery service is not configured")
	}
	if event.EventType != outbox.EventTypePayoutResultWebhook {
		return ErrUnsupportedEvent
	}

	payload, err := parsePayload(event.Payload)
	if err != nil {
		return err
	}

	payoutID, err := pgtypeutil.ParseUUID(payload.PayoutID)
	if err != nil {
		return ErrInvalidWebhookPayload
	}
	clientID, err := pgtypeutil.ParseUUID(payload.ClientID)
	if err != nil {
		return ErrInvalidWebhookPayload
	}

	delivery, err := s.store.CreateWebhookDelivery(ctx, db.CreateWebhookDeliveryParams{
		PayoutID:     payoutID,
		ClientID:     clientID,
		TargetUrl:    payload.TargetURL,
		Payload:      event.Payload,
		Status:       "pending",
		AttemptCount: 0,
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, payload.TargetURL, bytes.NewReader(event.Payload))
	if err != nil {
		return s.markFailed(ctx, delivery.ID, err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return s.markFailed(ctx, delivery.ID, err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return s.markFailed(ctx, delivery.ID, errors.New("webhook endpoint returned non-success status"))
	}

	_, err = s.store.MarkWebhookDeliveryDelivered(ctx, delivery.ID)
	return err
}

func (s *Service) markFailed(ctx context.Context, id pgtype.UUID, cause error) error {
	if _, err := s.store.MarkWebhookDeliveryFailed(ctx, id); err != nil {
		return err
	}

	s.logger.Printf("webhook delivery failed id=%s err=%v", id.String(), cause)
	return nil
}

func parsePayload(raw []byte) (outbox.PayoutResultWebhookPayload, error) {
	payload, err := outbox.UnmarshalPayoutResultWebhookPayload(raw)
	if err != nil {
		return outbox.PayoutResultWebhookPayload{}, ErrInvalidWebhookPayload
	}
	if payload.PayoutID == "" || payload.ClientID == "" || payload.TargetURL == "" || payload.Status == "" {
		return outbox.PayoutResultWebhookPayload{}, ErrInvalidWebhookPayload
	}

	return payload, nil
}
