package dto

import "strings"

const (
	AssetTargetManagementLibrary = "management_library"
	AssetTargetJoyCreatorLegacy  = "joycreator_library"
)

func NormalizeAssetTarget(target string) (string, bool) {
	switch strings.TrimSpace(target) {
	case "":
		return "", true
	case AssetTargetManagementLibrary, AssetTargetJoyCreatorLegacy:
		return AssetTargetManagementLibrary, true
	default:
		return "", false
	}
}

func PublicAssetTarget(target string) string {
	normalized, ok := NormalizeAssetTarget(target)
	if !ok {
		return ""
	}
	return normalized
}
