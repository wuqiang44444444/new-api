package service

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

// FunCloud 托管素材的视频引用边界。在 durable hold 与 Provider POST 之前，
// 校验 asset://fhas_* 引用的用户归属与可引用状态并冻结素材事实；发送阶段
// 派生内部临时 URL 替换引用，接受后不再读取当前素材状态。不带托管前缀的
// 引用原样透传，不探测、不双读、不回退。

const funCloudHostedMediaFactsKey = "funcloud_hosted_media_facts"

// GetFunCloudHostedMediaFacts 返回本次请求已接受并冻结的托管素材事实。
// 键为原始引用字符串（含 asset:// 前缀）。
func GetFunCloudHostedMediaFacts(c *gin.Context) map[string]model.TaskHostedMediaFact {
	if c == nil {
		return nil
	}
	value, exists := c.Get(funCloudHostedMediaFactsKey)
	if !exists {
		return nil
	}
	facts, _ := value.(map[string]model.TaskHostedMediaFact)
	return facts
}

// ValidateFunCloudHostedVideoMedia 在资金 hold 之前解析并冻结托管素材引用。
// 非 FunCloud V3 时保持原有透传合同；V3 非托管渠道明确拒绝托管引用；跨用户、已删除或不存在的引用返回
// invalid_video_parameter，不泄漏其它用户素材的存在性。
func ValidateFunCloudHostedVideoMedia(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	if c == nil || info == nil || info.TaskRelayInfo == nil || info.ChannelMeta == nil {
		return nil
	}
	if info.ChannelType != constant.ChannelTypeSeedanceLink || info.ChannelOtherSettings.VideoUpstreamProtocol != dto.VideoUpstreamProtocolFunCloudModelArkV3 {
		return nil
	}
	contract, ok := relaycommon.GetVideoContractRequest(c)
	if !ok || contract.ModelArk == nil {
		return nil
	}
	facts := make(map[string]model.TaskHostedMediaFact)
	var storeCtx context.Context
	for _, item := range contract.ModelArk.Content {
		for slot, media := range []*dto.VideoMediaURL{item.ImageURL, item.VideoURL, item.AudioURL} {
			if media == nil {
				continue
			}
			ref := strings.TrimSpace(media.URL)
			if !strings.HasPrefix(ref, "asset://"+model.FunCloudHostedAssetIDPrefix) {
				continue
			}
			if info.ChannelOtherSettings.AssetUpstreamProtocol != dto.AssetUpstreamProtocolFunCloudHosted || item.Type != "image_url" || slot != 0 {
				return TaskErrorWrapperLocal(errors.New("hosted media requires an image reference on a hosted model"), "invalid_video_parameter", http.StatusBadRequest)
			}
			if _, accepted := facts[ref]; accepted {
				continue
			}
			assetID := strings.TrimPrefix(ref, "asset://")
			record, err := model.GetFunCloudHostedAsset(info.UserId, assetID)
			if err != nil {
				return TaskErrorWrapperLocal(err, "internal_error", http.StatusInternalServerError)
			}
			if record == nil {
				return TaskErrorWrapperLocal(
					errors.New("hosted media reference is not available"),
					"invalid_video_parameter",
					http.StatusBadRequest,
				)
			}
			if storeCtx == nil {
				storeCtx, err = WithImageObjectStore(c.Request.Context())
				if err != nil {
					return TaskErrorWrapperLocal(errors.New("hosted media storage is unavailable"), "hosted_media_unavailable", http.StatusServiceUnavailable)
				}
			}
			if err := validateFunCloudHostedObject(storeCtx, record.StorageLocation, record.ObjectKey); err != nil {
				return TaskErrorWrapperLocal(errors.New("hosted media storage is unavailable"), "hosted_media_unavailable", http.StatusServiceUnavailable)
			}
			facts[ref] = model.TaskHostedMediaFact{
				StorageLocation: record.StorageLocation,
				AssetID:         record.ID,
				ObjectKey:       record.ObjectKey,
				MimeType:        record.MimeType,
				SizeBytes:       record.SizeBytes,
			}
		}
	}
	if len(facts) > 0 {
		c.Set(funCloudHostedMediaFactsKey, facts)
		c.Request = c.Request.WithContext(storeCtx)
	}
	return nil
}

// StageFunCloudHostedMediaSnapshot 把已接受的托管素材事实写入 Task 模板，
// 由既有 attempt RecoverySnapshot 持久化；只记录对象位置，不记录签名 URL。
func StageFunCloudHostedMediaSnapshot(c *gin.Context, task *model.Task) {
	if c == nil || task == nil {
		return
	}
	facts := GetFunCloudHostedMediaFacts(c)
	if len(facts) == 0 {
		return
	}
	refs := make([]string, 0, len(facts))
	for ref := range facts {
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	task.PrivateData.HostedMedia = make([]model.TaskHostedMediaFact, 0, len(refs))
	for _, ref := range refs {
		task.PrivateData.HostedMedia = append(task.PrivateData.HostedMedia, facts[ref])
	}
}
