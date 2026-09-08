package seedance

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 托管引用在发送前必须已经解析为内部 URL；解析缺口失败关闭，绝不把平台
// 命名空间泄漏给上游。
func TestFunCloudHostedRefWithoutFactsFailsClosed(t *testing.T) {
	payload := &requestPayload{Model: "seedance-2-5", Content: []ContentItem{{
		Type: "image_url", Role: "reference_image",
		ImageURL: &MediaURL{URL: "asset://" + model.FunCloudHostedAssetIDPrefix + "unresolved"},
	}}}
	_, err := buildFunCloudModelArkRequest(nil, payload)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "hosted asset reference")
}
