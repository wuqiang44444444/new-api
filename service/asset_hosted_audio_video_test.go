package service

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	assetadapter "github.com/QuantumNous/new-api/relay/channel/task/seedance/assets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type hostedAudioVideoTestStore struct{ *fakeHostedImageStore }

func (s hostedAudioVideoTestStore) putVideoResultObject(ctx context.Context, key, mime string, reader io.Reader, limit int64) (int64, error) {
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return 0, err
	}
	_, err = s.putImageObject(ctx, key, mime, data)
	return int64(len(data)), err
}

func TestHostedMediaCreationValidatesFullSourceBeforePublishing(t *testing.T) {
	db := withHostedAssetDB(t)
	// MPEG-1 Layer III, 128 kbps at 44.1 kHz. The frame occupies 417 bytes.
	frame := make([]byte, 417)
	copy(frame, []byte{0xff, 0xfb, 0x90, 0x00})
	mp3Body := bytes.Repeat(frame, 2)
	box := func(name string, body []byte) []byte {
		header := binary.BigEndian.AppendUint32(nil, uint32(len(body)+8))
		return append(append(header, name...), body...)
	}
	ftyp := box("ftyp", []byte("mp42\x00\x00\x00\x00"))
	hdlr := box("hdlr", []byte("\x00\x00\x00\x00\x00\x00\x00\x00vide"))
	moov := box("moov", box("trak", box("mdia", hdlr)))
	mp4Body := bytes.Join([][]byte{ftyp, moov, box("mdat", []byte{1, 2, 3, 4})}, nil)
	pinPublishedSeedanceServiceArtifact(t)
	channel := &model.Channel{Type: constant.ChannelTypeSeedanceLink, ModelMapping: common.GetPointer(`{"hosted-model":"viduq3-drama-mini"}`)}
	channel.SetOtherSettings(dto.ChannelOtherSettings{
		VideoUpstreamProtocol: dto.VideoUpstreamProtocolViduModelArkV3,
		AssetUpstreamProtocol: dto.AssetUpstreamProtocolFunCloudHosted,
	})
	adapter, err := seedanceAssetAdapter(channel, 7, "hosted-model")
	require.NoError(t, err)

	for _, tc := range []struct {
		name, mediaType, mime string
		body                  []byte
		limit                 int64
		valid                 bool
	}{
		{"complete MP3", "audio", "audio/mpeg", mp3Body, int64(len(mp3Body)), true},
		{"partial final MP3 frame", "audio", "audio/mpeg", mp3Body[:len(mp3Body)-1], 1024, false},
		{"partial final MP3 header", "audio", "audio/mpeg", mp3Body[:len(frame)+2], 1024, false},
		{"complete MP4", "video", "video/mp4", mp4Body, int64(len(mp4Body)), true},
		{"MP4 type header only", "video", "video/mp4", ftyp, 1024, false},
		{"MP4 missing media box", "video", "video/mp4", mp4Body[:len(ftyp)+len(moov)], 1024, false},
		{"MP4 partial media box", "video", "video/mp4", mp4Body[:len(mp4Body)-1], 1024, false},
		{"MP4 beyond byte limit", "video", "video/mp4", mp4Body, int64(len(mp4Body) - 1), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var before int64
			require.NoError(t, db.Model(&model.FunCloudHostedAsset{}).Count(&before).Error)
			store := hostedAudioVideoTestStore{newFakeHostedImageStore()}
			ctx := context.WithValue(t.Context(), imageObjectSessionKey{}, &imageObjectSession{store: store})
			result, err := adapter.CreateAsset(ctx, assetadapter.AssetRequest{
				Name: tc.name, MediaType: tc.mediaType, SourceType: tc.mime,
				Source: bytes.NewReader(tc.body), SourceMaxBytes: tc.limit,
			})
			var after int64
			require.NoError(t, db.Model(&model.FunCloudHostedAsset{}).Count(&after).Error)
			if !tc.valid {
				require.ErrorIs(t, err, ErrInvalidAssetRequest)
				assert.Empty(t, result.ResourceID)
				assert.Empty(t, store.puts)
				assert.Equal(t, before, after)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, "ready", result.Status)
			assert.Equal(t, before+1, after)
			row, err := model.GetFunCloudHostedAsset(7, result.ResourceID)
			require.NoError(t, err)
			require.NotNil(t, row)
			assert.Equal(t, tc.body, store.puts[row.ObjectKey])
			assert.EqualValues(t, len(tc.body), row.SizeBytes)
		})
	}
}
