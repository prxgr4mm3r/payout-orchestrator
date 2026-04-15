package payout

type Status string

const (
	StatusPending    Status = "pending"
	StatusProcessing Status = "processing"
	StatusSucceeded  Status = "succeeded"
	StatusFailed     Status = "failed"
)

func (s Status) Valid() bool {
	switch s {
	case StatusPending, StatusProcessing, StatusSucceeded, StatusFailed:
		return true
	default:
		return false
	}
}

func CanTransition(from, to Status) bool {
	switch from {
	case StatusPending:
		return to == StatusProcessing
	case StatusProcessing:
		return to == StatusSucceeded || to == StatusFailed
	default:
		return false
	}
}
