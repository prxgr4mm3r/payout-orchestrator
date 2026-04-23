package outbox

import "encoding/json"

const EventTypeProcessPayout = "process_payout"

type ProcessPayoutPayload struct {
	PayoutID string `json:"payout_id"`
	ClientID string `json:"client_id"`
}

func MarshalProcessPayoutPayload(payoutID, clientID string) ([]byte, error) {
	return json.Marshal(ProcessPayoutPayload{
		PayoutID: payoutID,
		ClientID: clientID,
	})
}

func UnmarshalProcessPayoutPayload(raw []byte) (ProcessPayoutPayload, error) {
	var payload ProcessPayoutPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ProcessPayoutPayload{}, err
	}

	return payload, nil
}
