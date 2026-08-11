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
	Name string `json:"name"`
}

type AssetResponse struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	AssetKind    string `json:"asset_kind"`
	MediaType    string `json:"media_type"`
	Model        string `json:"model,omitempty"`
	AssetGroupID string `json:"asset_group_id,omitempty"`
	Status       string `json:"status"`
	ErrorCode    string `json:"error_code,omitempty"`
	Error        string `json:"error,omitempty"`
	CreatedAt    int64  `json:"created_at"`
	UpdatedAt    int64  `json:"updated_at"`
}

type CreateAssetGroupRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	GroupKind   string `json:"group_kind"`
	Model       string `json:"model"`
	RedirectURL string `json:"redirect_url,omitempty"`
}

type AssetGroupResponse struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Description     string `json:"description,omitempty"`
	GroupKind       string `json:"group_kind"`
	Model           string `json:"model"`
	Status          string `json:"status"`
	VerificationURL string `json:"verification_url,omitempty"`
	ExpiresAt       int64  `json:"expires_at,omitempty"`
	ErrorCode       string `json:"error_code,omitempty"`
	Error           string `json:"error,omitempty"`
	CreatedAt       int64  `json:"created_at"`
	UpdatedAt       int64  `json:"updated_at"`
}
