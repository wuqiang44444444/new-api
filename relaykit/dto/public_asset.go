package dto

const (
	PublicAssetNameMaxCharacters     = 64
	PublicAssetFunCloudMaxBytes      = 100 * 1024 * 1024
	PublicAssetFunCloudRedirectLimit = 5

	PublicAssetGroupRequired    = "required"
	PublicAssetGroupOptional    = "optional"
	PublicAssetGroupUnsupported = "unsupported"
)

type PublicAssetCreation struct {
	Method            string                    `json:"method"`
	Path              string                    `json:"path"`
	ContentType       string                    `json:"content_type"`
	RequiredFields    []string                  `json:"required_fields"`
	NameMaxCharacters int                       `json:"name_max_characters"`
	Source            PublicAssetSourceContract `json:"source"`
	Example           PublicAssetCreateExample  `json:"example"`
}

type PublicAssetSourceContract struct {
	Type                         string                        `json:"type"`
	URLScheme                    string                        `json:"url_scheme"`
	PublicNetworkOnly            bool                          `json:"public_network_only"`
	Port                         int                           `json:"port"`
	MaxURLLength                 int                           `json:"max_url_length"`
	ExpiresAtMinRemainingSeconds int64                         `json:"expires_at_min_remaining_seconds"`
	MaxBytes                     int64                         `json:"max_bytes,omitempty"`
	RedirectLimit                int                           `json:"redirect_limit,omitempty"`
	ContentTypeMustMatchMedia    bool                          `json:"content_type_must_match_media_type"`
	AcceptedContentTypes         []PublicAssetSourceMediaTypes `json:"accepted_content_types,omitempty"`
}

type PublicAssetSourceMediaTypes struct {
	MediaType    string   `json:"media_type"`
	ContentTypes []string `json:"content_types"`
}

type PublicAssetCreateExample struct {
	Name         string                   `json:"name"`
	AssetKind    string                   `json:"asset_kind"`
	MediaType    string                   `json:"media_type"`
	Model        string                   `json:"model"`
	AssetGroupID string                   `json:"asset_group_id,omitempty"`
	Source       PublicAssetSourceExample `json:"source"`
}

type PublicAssetSourceExample struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}
