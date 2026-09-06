package gemini

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"

	"github.com/gin-gonic/gin"
)

// Gemini/Vertex 标准图片入口的 generateContent 转换与响应核心（G1 v1）。
// imagen :predict 路径与本文件互不重叠；支持的模型由 model_setting 的
// imagine 模型登记决定，不从名称片段另行推断。

// geminiImageResponseFormatKey 把转换阶段解析出的 response_format 传递给
// 同一请求的响应处理阶段。
const geminiImageResponseFormatKey = "gemini_image_response_format"
const geminiImageSizeKey = "gemini_image_size"

// SetupGenerateContentImageHeader describes the converted JSON body, regardless
// of whether the caller supplied JSON or multipart image inputs.
func SetupGenerateContentImageHeader(c *gin.Context, header *http.Header) {
	if c != nil {
		if _, converted := c.Get(geminiImageResponseFormatKey); converted {
			header.Set("Content-Type", "application/json")
		}
	}
}

// SupportsGenerateContentImage reports whether the mapped upstream model is a
// registered imagine model served through generateContent.
func SupportsGenerateContentImage(upstreamModel string) bool {
	return model_setting.IsGeminiModelSupportImagine(upstreamModel)
}

// ConvertImageRequestToGenerateContent converts the unified northbound image
// contract into a Gemini generateContent request (G1 §2.1).
func ConvertImageRequestToGenerateContent(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	contract, apiErr := ParseGeminiImageContract(c, info, &request)
	if apiErr != nil {
		return nil, apiErr
	}
	geminiRequest, err := BuildGenerateContentImageRequest(contract)
	if err != nil {
		return nil, err
	}
	if c != nil {
		c.Set(geminiImageResponseFormatKey, contract.ResponseFormat)
		c.Set(geminiImageSizeKey, contract.Size)
	}
	if info != nil && info.UpstreamModelName == "" {
		info.UpstreamModelName = contract.Model
	}
	return geminiRequest, nil
}

// ParseGeminiImageContract runs the unified contract parse plus the Google
// family matrix (G1 §2.1)：不支持的合同外字段一律显式拒绝。
func ParseGeminiImageContract(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ImageRequest) (*GeminiImageContract, *types.NewAPIError) {
	contract, apiErr := service.ParseImageContract(c, info, request)
	if apiErr != nil {
		return nil, apiErr
	}
	if contract.N != 1 {
		// P3：Google 每响应一图；在 Provider 调用前拒绝，不循环、不钳制。
		return nil, GeminiImageBadRequest("n must be 1 for gemini image models")
	}
	if apiErr := service.RejectNonEmptyImageFields(request,
		"quality", "style", "background", "moderation", "output_format", "output_compression",
		"partial_images", "input_fidelity", "watermark", "watermark_enabled", "user_id", "extra_fields",
	); apiErr != nil {
		return nil, apiErr
	}
	if apiErr := service.RejectExtraJSONFields(request); apiErr != nil {
		return nil, apiErr
	}
	if contract.Stream != nil && *contract.Stream {
		// P14：显式 stream=true 未发布（无 SSE 中间成品，不拿 thought 冒充）。
		return nil, GeminiImageBadRequest("stream is not supported for gemini image models")
	}
	if !IsValidGeminiImageSizeToken(contract.Size) {
		// E6：非法 size 显式报错，不静默回退 Provider 默认（评审 S10）。
		return nil, GeminiImageBadRequest("size must be auto or a supported WxH size with an exact aspect ratio (dimensions up to 4096)")
	}
	if contract.ResponseFormat == "url" {
		ctx := context.Background()
		if c != nil && c.Request != nil {
			ctx = c.Request.Context()
		}
		storeCtx, err := service.WithImageObjectStore(ctx)
		if err != nil {
			return nil, GeminiImageBadRequest("response_format=url requires the platform artifact store")
		}
		if c != nil && c.Request != nil {
			c.Request = c.Request.WithContext(storeCtx)
		}
	}
	parsed := &GeminiImageContract{
		Operation: contract.Operation,
		Model:     contract.Model,
		Prompt:    contract.Prompt,
		Size:      contract.Size,
		Images:    contract.Images,
	}
	// G1：本族默认 b64_json（P13）。显式 url 在存储启用时由响应阶段落位 OSS。
	parsed.ResponseFormat = contract.ResponseFormat
	if parsed.ResponseFormat == "" {
		parsed.ResponseFormat = "b64_json"
	}
	return parsed, nil
}

