package service

import "errors"

var (
	ErrRealPersonAuthorizationNotRetryable = errors.New("real-person authorization is not retryable")
	ErrRealPersonVerificationUpstream      = errors.New("real-person verification upstream failed")
)
