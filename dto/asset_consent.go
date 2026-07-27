package dto

type CreateRealPersonAuthorizationRequest struct {
	Model  string `json:"model"`
	Locale string `json:"locale,omitempty"`
}

type RealPersonAuthorizationResponse struct {
	ID         string `json:"id"`
	Status     string `json:"status"`
	ErrorCode  string `json:"error_code,omitempty"`
	ConsentURL string `json:"consent_url,omitempty"`
	ExpiresAt  int64  `json:"expires_at,omitempty"`
	CreatedAt  int64  `json:"created_at"`
	UpdatedAt  int64  `json:"updated_at"`
}

type CreateConsentPolicyRequest struct {
	Version     string `json:"version"`
	Locale      string `json:"locale"`
	Title       string `json:"title"`
	Content     string `json:"content"`
	EffectiveAt int64  `json:"effective_at,omitempty"`
}