// GeminiImageContract is the family-checked contract consumed by
// BuildGenerateContentImageRequest.
type GeminiImageContract struct {
	Operation      service.ImageOperation
	Model          string
	Prompt         string
	Size           string
	ResponseFormat string
	Images         []service.ImageContractInput
}

// BuildGenerateContentImageRequest is the pure conversion core shared by the
// sync relay path and the async image worker.
func BuildGenerateContentImageRequest(contract *GeminiImageContract) (*dto.GeminiChatRequest, error) {
	parts := make([]dto.GeminiPart, 0, len(contract.Images)+1)
	parts = append(parts, dto.GeminiPart{Text: contract.Prompt})
	for _, image := range contract.Images {
		if image.IsURL() {
			// U1/U4：公开 HTTPS URL 以 fileData 透传，MIME 允许缺省，
			// 不下载探测、不按后缀猜测。
			parts = append(parts, dto.GeminiPart{FileData: &dto.GeminiFileData{FileUri: image.URL}})
			continue
		}
		parts = append(parts, dto.GeminiPart{InlineData: &dto.GeminiInlineData{
			MimeType: image.MimeType,
			Data:     base64.StdEncoding.EncodeToString(image.Data),
		}})
	}

	generationConfig := dto.GeminiChatGenerationConfig{
		ResponseModalities: []string{"TEXT", "IMAGE"},
	}
	if aspectRatio, imageSize, ok := SizeToGeminiImageConfig(contract.Size); ok {
		imageConfig := map[string]any{"aspectRatio": aspectRatio}
		if imageSize != "" {
			imageConfig["imageSize"] = imageSize
		}
		encoded, err := common.Marshal(imageConfig)
		if err != nil {
			return nil, err
		}
		generationConfig.ImageConfig = encoded
	}

	return &dto.GeminiChatRequest{
		Contents: []dto.GeminiChatContent{
			{Role: "user", Parts: parts},
		},
		GenerationConfig: generationConfig,
	}, nil
}

// SizeToGeminiImageConfig maps the standard size token onto Google's
// aspect-ratio + resolution-tier vocabulary (P4/P5)。ok=false 表示使用
// Provider 默认（不发送 imageConfig）。
func SizeToGeminiImageConfig(size string) (aspectRatio, imageSize string, ok bool) {
	size = strings.TrimSpace(size)
	if size == "" || strings.EqualFold(size, "auto") {
		return "", "", false
	}

	width, height, err := parsePixelSize(size)
	if err != nil {
		return "", "", false
	}
	ratio := PixelSizeToGeminiAspectRatio(width, height)
	return ratio, PixelSizeToGeminiImageSize(width, height), ratio != "" && width <= 4096 && height <= 4096
}

// IsValidGeminiImageSizeToken 报告 size 是否属于本族已发布取值：空、
// auto 或精确支持比例的 WxH；不接受 Provider 私有比例字符串。
func IsValidGeminiImageSizeToken(size string) bool {
	size = strings.TrimSpace(size)
	if size == "" || strings.EqualFold(size, "auto") {
		return true
	}
	w, h, err := parsePixelSize(size)
	return err == nil && w <= 4096 && h <= 4096 && PixelSizeToGeminiAspectRatio(w, h) != ""
}

func parsePixelSize(size string) (int, int, error) {
	lower := strings.ToLower(size)
	x := strings.Index(lower, "x")
	if x <= 0 || x == len(size)-1 {
		return 0, 0, fmt.Errorf("invalid size %q", size)
	}
	width, err := atoiPositive(size[:x])
	if err != nil {
		return 0, 0, err
	}
	height, err := atoiPositive(size[x+1:])
	if err != nil {
		return 0, 0, err
	}
	return width, height, nil
}

