package service

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"regexp"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"

	"github.com/gin-gonic/gin"
)

// 统一图片北向合同（G1 v1，字段合同以 docs/20-architecture/图片服务与异步Provider适配架构.md §2 为准）。
// 该层只做与 Provider 无关的形状、预算与三态语义校验；族（模型）级字段生效矩阵由各
// adapter 按已登记 profile 决定。

const (
	// 统一标准合同的 n 上限（P3）。dto.MaxImageN=128 仍是无差别安全上限。
	MaxUnifiedImageN = 10
	// E5：输入图片数量与解码字节预算（按解码后字节计，不按 Base64 长度）。
	MaxImageInputs     = 14
	MaxImageInputBytes = 20 * 1024 * 1024
	MaxImageTotalBytes = 50 * 1024 * 1024
)

type ImageOperation string

const (
	ImageOperationGenerations ImageOperation = "generations"
	ImageOperationEdits       ImageOperation = "edits"
)

var imageMultipartKeyPattern = regexp.MustCompile(`^image(\[\]|\[\d+\])?$`)

// ImageContractInput is one normalized reference image. Exactly one of Data or
// URL is set; URL inputs are passed through untouched and never downloaded.
type ImageContractInput struct {
	MimeType string
	Data     []byte
	URL      string
}

func (i ImageContractInput) IsURL() bool { return i.URL != "" }

// ImageContract is the parsed, budget-checked northbound image request shared
// by every image execution path (sync Google, image relay, async acceptance).
type ImageContract struct {
	Operation      ImageOperation
	Model          string
	Prompt         string
	N              uint
	Size           string
	ResponseFormat string
	Stream         *bool
	Images         []ImageContractInput
}

// ParseImageContract validates the unified northbound shape for both image
// creation operations. Provider-specific fields stay on the original
// dto.ImageRequest for family-level checks (see RejectNonEmptyImageFields).
func ParseImageContract(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ImageRequest) (*ImageContract, *types.NewAPIError) {
	if request == nil {
		return nil, imageContractError("request is required")
	}
	operation := ImageOperationGenerations
	if info != nil && info.RelayMode == relayconstant.RelayModeImagesEdits {
		operation = ImageOperationEdits
	}
	contract := &ImageContract{
		Operation:      operation,
		Model:          strings.TrimSpace(request.Model),
		Prompt:         request.Prompt,
		Size:           strings.TrimSpace(request.Size),
		ResponseFormat: strings.TrimSpace(request.ResponseFormat),
		Stream:         request.Stream,
	}
	if contract.Model == "" {
		return nil, imageContractError("model is required")
	}
	if strings.TrimSpace(request.Prompt) == "" {
		return nil, imageContractError("prompt is required")
	}

	// n：未传或 null 为 1；显式 0（含原生解析器归一前的记录，评审 S8）
	// 或超过统一标准上限都是 400（E6/P3），不把显式 0 当默认值，也不循环
	// 调用或钳制。
	if request.NExplicitZero {
		return nil, imageContractError(fmt.Sprintf("n must be an integer between 1 and %d", MaxUnifiedImageN))
	}
	if request.N != nil {
		if *request.N == 0 || *request.N > MaxUnifiedImageN {
			return nil, imageContractError(fmt.Sprintf("n must be an integer between 1 and %d", MaxUnifiedImageN))
		}
		contract.N = *request.N
	} else {
		contract.N = 1
	}

	switch contract.ResponseFormat {
	case "", "url", "b64_json":
	default:
		return nil, imageContractError("response_format must be url or b64_json")
	}

	if imageMaskPresent(c, request) {
		// E2/C3：v1 没有任何族能真实履约 mask 语义，统一拒绝，不降级为提示词。
		return nil, imageContractError("mask is not supported by any published image model contract")
	}

	images, apiErr := collectImageInputs(c, operation, request)
	if apiErr != nil {
		return nil, apiErr
	}
	contract.Images = images
	if contract.Operation == ImageOperationEdits && len(images) == 0 {
		return nil, imageContractError("edits require at least one input image")
	}
	if contract.Operation == ImageOperationGenerations && len(images) > 0 {
		return nil, imageContractError("input images are only accepted by /v1/images/edits")
	}
	if len(images) > MaxImageInputs {
		return nil, imageContractError(fmt.Sprintf("at most %d input images are accepted", MaxImageInputs))
	}

	totalBytes := 0
	for _, image := range images {
		if image.IsURL() {
			continue // URL 只计数，不下载探测（U1/U2）
		}
		if len(image.Data) > MaxImageInputBytes {
			return nil, imageContractError("a single input image must not exceed 20 MB after decoding")
		}
		totalBytes += len(image.Data)
	}
	if totalBytes > MaxImageTotalBytes {
		return nil, imageContractError("input images must not exceed 50 MB in total after decoding")
	}
	return contract, nil
}

