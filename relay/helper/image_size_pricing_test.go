package helper

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeneratedImagePriceSettlesActualCountFromNativeJSONAndMultipart(t *testing.T) {
	prices := map[string]float64{
		"1K_low": 0.01667, "1K_medium": 0.05484, "1K_high": 0.18364,
		"2K_low": 0.02070, "2K_medium": 0.09119, "2K_high": 0.32907,
		"4K_low": 0.02848, "4K_medium": 0.16106, "4K_high": 0.60859,
	}
	expression, err := billingexpr.GenerateImageSizePrices("medium_three", prices)
	require.NoError(t, err)

	for _, format := range []string{"JSON generation", "multipart edit"} {
		t.Run(format, func(t *testing.T) {
			const model = "gpt-image-2-r"
			size := "3840x2160"
			mode := relayconstant.RelayModeImagesGenerations
			request := httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewBufferString(
				`{"model":"gpt-image-2-r","prompt":"draw","size":"3840x2160","quality":"high","n":2}`))
			request.Header.Set("Content-Type", "application/json")
			if format == "multipart edit" {
				size = "2160x3840"
				mode = relayconstant.RelayModeImagesEdits
				var body bytes.Buffer
				writer := multipart.NewWriter(&body)
				for key, value := range map[string]string{"model": model, "prompt": "edit", "size": size, "quality": "high", "n": "2"} {
					require.NoError(t, writer.WriteField(key, value))
				}
				file, err := writer.CreateFormFile("image", "ref.png")
				require.NoError(t, err)
				_, err = file.Write([]byte("test image"))
				require.NoError(t, err)
				require.NoError(t, writer.Close())
				request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", &body)
				request.Header.Set("Content-Type", writer.FormDataContentType())
			}
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = request
			common.SetContextKey(ctx, constant.ContextKeyChannelType, constant.ChannelTypeOpenAI)
			image, err := GetAndValidOpenAIImageRequest(ctx, mode)
			require.NoError(t, err)
			assert.Equal(t, size, image.Size)
			assert.Equal(t, "high", image.Quality)
			info := relaycommon.RelayInfo{Request: image, OriginModelName: model}
			input, err := ResolveImageBillingRequestInput(ctx, &info, billingexpr.RequestInput{})
			require.NoError(t, err)
			require.NotNil(t, input.ImageCount)
			assert.Equal(t, 2, *input.ImageCount)
			snapshot := &billingexpr.BillingSnapshot{BillingMode: "tiered_expr", ExprString: expression,
				ExprHash: billingexpr.ExprHashString(expression), QuotaPerUnit: 500000, GroupRatio: 1, EstimatedImageCount: input.ImageCount}
			estimate, err := billingexpr.ComputeTieredQuotaWithRequest(snapshot, billingexpr.TokenParams{}, input)
			require.NoError(t, err)
			assert.Equal(t, 608590, estimate.ActualQuotaAfterGroup)
			snapshot.EstimatedQuotaAfterGroup = estimate.ActualQuotaAfterGroup
			one := 1
			info.BillingRequestInput, info.TieredBillingSnapshot, info.BillingImageCount = &input, snapshot, &one
			applied, quota, result := service.TryTieredSettle(&info, billingexpr.TokenParams{P: 2399, C: 1840})
			require.True(t, applied)
			require.NotNil(t, result)
			assert.Equal(t, "4K_high", result.MatchedTier)
			assert.Equal(t, billingexpr.BillingUnitRequest, result.BillingUnit)
			assert.Equal(t, 304295, quota)
		})
	}
}
