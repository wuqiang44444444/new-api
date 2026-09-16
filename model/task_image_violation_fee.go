package model

// TaskImageViolationFeePolicy is the admission-time native policy. Nil means
// no policy evidence was captured; it must never be filled from current settings.
type TaskImageViolationFeePolicy struct {
	Enabled    bool    `json:"enabled"`
	BaseAmount float64 `json:"base_amount"`
}
