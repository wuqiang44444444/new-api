package service

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// 已登记本站托管协议的视频引用边界。在 durable hold 与 Provider POST 之前，
// 校验 asset://fhas_* 的归属、类型和状态，冻结对象事实并预签名；发送阶段
// 使用当次签名替换引用，接受后不再读取当前素材状态。不带托管前缀的
// 引用在 FunCloud 原样透传；Synlink 无法消费素材 ID，预扣前拒绝。

const funCloudHostedMediaFactsKey = "funcloud_hosted_media_facts"
const hostedVideoInputsKey = "hosted_video_inputs"

type hostedVideoURL struct {
	placeholder string
	signed      string
}

// HostedVideoProviderRequest supplies HTTPS placeholders to the normal adapter
// conversion. Actual signatures stay in the host and the northbound request
// remains available for billing and snapshots.
func HostedVideoProviderRequest(c *gin.Context, request *dto.ModelArkVideoCreateRequest) *dto.ModelArkVideoCreateRequest {
	value, _ := c.Get(hostedVideoInputsKey)
	urls, _ := value.(map[string]hostedVideoURL)
	if len(urls) == 0 {
		return request
	}
	copyRequest := *request
	copyRequest.Content = append([]dto.ModelArkVideoContent(nil), request.Content...)
	for i := range copyRequest.Content {
		item := &copyRequest.Content[i]
		for _, media := range []**dto.VideoMediaURL{&item.ImageURL, &item.AudioURL, &item.VideoURL} {
			if *media != nil {
				if prepared, ok := urls[strings.TrimSpace((*media).URL)]; ok {
					*media = &dto.VideoMediaURL{URL: prepared.placeholder}
				}
			}
		}
	}
	return &copyRequest
}

// ResolveHostedVideoProviderBody replaces whole JSON string values, including
// URLs in flattened adapter fields. An omitted reference fails before hold.
func ResolveHostedVideoProviderBody(c *gin.Context, body []byte) ([]byte, error) {
	value, _ := c.Get(hostedVideoInputsKey)
	urls, _ := value.(map[string]hostedVideoURL)
	for _, prepared := range urls {
		placeholder, err := common.Marshal(prepared.placeholder)
		if err != nil {
			return nil, err
		}
		if !bytes.Contains(body, placeholder) {
			return nil, errors.New("video adapter did not preserve the hosted media input")
		}
		signed, err := common.Marshal(prepared.signed)
		if err != nil {
			return nil, err
		}
		body = bytes.ReplaceAll(body, placeholder, signed)
	}
	return body, nil
}

// GetFunCloudHostedMediaURL returns a pre-signed URL held only for this request.
func GetFunCloudHostedMediaURL(c *gin.Context, ref string) string {
	if c == nil {
		return ""
	}
	value, _ := c.Get(hostedVideoInputsKey)
	urls, _ := value.(map[string]hostedVideoURL)
	return urls[ref].signed
}

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
// 适用于 ModelArk V3 类型化视频；跨用户、已删除或不存在的引用返回
// invalid_video_parameter，不泄漏其它用户素材的存在性。
func ValidateFunCloudHostedVideoMedia(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	if c == nil || info == nil || info.TaskRelayInfo == nil || info.ChannelMeta == nil {
		return nil
	}
	if info.ChannelType != constant.ChannelTypeSeedanceLink && info.ChannelType != constant.ChannelTypeMiniMaxLink {
		return nil
	}
	contract, ok := relaycommon.GetVideoContractRequest(c)
	if !ok || contract.ModelArk == nil {
		return nil
	}
	facts := make(map[string]model.TaskHostedMediaFact)
	prepared := make(map[string]hostedVideoURL)
	var storeCtx context.Context
	for _, item := range contract.ModelArk.Content {
		for slot, media := range []*dto.VideoMediaURL{item.ImageURL, item.VideoURL, item.AudioURL} {
			if media == nil {
				continue
			}
			ref := strings.TrimSpace(media.URL)
			if !strings.HasPrefix(ref, "asset://"+model.FunCloudHostedAssetIDPrefix) {
				if (info.ChannelOtherSettings.VideoUpstreamProtocol == dto.VideoUpstreamProtocolSynlinkVideoV1 || info.ChannelOtherSettings.VideoUpstreamProtocol == dto.VideoUpstreamProtocolViduModelArkV3) && strings.HasPrefix(ref, "asset://") {
					return TaskErrorWrapperLocal(errors.New("this video protocol requires a platform-hosted asset or a media URL"), "invalid_video_parameter", http.StatusBadRequest)
				}
				continue
			}
			mediaType := [...]string{"image", "video", "audio"}[slot]
			if item.Type != mediaType+"_url" {
				return TaskErrorWrapperLocal(errors.New("hosted media type does not match its slot"), "invalid_video_parameter", http.StatusBadRequest)
			}
			if accepted, exists := facts[ref]; exists {
				if accepted.MediaType != mediaType {
					return TaskErrorWrapperLocal(errors.New("hosted media type does not match its slot"), "invalid_video_parameter", http.StatusBadRequest)
				}
				continue
			}
			assetID := strings.TrimPrefix(ref, "asset://")
			record, err := model.GetFunCloudHostedAsset(info.UserId, assetID)
			if err != nil {
				return TaskErrorWrapperLocal(err, "internal_error", http.StatusInternalServerError)
			}
			if record == nil || record.MediaType != mediaType {
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
			fact := model.TaskHostedMediaFact{
				StorageLocation: record.StorageLocation,
				AssetID:         record.ID,
				ObjectKey:       record.ObjectKey,
				MediaType:       record.MediaType,
				MimeType:        record.MimeType,
				SizeBytes:       record.SizeBytes,
			}
			url, err := SignFunCloudHostedAssetURL(storeCtx, fact)
			if err != nil || url == "" {
				return TaskErrorWrapperLocal(errors.New("hosted media signing is unavailable"), "hosted_media_unavailable", http.StatusServiceUnavailable)
			}
			facts[ref] = fact
			prepared[ref] = hostedVideoURL{placeholder: "https://hosted-media.invalid/" + uuid.NewString(), signed: url}
		}
	}
	if len(facts) > 0 {
		c.Set(funCloudHostedMediaFactsKey, facts)
		c.Set(hostedVideoInputsKey, prepared)
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
