package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLegacyModelArkLogModelMappingVisibility(t *testing.T) {
	for _, tc := range []struct {
		name, path, plugin string
		hidden             bool
	}{
		{name: "create with frozen route", path: "/api/v3/contents/generations/tasks", hidden: true},
		{name: "settlement with frozen Seedance plugin", plugin: "seedance-link", hidden: true},
		{name: "settlement with frozen MiniMax plugin", plugin: "minimax-link", hidden: true},
		{name: "native video", path: "/v1/videos"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			other := common.MapToJsonStr(map[string]any{
				"task_id": "video-task", "request_path": tc.path, "is_model_mapped": true,
				"upstream_model_name": "mapped-video", "model_price": 0.04,
				"admin_info": map[string]any{"task_plugin": map[string]any{"key": tc.plugin}},
			})
			userLog := &Log{Other: other}
			formatUserLogs([]*Log{userLog}, 0)
			projected, err := common.StrToMap(userLog.Other)
			require.NoError(t, err)
			assert.Equal(t, "video-task", projected["task_id"])
			assert.Equal(t, 0.04, projected["model_price"])
			if tc.hidden {
				assert.NotContains(t, projected, "upstream_model_name")
				assert.NotContains(t, projected, "is_model_mapped")
			} else {
				assert.Equal(t, "mapped-video", projected["upstream_model_name"])
				assert.Equal(t, true, projected["is_model_mapped"])
			}
			adminLog := &Log{Other: other}
			FormatAdminLogs([]*Log{adminLog})
			assert.Contains(t, adminLog.Other, "mapped-video")
		})
	}
}