func atoiPositive(value string) (int, error) {
	parsed := 0
	for _, r := range value {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("invalid number %q", value)
		}
		parsed = parsed*10 + int(r-'0')
		if parsed > 1_000_000 {
			return 0, fmt.Errorf("size dimension %q is out of range", value)
		}
	}
	if parsed == 0 {
		return 0, fmt.Errorf("size dimension must be positive")
	}
	return parsed, nil
}

// PixelSizeToGeminiAspectRatio picks Google's closest supported aspect ratio
// without cropping or stretching (P5)。
func PixelSizeToGeminiAspectRatio(width, height int) string {
	for _, ratio := range [][2]int{{1, 1}, {2, 3}, {3, 2}, {3, 4}, {4, 3}, {4, 5}, {5, 4}, {9, 16}, {16, 9}, {21, 9}} {
		if width*ratio[1] == height*ratio[0] {
			return fmt.Sprintf("%d:%d", ratio[0], ratio[1])
		}
	}
	return ""
}

// PixelSizeToGeminiImageSize picks the resolution tier by long edge (P4)。
func PixelSizeToGeminiImageSize(width, height int) string {
	longEdge := width
	if height > longEdge {
		longEdge = height
	}
	switch {
	case longEdge >= 3000:
		return "4K"
	case longEdge >= 1700:
		return "2K"
	default:
		return "1K"
	}
}

// GeminiImageResult is one final provider image (headless execution shape).
type GeminiImageResult struct {
	MimeType string
	Data     []byte
}