// RejectNonEmptyImageFields fails closed when any named standard field carries
// an explicit non-empty value the family has not published (P7—P11/C3).
// Empty strings, absent fields, and null are all treated as unset (E6).
func RejectNonEmptyImageFields(request *dto.ImageRequest, fieldNames ...string) *types.NewAPIError {
	if request == nil {
		return nil
	}
	for _, name := range fieldNames {
		nonEmpty := false
		switch name {
		case "quality":
			nonEmpty = strings.TrimSpace(request.Quality) != ""
		case "style":
			nonEmpty = rawMessageNonEmpty(request.Style)
		case "user":
			nonEmpty = rawMessageNonEmpty(request.User)
		case "background":
			nonEmpty = rawMessageNonEmpty(request.Background)
		case "moderation":
			nonEmpty = rawMessageNonEmpty(request.Moderation)
		case "output_format":
			nonEmpty = rawMessageNonEmpty(request.OutputFormat)
		case "output_compression":
			nonEmpty = rawMessageNonEmpty(request.OutputCompression)
		case "partial_images":
			nonEmpty = rawMessageNonEmpty(request.PartialImages)
		case "input_fidelity":
			nonEmpty = rawMessageNonEmpty(request.InputFidelity)
		case "watermark":
			nonEmpty = request.Watermark != nil
		case "watermark_enabled":
			nonEmpty = rawMessageNonEmpty(request.WatermarkEnabled)
		case "user_id":
			nonEmpty = rawMessageNonEmpty(request.UserId)
		case "extra_fields":
			nonEmpty = rawMessageNonEmpty(request.ExtraFields)
		}
		if nonEmpty {
			return imageContractError(fmt.Sprintf("%s is not supported by this image model contract", name))
		}
	}
	return nil
}

// RejectExtraJSONFields rejects unknown top-level JSON fields for contract
// families published with additional_properties=false.
func RejectExtraJSONFields(request *dto.ImageRequest) *types.NewAPIError {
	if request != nil && len(request.Extra) > 0 {
		names := make([]string, 0, len(request.Extra))
		for name := range request.Extra {
			names = append(names, name)
		}
		sort.Strings(names)
		return imageContractError(fmt.Sprintf("unsupported image request fields: %s", strings.Join(names, ", ")))
	}
	return nil
}

func rawMessageNonEmpty(raw []byte) bool {
	trimmed := strings.TrimSpace(string(raw))
	return trimmed != "" && trimmed != "null" && trimmed != `""`
}

