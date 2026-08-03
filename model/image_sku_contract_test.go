package model

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImageSKUCapabilityValidatesPublishedContract(t *testing.T) {
	tests := []struct {
		name      string
		model     string
		size      string
		images    any
		extra     map[string]json.RawMessage
		watermark *bool
		wantError string
	}{
		{name: "Moxing Seedream", model: "seedream-5-moxing", size: "3K", images: []string{"https://cdn.example/1.png"}, watermark: common.GetPointer(true)},
		{name: "Qihang two references", model: "seedream-5-qihang", size: "2K", images: []string{"https://cdn.example/1.png", "https://cdn.example/2.png"}},
		{name: "Qihang rejects third reference", model: "seedream-5-qihang", size: "2K", images: []string{"https://cdn.example/1.png", "https://cdn.example/2.png", "https://cdn.example/3.png"}, wantError: "more than 2"},
		{name: "Nano aspect ratio", model: "nano-banana-2", size: "4K", extra: map[string]json.RawMessage{"aspect_ratio": json.RawMessage(`"21:9"`)}},
		{name: "Nano rejects watermark", model: "nano-banana-2", size: "2K", watermark: common.GetPointer(false), wantError: "watermark is not supported"},
		{name: "Seedream rejects Nano size", model: "seedream-5-moxing", size: "4K", wantError: "size must be 2K or 3K"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			capability, ok := ResolveImageSKUCapability(test.model)
			require.True(t, ok)
			var image json.RawMessage
			if test.images != nil {
				image, _ = common.Marshal(test.images)
			}
			request := &dto.ImageRequest{
				Model: test.model, Prompt: "create an image", Size: test.size, N: common.GetPointer(uint(1)),
				Image: image, Extra: test.extra, Watermark: test.watermark,
			}
			err := capability.ValidateRequest(request)
			if test.wantError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), test.wantError)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestMediaImageTaskSnapshotRequiresCurrentLinkContract(t *testing.T) {
	capability, ok := ResolveImageSKUCapability("seedream-5-moxing")
	require.True(t, ok)
	implementation, ok := ResolveLinkImplementation(dto.LinkImplementationRef{
		ID: LinkImplementationMoxingImages, Version: LinkImplementationVersionV1,
	})
	require.True(t, ok)
	task := &Task{
		ClientProtocol: TaskClientProtocolOpenAIImages,
		Properties:     Properties{OriginModelName: capability.PublicModel},
		PrivateData: TaskPrivateData{
			NorthboundContractID: capability.ContractID, NorthboundContractVersion: capability.Version,
			SKUCapabilityVersion: capability.Version, SKUCapabilityHash: capability.ContentHash,
			LinkImplementationID: implementation.ID, LinkImplementationVersion: implementation.Version,
			LinkImplementationHash: implementation.ContentHash,
		},
	}
	assert.True(t, MediaImageTaskSnapshotIsCurrent(task))
	task.PrivateData.LinkImplementationHash = "stale"
	assert.False(t, MediaImageTaskSnapshotIsCurrent(task))
	task.PrivateData.LinkImplementationHash = implementation.ContentHash
	task.PrivateData.SKUCapabilityHash = "stale"
	assert.False(t, MediaImageTaskSnapshotIsCurrent(task))
}
