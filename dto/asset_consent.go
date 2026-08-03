package dto

type CreateRealPersonAuthorizationRequest struct {
	Model          string `json:"model"`
	EndUserSubject string `json:"end_user_subject"`
}

type RealPersonAuthorizationResponse struct {
	ID              string `json:"id"`
	Status          string `json:"status"`
	ErrorCode       string `json:"error_code,omitempty"`
	VerificationURL string `json:"verification_url,omitempty"`
	CleanupStatus   string `json:"cleanup_status,omitempty"`
	ExpiresAt       int64  `json:"expires_at,omitempty"`
	CreatedAt       int64  `json:"created_at"`
	UpdatedAt       int64  `json:"updated_at"`
}