func collectImageInputs(c *gin.Context, operation ImageOperation, request *dto.ImageRequest) ([]ImageContractInput, *types.NewAPIError) {
	var inputs []ImageContractInput
	var binaryBytes int

	if c != nil && c.Request != nil && c.Request.MultipartForm != nil {
		if operation == ImageOperationGenerations {
			// 文件字段只属于 edits；generations 的 multipart 只承载标量。
			for name := range c.Request.MultipartForm.File {
				if imageMultipartKeyPattern.MatchString(name) || name == "mask" {
					return nil, imageContractError("file fields are only accepted by /v1/images/edits")
				}
			}
		}
		for _, fileHeader := range sortedImageFileHeaders(c.Request.MultipartForm.File) {
			input, size, apiErr := multipartImageInput(fileHeader)
			if apiErr != nil {
				return nil, apiErr
			}
			binaryBytes += size
			if binaryBytes > MaxImageTotalBytes {
				return nil, imageContractError("input images must not exceed 50 MB in total after decoding")
			}
			inputs = append(inputs, input)
		}
	}

	jsonInputs, apiErr := jsonImageInputs(request)
	if apiErr != nil {
		return nil, apiErr
	}
	inputs = append(inputs, jsonInputs...)

	// 单图 image 字段（multipart form value 被 valid_request 序列化进
	// request.Image，或 JSON 直接传字符串）与 images 数组互斥（E6）。
	if len(inputs) > 0 && rawMessageNonEmpty(request.Image) {
		return nil, imageContractError("image and images must not be combined")
	}
	if len(inputs) == 0 {
		single, apiErr := singleImageFieldInput(request.Image)
		if apiErr != nil {
			return nil, apiErr
		}
		if single != nil {
			inputs = append(inputs, *single)
		}
	}
	return inputs, nil
}

