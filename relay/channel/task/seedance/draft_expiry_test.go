package seedance

import (
	"fmt"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
)

func TestViduDraftExpiryUsesTheDocumentedCreationCutover(t *testing.T) {
	cutover := time.Date(2026, time.October, 28, 2, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name    string
		baseURL string
		created time.Time
		life    time.Duration
	}{
		{"seven days before cutover", "https://api.vidu.cn/ent", cutover.Add(-time.Second), 7 * 24 * time.Hour},
		{"fourteen days at cutover", "https://API.vidu.cn:443/ent/", cutover, 14 * 24 * time.Hour},
	} {
		t.Run(tc.name, func(t *testing.T) {
			task := model.Task{
				PrivateData: model.TaskPrivateData{VideoUpstreamProtocol: dto.VideoUpstreamProtocolViduModelArkV3, VideoUpstreamQueryBaseURL: tc.baseURL},
				Data:        []byte(fmt.Sprintf(`{"created_at":%d}`, tc.created.Unix())),
			}
			expires := tc.created.Add(tc.life)
			assert.False(t, VideoDraftExpired(&task, expires.Add(-time.Second)))
			assert.True(t, VideoDraftExpired(&task, expires))
			task.Data = []byte(`{}`)
			assert.False(t, VideoDraftExpired(&task, expires))
		})
	}
}
