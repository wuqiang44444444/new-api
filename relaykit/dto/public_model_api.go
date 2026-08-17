package dto

type PublicModelAPI struct {
	Video  PublicVideoAPI `json:"video"`
	Assets PublicAssetAPI `json:"assets"`
}

type PublicVideoAPI struct {
	Protocol          string               `json:"protocol"`
	DocumentationPath string               `json:"documentation_path"`
	Operations        []PublicAPIOperation `json:"operations"`
	Creation          PublicVideoCreation  `json:"creation"`
}

type PublicVideoCreation struct {
	Method         string   `json:"method"`
	Path           string   `json:"path"`
	ContentType    string   `json:"content_type"`
	RequiredFields []string `json:"required_fields"`
	Model          string   `json:"model"`
}

type PublicAssetAPI struct {
	Supported         bool                 `json:"supported"`
	DocumentationPath string               `json:"documentation_path"`
	ManagementMode    string               `json:"management_mode"`
	RequiresModel     bool                 `json:"requires_model"`
	ReferenceFormat   string               `json:"reference_format"`
	ReuseScope        string               `json:"reuse_scope,omitempty"`
	Media             []PublicAssetMedia   `json:"media"`
	Operations        []PublicAPIOperation `json:"operations"`
	Creation          *PublicAssetCreation `json:"creation,omitempty"`
}

type PublicAssetMedia struct {
	Kind                  string `json:"kind"`
	MediaType             string `json:"media_type"`
	AssetGroupRequirement string `json:"asset_group_requirement"`
}

type PublicAPIOperation struct {
	Operation string `json:"operation"`
	Method    string `json:"method"`
	Path      string `json:"path"`
	Supported bool   `json:"supported"`
}