// ParseGenerateContentImageResponseBody is the headless core shared with the
// async image worker: final-image extraction (thought 排除), safety-rejection
// surfacing (R1/R5), and trusted usageMetadata normalization (R4)。
func ParseGenerateContentImageResponseBody(c *gin.Context, info *relaycommon.RelayInfo, body []byte) ([]GeminiImageResult, *dto.Usage, *types.NewAPIError) {
	var geminiResponse dto.GeminiChatResponse
	if err := common.Unmarshal(body, &geminiResponse); err != nil {
		return nil, nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	finalImages := collectGeminiFinalImages(&geminiResponse)
	if len(finalImages) == 0 {
		return nil, nil, geminiImageEmptyResultError(&geminiResponse)
	}
	usage := buildUsageFromGeminiMetadata(geminiResponse.GetUsageMetadata(), 0)

	results := make([]GeminiImageResult, 0, len(finalImages))
	for _, image := range finalImages {
		data, err := base64.StdEncoding.DecodeString(image.data)
		if err != nil {
			return nil, nil, types.NewError(err, types.ErrorCodeBadResponseBody)
		}
		result, err := normalizeGeminiImagePixels(c, GeminiImageResult{MimeType: image.mimeType, Data: data})
		if err != nil {
			return nil, nil, types.NewError(err, types.ErrorCodeBadResponseBody, types.ErrOptionWithSkipRetry())
		}
		results = append(results, result)
	}
	if !geminiResponse.HasUsageMetadata {
		return results, nil, nil
	}
	return results, &usage, nil
}

// GeminiGenerateContentImageHandler converts a generateContent image response
// into the OpenAI Images shape (R1—R3)：thought parts 排除、安全拒绝显式
// 失败、usage 采用可信 usageMetadata。
func GeminiGenerateContentImageHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(errors.New("empty provider response"), types.ErrorCodeBadResponseBody, http.StatusBadGateway)
	}
	responseBody, readErr := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if readErr != nil {
		return nil, types.NewOpenAIError(readErr, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	decoded, usage, apiErr := ParseGenerateContentImageResponseBody(c, info, responseBody)
	if apiErr != nil {
		return nil, apiErr
	}

	response := dto.ImageResponse{
		Created: common.GetTimestamp(),
		Data:    make([]dto.ImageData, 0, len(decoded)),
	}
	responseFormat := geminiImageRequestedResponseFormat(c)
	if responseFormat == "url" {
		storeCtx, err := service.WithImageObjectStore(c.Request.Context())
		if err != nil {
			return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
		}
		c.Request = c.Request.WithContext(storeCtx)
	}
	for _, image := range decoded {
		if responseFormat == "url" {
			url, apiErr := hostGeminiImageResult(c, geminiFinalImage{mimeType: image.MimeType, data: base64.StdEncoding.EncodeToString(image.Data)})
			if apiErr != nil {
				return nil, apiErr
			}
			response.Data = append(response.Data, dto.ImageData{Url: url})
			continue
		}
		response.Data = append(response.Data, dto.ImageData{B64Json: base64.StdEncoding.EncodeToString(image.Data)})
	}

	jsonResponse, err := common.Marshal(response)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(http.StatusOK)
	_, _ = c.Writer.Write(jsonResponse)
	// The native synchronous ImageHelper expects a non-nil usage object.
	// The headless parser still returns nil when upstream usage is absent.
	if usage == nil {
		usage = &dto.Usage{}
	}
	return usage, nil
}

type geminiFinalImage struct {
	mimeType string
	data     string // base64
}

// collectGeminiFinalImages keeps only final image parts：thought 标记、纯
// 文本与非图片 inlineData 一律排除（R1）。
func collectGeminiFinalImages(response *dto.GeminiChatResponse) []geminiFinalImage {
	var images []geminiFinalImage
	for _, candidate := range response.Candidates {
		for _, part := range candidate.Content.Parts {
			if part.Thought || part.InlineData == nil {
				continue
			}
			mimeType := strings.ToLower(strings.TrimSpace(part.InlineData.MimeType))
			if !strings.HasPrefix(mimeType, "image/") || part.InlineData.Data == "" {
				continue
			}
			images = append(images, geminiFinalImage{mimeType: mimeType, data: part.InlineData.Data})
		}
	}
	return images
}

// geminiImageEmptyResultError distinguishes safety rejections from empty or
// truncated responses so none can masquerade as a successful empty array
// (R1/R5)。
func geminiImageEmptyResultError(response *dto.GeminiChatResponse) *types.NewAPIError {
	if response.PromptFeedback != nil && response.PromptFeedback.BlockReason != nil {
		return types.NewErrorWithStatusCode(
			fmt.Errorf("request blocked by provider safety policy: %s", *response.PromptFeedback.BlockReason),
			types.ErrorCodePromptBlocked,
			http.StatusBadRequest,
			types.ErrOptionWithSkipRetry(),
		)
	}
	for _, candidate := range response.Candidates {
		if candidate.FinishReason != nil && isSafetyFinishReason(*candidate.FinishReason) {
			return types.NewErrorWithStatusCode(
				fmt.Errorf("image generation blocked by provider safety policy: %s", *candidate.FinishReason),
				types.ErrorCodePromptBlocked,
				http.StatusBadRequest,
				types.ErrOptionWithSkipRetry(),
			)
		}
	}
	return types.NewErrorWithStatusCode(
		errors.New("provider returned no final image"),
		types.ErrorCodeEmptyResponse,
		http.StatusBadGateway,
		types.ErrOptionWithSkipRetry(),
	)
}

func isSafetyFinishReason(reason string) bool {
	switch strings.ToUpper(strings.TrimSpace(reason)) {
	case "SAFETY", "PROHIBITED_CONTENT", "BLOCKLIST", "RECITATION":
		return true
	default:
		return false
	}
}

func hostGeminiImageResult(c *gin.Context, image geminiFinalImage) (string, *types.NewAPIError) {
	data, err := base64.StdEncoding.DecodeString(image.data)
	if err != nil {
		return "", types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	url, apiErr := service.PutEphemeralImageResult(c.Request.Context(), image.mimeType, data)
	if apiErr != nil {
		return "", apiErr
	}
	return url, nil
}

func geminiImageRequestedResponseFormat(c *gin.Context) string {
	if c == nil {
		return "b64_json"
	}
	if value, exists := c.Get(geminiImageResponseFormatKey); exists {
		if format, ok := value.(string); ok && (format == "url" || format == "b64_json") {
			return format
		}
	}
	return "b64_json"
}

// GeminiImageBadRequest is the family-level 400 used before any provider call.
func GeminiImageBadRequest(message string) *types.NewAPIError {
	return types.NewErrorWithStatusCode(
		errors.New(message),
		types.ErrorCodeInvalidRequest,
		http.StatusBadRequest,
		types.ErrOptionWithSkipRetry(),
	)
}
