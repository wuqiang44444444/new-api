package dto

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdvancedCustomMediaTaskImageBlockingPathContract(t *testing.T) {
	valid := &AdvancedCustomConfig{
		Routes: []AdvancedCustomRoute{
			{
				IncomingPath: "/v1/images/generations",
				UpstreamPath: "/v1/images/generations",
				Converter:    AdvancedCustomConverterMediaTaskImageBlocking,
			},
		},
	}
	require.NoError(t, valid.Validate())
	assert.True(t, IsAdvancedCustomConverterAllowed(AdvancedCustomConverterMediaTaskImageBlocking))

	invalid := &AdvancedCustomConfig{
		Routes: []AdvancedCustomRoute{
			{
				IncomingPath: "/v1/chat/completions",
				UpstreamPath: "/v1/images/generations",
				Converter:    AdvancedCustomConverterMediaTaskImageBlocking,
			},
		},
	}
	err := invalid.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "converter does not match incoming_path")

	nativeProtocol := &AdvancedCustomConfig{
		Routes: []AdvancedCustomRoute{
			{
				IncomingPath: "/v1beta/models/{model}:generateContent",
				UpstreamPath: "/v1beta/models/{model}:generateContent",
				Converter:    AdvancedCustomConverterMediaTaskImageBlocking,
			},
		},
	}
	err = nativeProtocol.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "converter does not match incoming_path")
}
