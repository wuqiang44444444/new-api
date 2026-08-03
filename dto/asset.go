package dto

type AssetSource struct {
	Type      string `json:"type"`
	URL       string `json:"url,omitempty"`
	ExpiresAt int64  `json:"expires_at,omitempty"`
}

type CreateAssetRequest struct {
	Name            string      `json:"name"`
	AssetKind       string      `json:"asset_kind"`
	MediaType       string      `json:"media_type"`
	AuthorizationID string      `json:"authorization_id,omitempty"`
	Model           string      `json:"model,omitempty"`
	Target          string      `json:"target,omitempty"`
	Source          AssetSource `json:"source"`
}

type UpdateAssetRequest struct {
	Name string `json:"name"`
}

type MigrateAssetRequest struct {
	Name             string      `json:"name,omitempty"`
	AuthorizationID  string      `json:"authorization_id,omitempty"`
	Model            string      `json:"model,omitempty"`
	Target           string      `json:"target,omitempty"`
	Source           AssetSource `json:"source"`
	MigrationBatchID string      `json:"migration_batch_id,omitempty"`
	MigrationReason  string      `json:"migration_reason"`
}

type AssetResponse struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	AssetKind         string `json:"asset_kind"`
	MediaType         string `json:"media_type"`
	Model             string `json:"model,omitempty"`
	Target            string `json:"target,omitempty"`
	SupersedesAssetID string `json:"supersedes_asset_id,omitempty"`
	MigrationBatchID  string `json:"migration_batch_id,omitempty"`
	MigrationReason   string `json:"migration_reason,omitempty"`
	Status            string `json:"status"`
	ErrorCode         string `json:"error_code,omitempty"`
	Error             string `json:"error,omitempty"`
	CreatedAt         int64  `json:"created_at"`
	UpdatedAt         int64  `json:"updated_at"`
}

type AssetBindingResponse struct {
	ID        string `json:"id"`
	AssetID   string `json:"asset_id"`
	Target    string `json:"target,omitempty"`
	Model     string `json:"model,omitempty"`
	Status    string `json:"status"`
	ErrorCode string `json:"error_code,omitempty"`
	Error     string `json:"error,omitempty"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}
