package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

// Link 视频结果的上游读取源（本地扩展）。转存 worker 按创建时冻结的协议、
// 连接与凭据在后台读取完整结果文件；保留 SSRF 防护、冻结凭据边界与带凭据
// 时的重定向限制，不把任意 URL 交给缺少保护的通用 HTTP 客户端。

type videoResultSource struct {
	body        io.ReadCloser
	contentType string
	cancel      context.CancelFunc
}

func (s *videoResultSource) close() error {
	if s == nil {
		return nil
	}
	if s.cancel != nil {
		defer s.cancel()
	}
	if s.body == nil {
		return nil
	}
	return s.body.Close()
}

// videoResultObjectKey 生成平台侧对象键（task + part），不从上游文件名或
// 模型后缀推导。主视频与尾帧各自独立对象。
func videoResultObjectKey(taskID, part string) (string, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" || len(taskID) > 128 || strings.ContainsAny(taskID, "/\\") || strings.Contains(taskID, "..") {
		return "", errors.New("video result object namespace is invalid")
	}
	if part != model.TaskVideoResultPartVideo && part != model.TaskVideoResultPartLastFrame {
		return "", errors.New("video result part is invalid")
	}
	return fmt.Sprintf("videos/tasks/%s/%s", taskID, part), nil
}

// videoResultMimeTypeForPart 决定登记与上传使用的 MIME：尾帧按图片，主视频
// 优先采用上游响应的 Content-Type（仅接受 video/*），其余按 video/mp4。
func videoResultMimeTypeForPart(part, upstreamContentType string) string {
	if part == model.TaskVideoResultPartLastFrame {
		contentType := strings.TrimSpace(upstreamContentType)
		if strings.HasPrefix(contentType, "image/") {
			return contentType
		}
		return "image/jpeg"
	}
	contentType := strings.TrimSpace(upstreamContentType)
	if strings.HasPrefix(contentType, "video/") {
		return contentType
	}
	return "video/mp4"
}

// resolveVideoResultSource 解析指定部件的冻结上游读取源。仅覆盖托管范围内
// 的 Seedance Link 与 MiniMax Link 线路；其他线路不属于托管范围。
func resolveVideoResultSource(ctx context.Context, task *model.Task, part string) (*videoResultSource, error) {
	channel, ok := model.FrozenVideoTaskChannel(task)
	if !ok {
		return nil, errVideoResultUnavailable
	}
	source := &videoResultSource{}
	var videoURL string
	var headers http.Header
	rejectRedirects := false
	switch channel.Type {
	case constant.ChannelTypeSeedanceLink:
		if part == model.TaskVideoResultPartLastFrame {
			if task.PrivateData.VideoUpstreamProtocol != dto.VideoUpstreamProtocolSynlinkVideoV1 {
				var result struct {
					Content struct {
						LastFrameURL string `json:"last_frame_url"`
					} `json:"content"`
				}
				if err := common.Unmarshal(task.Data, &result); err != nil {
					return nil, errVideoResultUnavailable
				}
				videoURL = result.Content.LastFrameURL
				break
			}
			u, h, reject, lastFrameErr := videoResultSynlinkLastFrameSource(task)
			if lastFrameErr != nil {
				return nil, lastFrameErr
			}
			videoURL = u
			headers = h
			rejectRedirects = reject
			break
		}
		mainURL, mainHeaders, mainReject, err := videoResultSeedanceMainSource(task)
		if err != nil {
			return nil, err
		}
		videoURL = mainURL
		headers = mainHeaders
		rejectRedirects = mainReject
	case constant.ChannelTypeMiniMaxLink:
		if part == model.TaskVideoResultPartLastFrame {
			return nil, errVideoResultUnavailable
		}
		if ResolveMiniMaxVideoContentFunc == nil {
			return nil, errors.New("minimax content resolver is unavailable")
		}
		contentURL, err := ResolveMiniMaxVideoContentFunc(ctx, task)
		if err != nil {
			// 结果 URL 由冻结连接按需解析；单次解析失败不区分瞬时/终态，
			// 按可重试退避重试同一任务同一部件，不换渠道、不回退旧代理；
			// 上游内容确已失效时由累计尝试上限收敛为交付不可用。
			return nil, fmt.Errorf("minimax content source unavailable: %w", err)
		}
		videoURL = contentURL
		rejectRedirects = true
	default:
		return nil, errVideoResultUnavailable
	}
	videoURL = strings.TrimSpace(videoURL)
	if videoURL == "" {
		return nil, errVideoResultUnavailable
	}
	if strings.HasPrefix(videoURL, "data:") {
		// data: 编码不属于托管范围，如实报告交付不可用。
		return nil, errVideoResultUnavailable
	}
	if videoResultSelfLoop(videoURL, task.TaskID) {
		return nil, errVideoResultUnavailable
	}
	if err := validateVideoResultFetchURL(videoURL, channel.GetSetting().Proxy); err != nil {
		return nil, err
	}
	client := fetchClientForVideoResult(channel.GetSetting().Proxy)
	if client == nil {
		return nil, errors.New("upstream fetch client is unavailable")
	}
	if rejectRedirects {
		noRedirect := *client
		noRedirect.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		}
		client = &noRedirect
	}
	fetchCtx, cancel := context.WithTimeout(ctx, videoResultTransferTimeout)
	// cancel 的所有权在成功路径移交给 source.close()；以下错误路径必须就地
	// 取消，避免读取超时定时器在无流可管时滞留。
	req, err := http.NewRequestWithContext(fetchCtx, http.MethodGet, videoURL, nil)
	if err != nil {
		cancel()
		return nil, err
	}
	for name, values := range headers {
		for _, value := range values {
			req.Header.Add(name, value)
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		cancel()
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		statusCode := resp.StatusCode
		_ = resp.Body.Close()
		cancel()
		if statusCode == http.StatusNotFound || statusCode == http.StatusGone || statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden {
			return nil, errVideoResultUnavailable
		}
		return nil, fmt.Errorf("upstream returned HTTP %d for video result", statusCode)
	}
	source.body = resp.Body
	source.contentType = resp.Header.Get("Content-Type")
	source.cancel = cancel
	return source, nil
}

