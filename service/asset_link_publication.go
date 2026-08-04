package service

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
)

func resolveAssetLinkPublication(customerModel, assetKind, mediaType string) (*model.LinkModelPublication, int64, error) {
	customerModel = strings.TrimSpace(customerModel)
	if customerModel == "" {
		return nil, 0, nil
	}
	publication, err := model.GetUniqueLinkModelPublication(model.LinkContractNamespaceDefault, customerModel)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if model.IsRegisteredLinkSKU(customerModel) {
			return nil, 0, fmt.Errorf("%w: registered Link SKU must be accessed through a published customer model", ErrAssetBindingInvalidRequest)
		}
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, fmt.Errorf("%w: %v", ErrAssetBindingInvalidRequest, err)
	}
	if capability, ok := model.ResolveVideoSKUCapability(publication.LinkSKU); ok {
		if !capability.SupportsLinkAssets {
			return nil, 0, fmt.Errorf("%w: published Link contract does not support assets", ErrUnsupportedAssetType)
		}
	} else if capability, ok := model.ResolveImageSKUCapability(publication.LinkSKU); !ok || !capability.SupportsLinkAssets {
		return nil, 0, fmt.Errorf("%w: published Link contract does not support assets", ErrUnsupportedAssetType)
	}

	var minimumTTL int64
	supported := false
	for _, implementation := range model.LinkImplementationsForSKU(publication.LinkSKU) {
		asset := implementation.AssetCapability
		if !slices.Contains(asset.AssetKinds, assetKind) || !slices.Contains(asset.MediaTypes, mediaType) {
			continue
		}
		supported = true
		if asset.Supports(model.LinkAssetResolutionSourceURL) && asset.SourceMinTTLSeconds > minimumTTL {
			minimumTTL = asset.SourceMinTTLSeconds
		}
	}
	if !supported {
		return nil, 0, fmt.Errorf("%w: published Link contract does not support this asset kind or media type", ErrUnsupportedAssetType)
	}
	return publication, minimumTTL, nil
}
