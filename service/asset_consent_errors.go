package service

import "errors"

var (
	ErrInvalidConsentPolicy                = errors.New("consent policy is invalid")
	ErrRealPersonAuthorizationNotRetryable = errors.New("real-person authorization is not retryable")
	ErrRealPersonVerificationUpstream      = errors.New("real-person verification upstream failed")
)
