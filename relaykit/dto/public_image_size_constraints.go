package dto

// PublicImageSizeConstraints describes explicit WxH pixel sizes. DefaultValue
// describes auto separately; aspect ratios are exact, not nearest matches.
type PublicImageSizeConstraints struct {
	Format       string   `json:"format"`
	MinDimension int      `json:"min_dimension"`
	MaxDimension int      `json:"max_dimension"`
	AspectRatios []string `json:"aspect_ratios"`
}
