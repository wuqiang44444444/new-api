package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
)

func MigrateRemoteAsset(
	ctx context.Context,
	userID, tokenID int,
	userGroup, usingGroup, idempotencyKey, sourcePublicID string,
	req dto.MigrateAssetRequest,
) (*model.Asset, error) {
	source, err := model.GetAssetByPublicIDForApp(userID, tokenID, sourcePublicID)
	if err != nil || source == nil {
		return source, err
	}
	if source.AssetKind == model.AssetKindRealPerson && strings.TrimSpace(req.AuthorizationID) == "" {
		return nil, fmt.Errorf("%w: real-person asset migration requires a new or revalidated authorization", ErrRealPersonAuthorizationNotReady)
	}
	if source.Status != model.AssetStatusReady {
		return nil, fmt.Errorf("%w: only ready assets can be migrated", ErrAssetBindingRequired)
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		req.Name = source.Name
	}
	req.MigrationReason = strings.TrimSpace(req.MigrationReason)
	if req.MigrationReason == "" || len([]rune(req.MigrationReason)) > 300 || strings.ContainsAny(req.MigrationReason, "\r\n") {
		return nil, fmt.Errorf("%w: migration_reason must contain 1 to 300 characters on one line", ErrInvalidAssetRequest)
	}
	req.MigrationBatchID = strings.TrimSpace(req.MigrationBatchID)
	if normalizedTarget, ok := dto.NormalizeAssetTarget(req.Target); ok {
		req.Target = normalizedTarget
	} else {
		return nil, fmt.Errorf("%w: unsupported binding target", ErrUnsupportedAssetBindingTarget)
	}
	idempotencyRequest := req
	if req.MigrationBatchID == "" {
		random, err := common.GenerateRandomCharsKey(20)
		if err != nil {
			return nil, err
		}
		req.MigrationBatchID = "mig_" + random
	}
	if len(req.MigrationBatchID) > 64 || strings.ContainsAny(req.MigrationBatchID, "\r\n") {
		return nil, fmt.Errorf("%w: migration_batch_id is invalid", ErrInvalidAssetRequest)
	}
	createRequest := dto.CreateAssetRequest{
		Name:            req.Name,
		AssetKind:       source.AssetKind,
		MediaType:       source.MediaType,
		AuthorizationID: req.AuthorizationID,
		Model:           req.Model,
		Target:          req.Target,
		Source:          req.Source,
	}
	endpoint := "/v1/assets/" + source.PublicID + "/migrations"
	return createRemoteAsset(
		ctx,
		userID,
		tokenID,
		userGroup,
		usingGroup,
		idempotencyKey,
		endpoint,
		idempotencyRequest,
		createRequest,
		&assetMigrationMetadata{
			SupersedesAssetID: &source.ID,
			BatchID:           req.MigrationBatchID,
			Reason:            req.MigrationReason,
		},
	)
}
