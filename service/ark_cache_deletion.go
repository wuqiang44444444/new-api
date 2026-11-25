package service

import (
	"context"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// SendArkCacheDelete is called after a durable deletion claim. The original
// authorization scope is required for both user and system operations.
func SendArkCacheDelete(ctx context.Context, row model.ArkPrefixCache, credential string) (bool, error) {
	reply, status, err := SendArkCacheRequest(ctx, credential, http.MethodDelete, row.ProviderID, nil)
	commitCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	now := common.GetTimestamp()
	if IsArkCacheMissing(reply, status) {
		return false, model.ObserveArkCacheAbsent(commitCtx, row.ID, now)
	}
	if err != nil || !reply.Deleted || reply.ID != row.ProviderID || reply.Object != "response" {
		return false, model.ErrArkCacheBusy
	}
	return true, model.ConfirmArkPrefixCacheDelete(commitCtx, row.ID, now)
}

func reconcileArkCacheDelete(ctx context.Context, row model.ArkPrefixCache, now int64) error {
	taken, err := model.ClaimArkCacheDeletionCheck(ctx, row.ID, now)
	if err != nil || !taken {
		return err
	}
	credential := ArkCacheResourceCredential(row)
	if credential == "" {
		if row.ExpiresAt > 0 && row.ExpiresAt <= now {
			return model.StopExpiredArkCacheDeletionCheck(ctx, row.ID, now)
		}
		return model.ErrArkCacheBusy
	}
	reply, status, err := SendArkCacheRequest(ctx, credential, http.MethodGet, row.ProviderID, nil)
	if IsArkCacheMissing(reply, status) {
		return model.ObserveArkCacheAbsent(ctx, row.ID, common.GetTimestamp())
	}
	if err != nil || reply.ID != row.ProviderID || reply.Object != "response" ||
		(reply.Model != row.Endpoint && reply.Model != ArkPrefixCacheModelVersion) {
		return model.ErrArkCacheBusy
	}
	_, err = SendArkCacheDelete(ctx, row, credential)
	return err
}
