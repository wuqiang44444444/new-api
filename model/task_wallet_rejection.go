package model

import (
	"errors"
)

// PreserveTaskWalletRejection keeps the public subscription error while
// recording that the wallet already failed and its fallback could not fund the task.
func PreserveTaskWalletRejection(walletErr, fallbackErr error) error {
	if errors.Is(walletErr, ErrTaskAttemptInsufficientQuota) &&
		!errors.Is(walletErr, ErrTaskAttemptTokenQuota) &&
		errors.Is(fallbackErr, ErrTaskAttemptSubscriptionUnavailable) {
		return taskWalletFallbackRejection{fallbackErr}
	}
	return fallbackErr
}

type taskWalletFallbackRejection struct{ error }

func (err taskWalletFallbackRejection) Unwrap() error { return err.error }
func (err taskWalletFallbackRejection) Is(target error) bool {
	return target == ErrTaskAttemptWalletFallbackRejected
}
