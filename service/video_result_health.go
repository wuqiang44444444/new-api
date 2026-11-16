package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
)

const videoResultStorageResource = "video_result_storage"
const videoResultStorageProbeKey = "object-storage-health/video-result"

func checkVideoResultStorageHealth(now time.Time) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if err := recordVideoResultStorageHealth(ctx, now.Unix()); err != nil {
		logger.LogWarn(ctx, "video delivery storage health check failed")
	}
}

// recordVideoResultStorageHealth runs independently of the transfer scheduler,
// which stops scheduling new transfer passes when storage is unavailable.
func recordVideoResultStorageHealth(ctx context.Context, now int64) error {
	key := fmt.Sprintf("%x", sha256.Sum256([]byte(videoResultStorageResource)))
	backlog, err := model.CountVideoResultTransferBacklog(ctx)
	if err != nil {
		return err
	}
	if backlog == 0 {
		gapCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		gaps, err := model.FindVideoResultRegistrationGaps(gapCtx, videoResultTransferPlatforms, now-int64(videoResultSweepWindow.Seconds()), 1)
		cancel()
		if err != nil {
			return err
		}
		if len(gaps) == 0 {
			return model.CloseResourceAlert(ctx, key, now)
		}
	}
	if videoResultStorageHealthy(ctx) {
		return model.CloseResourceAlert(ctx, key, now)
	}
	opened, err := model.OpenVideoResultStorageIncident(ctx, model.ResourceAlertEpisode{
		OpenKey: &key, Resource: videoResultStorageResource, OpenedAt: now,
		Subject: "视频结果存储不可用 / Video result storage unavailable",
	}, model.VideoDeliveryError{
		CreatedAt: now, NextAttemptAt: now,
		Stage: videoDeliveryStorageStage, Reason: "video_delivery_storage_unavailable",
	})
	if err != nil || !opened {
		return err
	}
	logger.LogWarn(ctx, "video delivery storage unavailable with waiting results")
	return nil
}

// Use the immutable storage client, without touching the image worker's
// readiness cache, in-flight probe or probe object.
func videoResultStorageHealthy(ctx context.Context) bool {
	store, err := currentImageObjectStore(ctx)
	if err != nil {
		return false
	}
	if _, ok := store.(videoResultObjectStore); !ok {
		return false
	}
	probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	payload := []byte("video result storage readiness")
	if _, err := store.putImageObject(probeCtx, videoResultStorageProbeKey, "text/plain", payload); err != nil {
		return false
	}
	body, err := store.fetchImageObjectBytes(probeCtx, videoResultStorageProbeKey)
	return err == nil && bytes.Equal(body, payload)
}
