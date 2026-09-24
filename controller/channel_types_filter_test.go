package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelTypesFilter(t *testing.T) {
	require.NoError(t, i18n.Init())
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMain, previousLog := common.MainDatabaseType(), common.LogDatabaseType()
	previousRedis := common.RedisEnabled
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMain, previousLog)
		common.RedisEnabled = previousRedis
	})
	db := setupModelListControllerTestDB(t)
	if dsn := os.Getenv("MINIMAX_MANAGEMENT_TEST_DSN"); dsn != "" {
		// Only a disposable local database is eligible for this optional matrix.
		require.Contains(t, dsn, "127.0.0.1")
		require.True(t, strings.Contains(dsn, "/minimax_management_test"))
		t.Setenv("SQL_DSN", dsn)
		wasMaster := common.IsMasterNode
		common.IsMasterNode = true
		t.Cleanup(func() { common.IsMasterNode = wasMaster })
		require.NoError(t, model.InitDB())
		db = model.DB
		sqlDB, err := db.DB()
		require.NoError(t, err)
		t.Cleanup(func() { _ = sqlDB.Close() })
	}
	for _, row := range []model.Channel{
		{Id: 1, Name: "sample-native", Type: 35, Status: 1, Group: "default", Models: "customer", Tag: common.GetPointer("a")},
		{Id: 2, Name: "sample-jd", Type: 64, Status: 1, Group: "default", Models: "customer", Tag: common.GetPointer("a")},
		{Id: 3, Name: "sample-jd-disabled", Type: 64, Status: 2, Group: "default", Models: "customer", Tag: common.GetPointer("b")},
		{Id: 4, Name: "sample-other", Type: 1, Status: 1, Group: "default", Models: "customer", Tag: common.GetPointer("c")},
		{Id: 5, Name: "sample-private", Type: 35, Status: 1, Group: "private", Models: "customer", Tag: common.GetPointer("d")},
	} {
		require.NoError(t, db.Create(&row).Error)
	}
	router := gin.New()
	router.GET("/list", GetAllChannels)
	router.GET("/search", SearchChannels)
	for _, route := range []string{"/list", "/search"} {
		for _, tc := range []struct {
			name, query string
			ids         []int
			total       int
		}{
			{"first page", "types=35,64&p=1&page_size=1", []int{1}, 3},
			{"second page", "types=35,64&p=2&page_size=1", []int{2}, 3},
			{"deduplicate", "types=35,%2064,35", []int{1, 2, 3}, 3},
			{"status", "types=35,64&status=enabled", []int{1, 2}, 2},
			{"unknown", "types=12345", []int{}, 0},
			{"legacy single", "type=35", []int{1}, 1},
			{"legacy all", "", []int{1, 2, 3, 4}, 4},
			{"tags first", "types=35,64&tag_mode=true&p=1&page_size=1", []int{1, 2}, 2},
			{"tags second", "types=35,64&tag_mode=true&p=2&page_size=1", []int{3}, 2},
		} {
			t.Run(route+"/"+tc.name, func(t *testing.T) {
				w := httptest.NewRecorder()
				router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, route+"?group=default&sort_by=id&sort_order=asc&keyword=sample&model=customer&"+tc.query, nil))
				require.Equal(t, http.StatusOK, w.Code)
				var response struct {
					Success bool `json:"success"`
					Data    struct {
						Items      []model.Channel `json:"items"`
						Total      int             `json:"total"`
						TypeCounts map[string]int  `json:"type_counts"`
					} `json:"data"`
				}
				require.NoError(t, common.Unmarshal(w.Body.Bytes(), &response))
				require.True(t, response.Success, w.Body.String())
				ids := make([]int, 0, len(response.Data.Items))
				for _, row := range response.Data.Items {
					ids = append(ids, row.Id)
				}
				assert.Equal(t, tc.ids, ids)
				assert.Equal(t, tc.total, response.Data.Total)
				assert.Equal(t, 1, response.Data.TypeCounts["1"], "type counts must ignore the selected type set")
			})
		}
		for _, query := range []string{"types=", "types=0", "types=-1", "types=35,", "types=35,no", "types=2147483648", "type=35&types=64", "type=&types=64"} {
			t.Run(route+"/invalid/"+query, func(t *testing.T) {
				w := httptest.NewRecorder()
				router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, route+"?"+query, nil))
				assert.Equal(t, http.StatusBadRequest, w.Code)
			})
		}
	}
	for _, tc := range []struct {
		query  string
		status int
	}{
		{"p=-1&page_size=20", http.StatusBadRequest},
		{"p=1&page_size=-1", http.StatusBadRequest},
		{"p=9223372036854775807&page_size=100", http.StatusOK},
	} {
		t.Run("tag pagination/"+tc.query, func(t *testing.T) {
			w := httptest.NewRecorder()
			require.NotPanics(t, func() {
				router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/search?types=35,64&tag_mode=true&"+tc.query, nil))
			})
			assert.Equal(t, tc.status, w.Code)
			if tc.status == http.StatusOK {
				var response struct {
					Data struct {
						Items []model.Channel `json:"items"`
						Total int             `json:"total"`
					} `json:"data"`
				}
				require.NoError(t, common.Unmarshal(w.Body.Bytes(), &response))
				assert.Empty(t, response.Data.Items)
				assert.Equal(t, 3, response.Data.Total)
			}
		})
	}
}

func TestMiniMaxStandardVideoRejectsChatProbe(t *testing.T) {
	result := testChannel(context.Background(), &model.Channel{Type: 64}, 1, "customer", "", false)
	require.Error(t, result.localErr)
	assert.False(t, result.upstreamAttempted)
}
