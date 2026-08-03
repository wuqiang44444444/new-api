package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
)

func RenameAsset(userID int, publicID, name string) (*model.Asset, error) {
	asset, err := model.GetAssetByPublicID(userID, publicID)
	if err != nil || asset == nil {
		return asset, err
	}
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 64 {
		return nil, fmt.Errorf("%w: asset name must contain 1 to 64 characters", ErrInvalidAssetRequest)
	}
	operationID, err := common.GenerateRandomCharsKey(16)
	if err != nil {
		return nil, err
	}
	asset.Name = name
	asset.UpdatedAt = common.GetTimestamp()
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(asset).Updates(map[string]any{"name": name, "updated_at": asset.UpdatedAt}).Error; err != nil {
			return err
		}
		var bindings []model.AssetBinding
		if err := tx.Where("asset_id = ? AND (status = ? OR (status = ? AND upstream_resource_id <> ?))", asset.ID, model.AssetBindingStatusActive, model.AssetBindingStatusProcessing, "").Find(&bindings).Error; err != nil {
			return err
		}
		for i := range bindings {
			bindingID := bindings[i].ID
			if _, err := model.EnsureAssetOperationJob(tx, &model.AssetOperationJob{IdempotencyKey: fmt.Sprintf("update-binding:%d:%s", bindingID, operationID), Kind: "update_binding", AssetID: &asset.ID, BindingID: &bindingID}, false); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return asset, nil
}

func DeleteAsset(_ context.Context, userID int, publicID string) error {
	asset, err := model.GetAssetByPublicID(userID, publicID)
	if err != nil || asset == nil {
		return err
	}
	now := common.GetTimestamp()
	return model.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(asset).Updates(map[string]any{"status": model.AssetStatusDeleting, "updated_at": now}).Error; err != nil {
			return err
		}
		var bindings []model.AssetBinding
		if err := tx.Where("asset_id = ? AND status <> ?", asset.ID, model.AssetBindingStatusDeleted).Find(&bindings).Error; err != nil {
			return err
		}
		for i := range bindings {
			bindingID := bindings[i].ID
			if err := tx.Model(&model.AssetBinding{}).Where("id = ?", bindingID).Updates(map[string]any{"status": model.AssetBindingStatusDeleting, "updated_at": now}).Error; err != nil {
				return err
			}
			job := &model.AssetOperationJob{IdempotencyKey: fmt.Sprintf("delete-binding:%d", bindingID), Kind: "delete_binding", AssetID: &asset.ID, BindingID: &bindingID, Status: model.AssetJobPending}
			if _, err := model.EnsureAssetOperationJob(tx, job, true); err != nil {
				return err
			}
		}
		if len(bindings) == 0 {
			if err := model.DeleteAssetSourceTx(tx, asset.ID); err != nil {
				return err
			}
			return tx.Model(asset).Updates(map[string]any{"status": model.AssetStatusDeleted, "deleted_at": now, "updated_at": now, "error_code": "", "error_message": ""}).Error
		}
		return nil
	})
}

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
