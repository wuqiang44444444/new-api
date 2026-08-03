package service

import (
	"fmt"

	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
)

func lockRemoteCreateState(tx *gorm.DB, asset *model.Asset, binding *model.AssetBinding) (*model.Asset, *model.AssetBinding, bool, error) {
	authorizationActive := true
	if asset.AuthorizationID != nil {
		authorization, err := model.LockRealPersonAuthorization(tx, *asset.AuthorizationID)
		if err != nil {
			return nil, nil, false, err
		}
		authorizationActive = authorization.UserID == asset.UserID && authorization.AppID == asset.AppID && authorization.EndUserSubjectHash != "" && authorization.EndUserSubjectHash == asset.EndUserSubjectHash && authorization.Status == model.RealPersonAuthorizationAuthorized && authorization.RevokedAt == 0
	}
	currentAsset, err := model.LockAsset(tx, asset.ID)
	if err != nil {
		return nil, nil, false, err
	}
	currentBinding, err := model.LockAssetBinding(tx, binding.ID)
	if err != nil {
		return nil, nil, false, err
	}
	if currentBinding.AssetID != currentAsset.ID {
		return nil, nil, false, fmt.Errorf("remote asset binding no longer belongs to the asset")
	}
	return currentAsset, currentBinding, authorizationActive, nil
}

func remoteCreateWatchdogKey(bindingID int64) string {
	return fmt.Sprintf("resolve-unknown-create:%d", bindingID)
}

func finishRemoteCreateWatchdogTx(tx *gorm.DB, bindingID int64) error {
	return model.CompleteQueuedAssetOperationJobTx(tx, remoteCreateWatchdogKey(bindingID))
}

func remoteCreateMayProceed(asset *model.Asset, binding *model.AssetBinding) (bool, error) {
	proceed := false
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		currentAsset, currentBinding, authorizationActive, err := lockRemoteCreateState(tx, asset, binding)
		if err != nil {
			return err
		}
		*asset = *currentAsset
		*binding = *currentBinding
		proceed = authorizationActive && currentAsset.Status == model.AssetStatusCreating && currentBinding.Status == model.AssetBindingStatusCreating
		if proceed {
			return nil
		}
		return finishRemoteCreateWatchdogTx(tx, currentBinding.ID)
	})
	return proceed, err
}

func remoteCreateStateOpen(asset *model.Asset, binding *model.AssetBinding, authorizationActive bool) bool {
	if !authorizationActive {
		return false
	}
	assetOpen := asset.Status == model.AssetStatusCreating || asset.Status == model.AssetStatusCreateUnknown
	bindingOpen := binding.Status == model.AssetBindingStatusCreating || binding.Status == model.AssetBindingStatusCreateUnknown
	return assetOpen && bindingOpen
}
