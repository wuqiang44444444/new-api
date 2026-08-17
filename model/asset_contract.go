package model

const (
	AssetKindGeneral    = "general"
	AssetKindRealPerson = "real_person"
)

func ValidateAssetKind(kind string) bool {
	return kind == AssetKindGeneral || kind == AssetKindRealPerson
}

func ValidateAssetMediaType(mediaType string) bool {
	switch mediaType {
	case "image", "video", "audio":
		return true
	default:
		return false
	}
}
