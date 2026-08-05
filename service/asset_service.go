package service

import (
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
)

func AssetResponse(asset *model.Asset) dto.AssetResponse {
	publicError := ""
	if asset.ErrorCode != "" {
		publicError = "asset processing failed"
	}
	modelName, target := "", ""
	var binding model.AssetBinding
	if model.DB.Select("requested_model", "binding_target").Where("asset_id = ?", asset.ID).Order("id asc").First(&binding).Error == nil {
		modelName, target = binding.RequestedModel, binding.BindingTarget
	}
	if modelName == "" {
		modelName = asset.RequestedModel
	}
	supersedesPublicID := ""
	if asset.SupersedesAssetID != nil {
		var superseded model.Asset
		if model.DB.Select("public_id").First(&superseded, "id = ? AND user_id = ?", *asset.SupersedesAssetID, asset.UserID).Error == nil {
			supersedesPublicID = superseded.PublicID
		}
	}
	return dto.AssetResponse{
		ID: asset.PublicID, Name: asset.Name, AssetKind: asset.AssetKind, MediaType: asset.MediaType,
		Model: modelName, Target: dto.PublicAssetTarget(target), SupersedesAssetID: supersedesPublicID,
		MigrationBatchID: asset.MigrationBatchID, MigrationReason: asset.MigrationReason,
		Status: asset.Status, ErrorCode: asset.ErrorCode, Error: publicError,
		CreatedAt: asset.CreatedAt, UpdatedAt: asset.UpdatedAt,
	}
}
