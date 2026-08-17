package dto

type AssetSource struct {
	Type      string `json:"type"`
	URL       string `json:"url,omitempty"`
	ExpiresAt int64  `json:"expires_at,omitempty"`
}

type CreateAssetRequest struct {
	Name         string      `json:"name"`
	AssetKind    string      `json:"asset_kind"`
	MediaType    string      `json:"media_type"`
	Model        string      `json:"model,omitempty"`
	AssetGroupID string      `json:"asset_group_id,omitempty"`
	Source       AssetSource `json:"source"`
}

type UpdateAssetRequest struct {
	Name  string `json:"name"`
	Model string `json:"model"`
}

type AssetResponse struct {
	Object    string `json:"object"`
	ID        string `json:"id"`
	Model     string `json:"model"`
	Reference string `json:"reference,omitempty"`
	Status    string `json:"status"`
	ErrorCode string `json:"error_code,omitempty"`
	Error     string `json:"error,omitempty"`
}

type CreateAssetGroupRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	GroupKind   string `json:"group_kind"`
	Model       string `json:"model"`
	RedirectURL string `json:"redirect_url,omitempty"`
}

type AssetGroupResponse struct {
	Object          string `json:"object"`
	ID              string `json:"id"`
	Model           string `json:"model"`
	GroupID         string `json:"group_id,omitempty"`
	Status          string `json:"status"`
	VerificationURL string `json:"verification_url,omitempty"`
	ExpiresAt       int64  `json:"expires_at,omitempty"`
}
