// Package assets implements Seedance asset-library protocols.
package assets

import "github.com/QuantumNous/new-api/dto"

const tokenSaveAssetRoot = "/v1/asset"

type TokenSaveAssetAdapter struct {
	*joyCreatorAssetAdapter
}

func NewTokenSaveAssetAdapter(baseURL, apiKey string, httpClient HTTPDoer) *TokenSaveAssetAdapter {
	return &TokenSaveAssetAdapter{joyCreatorAssetAdapter: newJoyCreatorAssetAdapter(
		baseURL,
		apiKey,
		httpClient,
		tokenSaveAssetRoot,
		dto.AssetUpstreamProfileRelay,
		"TokenSave",
	)}
}