func sortedImageFileHeaders(files map[string][]*multipart.FileHeader) []*multipart.FileHeader {
	names := make([]string, 0, len(files))
	for name := range files {
		if !imageMultipartKeyPattern.MatchString(name) {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	headers := make([]*multipart.FileHeader, 0, len(names))
	for _, name := range names {
		headers = append(headers, files[name]...)
	}
	return headers
}

func multipartImageInput(header *multipart.FileHeader) (ImageContractInput, int, *types.NewAPIError) {
	if header.Size > MaxImageInputBytes {
		return ImageContractInput{}, 0, imageContractError("a single input image must not exceed 20 MB after decoding")
	}
	file, err := header.Open()
	if err != nil {
		return ImageContractInput{}, 0, imageContractError("failed to read input image")
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, MaxImageInputBytes+1))
	if err != nil {
		return ImageContractInput{}, 0, imageContractError("failed to read input image")
	}
	if len(data) > MaxImageInputBytes {
		return ImageContractInput{}, 0, imageContractError("a single input image must not exceed 20 MB after decoding")
	}
	mimeType := strings.TrimSpace(header.Header.Get("Content-Type"))
	if !isAllowedImageMime(mimeType) {
		mimeType = http.DetectContentType(data)
	}
	if !isAllowedImageMime(mimeType) {
		return ImageContractInput{}, 0, imageContractError("input images must be PNG, JPEG, or WebP")
	}
	return ImageContractInput{MimeType: mimeType, Data: data}, len(data), nil
}

func jsonImageInputs(request *dto.ImageRequest) ([]ImageContractInput, *types.NewAPIError) {
	if !rawMessageNonEmpty(request.Images) {
		return nil, nil
	}
	// json.RawMessage 逐项原文解析；[][]byte 会被 encoding/json 当作
	// base64 字符串解码，导致字符串引用全部失败。
	var rawItems []json.RawMessage
	if err := common.Unmarshal(request.Images, &rawItems); err != nil {
		return nil, imageContractError("images must be an array of image references")
	}
	inputs := make([]ImageContractInput, 0, len(rawItems))
	for _, item := range rawItems {
		input, apiErr := singleImageFieldInput(item)
		if apiErr != nil {
			return nil, apiErr
		}
		if input == nil {
			return nil, imageContractError("images must be an array of image references")
		}
		inputs = append(inputs, *input)
	}
	return inputs, nil
}

func singleImageFieldInput(raw []byte) (*ImageContractInput, *types.NewAPIError) {
	if !rawMessageNonEmpty(raw) {
		return nil, nil
	}
	var value string
	if err := common.Unmarshal(raw, &value); err != nil {
		return nil, imageContractError("image references must be strings")
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	if strings.HasPrefix(value, "data:") {
		mimeType, data, err := decodeDataURLImage(value)
		if err != nil {
			return nil, imageContractError(err.Error())
		}
		if len(data) > MaxImageInputBytes {
			return nil, imageContractError("a single input image must not exceed 20 MB after decoding")
		}
		return &ImageContractInput{MimeType: mimeType, Data: data}, nil
	}
	if strings.HasPrefix(strings.ToLower(value), "https://") {
		// U1：HTTPS URL 原样透传给 Provider，不下载、不探测、不改写。
		return &ImageContractInput{URL: value}, nil
	}
	return nil, imageContractError("image references must be data URLs or HTTPS URLs")
}

func decodeDataURLImage(value string) (string, []byte, error) {
	rest := strings.TrimPrefix(value, "data:")
	comma := strings.Index(rest, ",")
	if comma < 0 {
		return "", nil, errors.New("image data URL is malformed")
	}
	meta := rest[:comma]
	payload := rest[comma+1:]
	mimeType := ""
	isBase64 := false
	for _, part := range strings.Split(meta, ";") {
		part = strings.TrimSpace(part)
		if strings.EqualFold(part, "base64") {
			isBase64 = true
			continue
		}
		if strings.Contains(part, "/") {
			mimeType = strings.ToLower(part)
		}
	}
	if !isAllowedImageMime(mimeType) {
		return "", nil, errors.New("input images must be PNG, JPEG, or WebP data URLs")
	}
	if !isBase64 {
		return "", nil, errors.New("image data URL must use base64 encoding")
	}
	data, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		// 容忍缺失 padding 与 URL-safe 变体，但拒绝一切解码失败。
		if data, err = base64.RawStdEncoding.DecodeString(strings.TrimRight(payload, "=")); err != nil {
			if data, err = base64.URLEncoding.DecodeString(payload); err != nil {
				return "", nil, errors.New("image data URL could not be decoded")
			}
		}
	}
	if len(data) > MaxImageInputBytes {
		return "", nil, errors.New("a single input image must not exceed 20 MB after decoding")
	}
	if !isAllowedImageMime(http.DetectContentType(data)) {
		return "", nil, errors.New("input images must be PNG, JPEG, or WebP")
	}
	return mimeType, data, nil
}

func imageMaskPresent(c *gin.Context, request *dto.ImageRequest) bool {
	if rawMessageNonEmpty(request.Mask) {
		return true
	}
	if c != nil && c.Request != nil && c.Request.MultipartForm != nil {
		if _, ok := c.Request.MultipartForm.File["mask"]; ok {
			return true
		}
		if len(c.Request.MultipartForm.Value["mask"]) > 0 && strings.TrimSpace(c.Request.MultipartForm.Value["mask"][0]) != "" {
			return true
		}
	}
	return false
}

func isAllowedImageMime(mimeType string) bool {
	switch strings.ToLower(strings.TrimSpace(strings.Split(mimeType, ";")[0])) {
	case "image/png", "image/jpeg", "image/jpg", "image/webp":
		return true
	default:
		return false
	}
}

func imageContractError(message string) *types.NewAPIError {
	return types.NewErrorWithStatusCode(
		errors.New(message),
		types.ErrorCodeInvalidRequest,
		http.StatusBadRequest,
		types.ErrOptionWithSkipRetry(),
	)
}

// PreferRespondAsync reports whether the client explicitly selected the async
// image execution mode. Only the exact respond-async token counts (P14/B4).
func PreferRespondAsync(c *gin.Context) bool {
	if c == nil || c.Request == nil {
		return false
	}
	for _, preference := range strings.Split(c.GetHeader("Prefer"), ",") {
		token := strings.ToLower(strings.TrimSpace(strings.SplitN(strings.TrimSpace(preference), ";", 2)[0]))
		if token == "respond-async" {
			return true
		}
	}
	return false
}

// ImageCreateOperationFromPath resolves the idempotency operation namespace
// for the current image create route.
func ImageCreateOperationFromPath(c *gin.Context) ImageOperation {
	if c != nil && c.Request != nil && strings.HasPrefix(c.Request.URL.Path, "/v1/images/edits") {
		return ImageOperationEdits
	}
	return ImageOperationGenerations
}
