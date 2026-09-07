package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"mime/multipart"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	_ "golang.org/x/image/webp"
)

// ParseImageRelayContract has no network or storage side effects, including at async admission.
func ParseImageRelayContract(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ImageRequest, protocol dto.ImageUpstreamProtocol) (*ImageContract, *types.NewAPIError) {
	// Cache only within this HTTP request. The digest includes the mapped request
	// and execution mode, so channel/model changes cannot reuse another contract.
	var digest [32]byte
	if c != nil && c.Request != nil && info != nil && request != nil {
		clean := *request
		clean.Image, clean.Images = nil, nil
		body, err := common.Marshal(clean)
		if err != nil {
			return nil, imageContractError("invalid image request")
		}
		h := sha256.New()
		h.Write(body)
		h.Write([]byte{0})
		h.Write(request.Image)
		h.Write([]byte{0})
		h.Write(request.Images)
		copy(digest[:], h.Sum(nil))
		if value, ok := c.Get("image_relay_contract"); ok {
			cached := value.(imageRelayContractCache)
			if cached.digest == digest && cached.protocol == protocol && cached.mode == info.RelayMode && cached.request == c.Request && cached.form == c.Request.MultipartForm {
				return cached.contract, nil
			}
		}
	}

	contract, apiErr := ParseImageContract(c, info, request)
	if apiErr != nil {
		return nil, apiErr
	}
	count, limit, ok := constant.ImageRelayInputLimits(protocol, request.Model)
	if !ok {
		return nil, imageContractError("unsupported image relay model")
	}
	if len(contract.Images) > count {
		return nil, imageContractError(fmt.Sprintf("model accepts at most %d reference images", count))
	}
	for _, input := range contract.Images {
		if input.IsURL() {
			continue
		}
		if len(input.Data) > limit {
			return nil, imageContractError(fmt.Sprintf("reference image exceeds %d bytes", limit))
		}
		switch input.MimeType {
		case "image/jpeg", "image/png", "image/webp":
		default:
			return nil, imageContractError("reference images must be JPEG, PNG or WebP")
		}
		cfg, _, err := image.DecodeConfig(bytes.NewReader(input.Data))
		if err != nil {
			return nil, imageContractError("invalid reference image content")
		}
		if protocol == dto.ImageUpstreamProtocolMoxingImagesV1 && (cfg.Width <= 14 || cfg.Height <= 14 || int64(cfg.Width)*int64(cfg.Height) > 36_000_000 || float64(cfg.Width)/float64(cfg.Height) > 16 || float64(cfg.Height)/float64(cfg.Width) > 16) {
			return nil, imageContractError("reference image dimensions are outside the model limits")
		}
	}
	if c != nil && c.Request != nil && info != nil {
		c.Set("image_relay_contract", imageRelayContractCache{digest: digest, protocol: protocol, mode: info.RelayMode, request: c.Request, form: c.Request.MultipartForm, contract: contract})
	}
	return contract, nil
}

// PrepareImageUpstreamRequest is invoked only at execution, never during acceptance validation.
func PrepareImageUpstreamRequest(ctx context.Context, request any) error {
	if prepared, ok := request.(interface{ PrepareImageInputs(context.Context) error }); ok {
		return prepared.PrepareImageInputs(ctx)
	}
	return nil
}

// MultipartForm is immutable after request parsing; the cache never crosses requests.
type imageRelayContractCache struct {
	digest   [32]byte
	protocol dto.ImageUpstreamProtocol
	mode     int
	request  *http.Request
	form     *multipart.Form
	contract *ImageContract
}
