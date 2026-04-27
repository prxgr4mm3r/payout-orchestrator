package outbox

import "encoding/json"

const (
	EventTypeProcessPayout       = "process_payout"
	EventTypePayoutResultWebhook = "payout_result_webhook"
)

type ProcessPayoutPayload struct {
	PayoutID string `json:"payout_id"`
	ClientID string `json:"client_id"`
}

type PayoutResultWebhookPayload struct {
	EventType     string `json:"event_type"`
	PayoutID      string `json:"payout_id"`
	ClientID      string `json:"client_id"`
	Status        string `json:"status"`
	FailureReason string `json:"failure_reason,omitempty"`
}

func MarshalProcessPayoutPayload(payoutID, clientID string) ([]byte, error) {
	return json.Marshal(ProcessPayoutPayload{
		PayoutID: payoutID,
		ClientID: clientID,
	})
}

func MarshalPayoutResultWebhookPayload(payoutID, clientID, status, failureReason string) ([]byte, error) {
	return json.Marshal(PayoutResultWebhookPayload{
		EventType:     EventTypePayoutResultWebhook,
		PayoutID:      payoutID,
		ClientID:      clientID,
		Status:        status,
		FailureReason: failureReason,
	})
}

func UnmarshalProcessPayoutPayload(raw []byte) (ProcessPayoutPayload, error) {
	var payload ProcessPayoutPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ProcessPayoutPayload{}, err
	}

	return payload, nil
}
