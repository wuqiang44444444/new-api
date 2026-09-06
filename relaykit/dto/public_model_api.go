package dto

type PublicModelAPI struct {
	Image  *PublicImageAPI `json:"image,omitempty"`
	Video  *PublicVideoAPI `json:"video,omitempty"`
	Assets *PublicAssetAPI `json:"assets,omitempty"`
}

type PublicImageAPI struct {
	DocumentationPath string               `json:"documentation_path"`
	Operations        []PublicAPIOperation `json:"operations"`
	Creation          PublicImageCreation  `json:"creation"`
	// Edit 是 /v1/images/edits 的合同投影；仅同时发布编辑能力的模型族填写。
	Edit *PublicImageCreation `json:"edit,omitempty"`
}

type PublicImageCreation struct {
	Method               string               `json:"method"`
	Path                 string               `json:"path"`
	ContentType          string               `json:"content_type"`
	RequiredFields       []string             `json:"required_fields"`
	Model                string               `json:"model"`
	AdditionalProperties bool                 `json:"additional_properties"`
	Parameters           []PublicAPIParameter `json:"parameters"`
	// RequiredOneOf requires exactly one of these alternative input fields.
	RequiredOneOf []string `json:"required_one_of,omitempty"`
}

type PublicVideoAPI struct {
	Protocol          string               `json:"protocol"`
	DocumentationPath string               `json:"documentation_path"`
	Operations        []PublicAPIOperation `json:"operations"`
	Creation          PublicVideoCreation  `json:"creation"`
}

type PublicVideoCreation struct {
	Method               string                   `json:"method"`
	Path                 string                   `json:"path"`
	ContentType          string                   `json:"content_type"`
	RequiredFields       []string                 `json:"required_fields"`
	Model                string                   `json:"model"`
	AdditionalProperties bool                     `json:"additional_properties"`
	Parameters           []PublicAPIParameter     `json:"parameters"`
	ContentTypes         []PublicVideoContentType `json:"content_types"`
}

// PublicAPIParameter is the machine-readable, customer-facing contract for one
// accepted request field. Nested names use dot notation, for example
// extra_fields.aspect_ratio. Fields absent from the list are unsupported when
// AdditionalProperties is false.
type PublicAPIParameter struct {
	Name          string   `json:"name"`
	Type          string   `json:"type"`
	ItemType      string   `json:"item_type,omitempty"`
	Required      bool     `json:"required"`
	Enum          []string `json:"enum,omitempty"`
	FixedValue    any      `json:"fixed_value,omitempty"`
	DefaultValue  any      `json:"default_value,omitempty"`
	Minimum       *int     `json:"minimum,omitempty"`
	Maximum       *int     `json:"maximum,omitempty"`
	SpecialValues []int    `json:"special_values,omitempty"`
	MinLength     *int     `json:"min_length,omitempty"`
	MaxLength     *int     `json:"max_length,omitempty"`
	MinItems      *int     `json:"min_items,omitempty"`
	MaxItems      *int     `json:"max_items,omitempty"`
}

type PublicVideoContentType struct {
	Type           string   `json:"type"`
	Roles          []string `json:"roles,omitempty"`
	RequiredFields []string `json:"required_fields"`
	MinItems       int      `json:"min_items,omitempty"`
	MaxItems       int      `json:"max_items,omitempty"`
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