// videoResultSeedanceMainSource 解析 Seedance 主视频的冻结读取源。飞彩线路
// 必须同源于冻结查询根地址并携带冻结凭据；其余线路使用登记的 ResultURL。
func videoResultSeedanceMainSource(task *model.Task) (string, http.Header, bool, error) {
	profile := task.PrivateData.VideoUpstreamProfile
	if profile == dto.VideoUpstreamProfileThirdPartyFeicaiVideos {
		if _, err := relaycommon.ResolveVideoSouthboundAdapterVersion(
			constant.ChannelTypeSeedanceLink,
			dto.VideoUpstreamProfileThirdPartyFeicaiVideos,
			task.PrivateData.SouthboundAdapterVersion,
		); err != nil {
			return "", nil, false, errVideoResultUnavailable
		}
		key := strings.TrimSpace(task.PrivateData.Key)
		if key == "" {
			return "", nil, false, errVideoResultUnavailable
		}
		resultURL, err := relaycommon.ValidateSameOriginVideoResultURL(
			task.PrivateData.ResultURL,
			strings.TrimSpace(task.PrivateData.VideoUpstreamQueryBaseURL),
		)
		if err != nil {
			return "", nil, false, errVideoResultUnavailable
		}
		headers := http.Header{}
		headers.Set("Authorization", "Bearer "+key)
		return resultURL, headers, true, nil
	}
	return task.PrivateData.ResultURL, nil, false, nil
}

// videoResultSynlinkLastFrameSource 为 Synlink 尾帧构造冻结接口并附加冻结
// 凭据；请求与返回保持原语义（ReturnLastFrame 快照必须存在）。
func videoResultSynlinkLastFrameSource(task *model.Task) (string, http.Header, bool, error) {
	if task.PrivateData.VideoUpstreamProtocol != dto.VideoUpstreamProtocolSynlinkVideoV1 {
		return "", nil, false, errVideoResultUnavailable
	}
	if _, err := relaycommon.ResolveVideoSouthboundAdapterVersion(
		constant.ChannelTypeSeedanceLink,
		dto.VideoUpstreamProfileThirdPartySynlinkVideoV1,
		task.PrivateData.SouthboundAdapterVersion,
	); err != nil {
		return "", nil, false, errVideoResultUnavailable
	}
	baseURL := strings.TrimSpace(task.PrivateData.VideoUpstreamQueryBaseURL)
	key := strings.TrimSpace(task.PrivateData.Key)
	request := task.PrivateData.ClientRequest
	if baseURL == "" || key == "" || task.GetUpstreamTaskID() == "" ||
		request == nil || request.ReturnLastFrame == nil || !*request.ReturnLastFrame {
		return "", nil, false, errVideoResultUnavailable
	}
	contentURL := strings.TrimRight(baseURL, "/") + dto.SynlinkLastFramePath(task.GetUpstreamTaskID())
	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+key)
	return contentURL, headers, true, nil
}

// videoResultSelfLoop 检查结果 URL 是否指向本站内容代理路径（防代理循环）。
func videoResultSelfLoop(rawURL, taskID string) bool {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed == nil {
		return false
	}
	path, err := url.PathUnescape(parsed.EscapedPath())
	if err != nil {
		path = parsed.Path
	}
	return path == "/v1/videos/"+taskID+"/content" ||
		(strings.HasPrefix(path, "/v1/tasks/"+taskID+"/artifacts/") && strings.HasSuffix(path, "/content"))
}

// errVideoResultUnavailable 表示上游结果已失效或无法托管，如实记录交付
// 不可用；不回退上游直链或旧代理。
var errVideoResultUnavailable = errors.New("video result delivery is unavailable")

// validateVideoResultFetchURL 复用既有 SSRF 防护语义：无代理走拨号时校验，
// 有代理走 fetch 设置分支。任意 Provider 返回的 URL 不得绕过防护。
func validateVideoResultFetchURL(rawURL, proxy string) error {
	if proxy == "" {
		return ValidateSSRFProtectedFetchURL(rawURL)
	}
	fetchSetting := system_setting.GetFetchSetting()
	return common.ValidateURLWithFetchSetting(
		rawURL,
		fetchSetting.EnableSSRFProtection,
		fetchSetting.AllowPrivateIp,
		fetchSetting.DomainFilterMode,
		fetchSetting.IpFilterMode,
		fetchSetting.DomainList,
		fetchSetting.IpList,
		fetchSetting.AllowedPorts,
		fetchSetting.ApplyIPFilterForDomain,
	)
}

// fetchClientForVideoResult 返回带 SSRF 防护或代理客户端。
func fetchClientForVideoResult(proxy string) *http.Client {
	if proxy != "" {
		client, err := GetHttpClientWithProxy(proxy)
		if err != nil {
			return nil
		}
		return client
	}
	return GetSSRFProtectedHTTPClient()
}
