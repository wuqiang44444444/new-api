package dto

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSupportsPersistentMediaImageTaskUsesExactRouteContract(t *testing.T) {
	config := &AdvancedCustomConfig{Routes: []AdvancedCustomRoute{
		{
			IncomingPath: "/v1/images/generations",
			UpstreamPath: "/v1/images/generations",
			Converter:    AdvancedCustomConverterMediaTaskImageBlocking,
			Models:       []string{"seedream-5-moxing"},
		},
		{
			IncomingPath: "/v1/images/generations",
			UpstreamPath: "/v1/images/generations",
			Converter:    "none",
			Models:       []string{"gpt-image-2"},
		},
	}}

	assert.True(t, config.SupportsPersistentMediaImageTask("/v1/images/generations", "seedream-5-moxing"))
	assert.False(t, config.SupportsPersistentMediaImageTask("/v1/images/generations", "gpt-image-2"))
	assert.False(t, config.SupportsPersistentMediaImageTask("/v1/images/edits", "seedream-5-moxing"))
	assert.False(t, config.SupportsPersistentMediaImageTask("/v1beta/models/seedream-5-moxing:generateContent", "seedream-5-moxing"))
	assert.False(t, (*AdvancedCustomConfig)(nil).SupportsPersistentMediaImageTask("/v1/images/generations", "seedream-5-moxing"))
}
