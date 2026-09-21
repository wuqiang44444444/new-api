package controller

import (
	"fmt"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupBillingURLNameControllerTest(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMain, previousLog := common.MainDatabaseType(), common.LogDatabaseType()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "billing-url.db")), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMain, previousLog)
		_ = sqlDB.Close()
	})
	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.ProviderChannelBillingDiscount{}, &model.ProviderBillingAudit{}, &model.ProviderURLGroupDisplayName{}))
	gin.SetMode(gin.TestMode)
	return db
}

func newBillingAdminTestRouter() *gin.Engine {
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("id", 9); c.Set("role", common.RoleRootUser) })
	router.PUT("/discounts", PutAdminProviderBillingDiscount)
	router.PUT("/names", PutAdminUpstreamURLGroupName)
	return router
}

func putBillingAdminJSON(router *gin.Engine, path string, body string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("PUT", path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	return recorder
}

// Manual discount saves no longer require an operator-typed reason; the audit
// records the server-side system operation description instead.
func TestPutAdminProviderBillingDiscountSavesWithoutReason(t *testing.T) {
	db := setupBillingURLNameControllerTest(t)
	require.NoError(t, db.Create(&model.Channel{Id: 31, Name: "channel"}).Error)
	period := time.Date(2026, time.August, 1, 0, 0, 0, 0, billingSettlementLocation).Unix()
	router := newBillingAdminTestRouter()

	recorder := putBillingAdminJSON(router, "/discounts",
		fmt.Sprintf(`{"period_start":%d,"channel_id":31,"discount":"0.8","expected_version":0}`, period))
	require.Equal(t, 200, recorder.Code, recorder.Body.String())

	var discount model.ProviderChannelBillingDiscount
	require.NoError(t, db.Where("period_start = ? AND channel_id = ?", period, 31).First(&discount).Error)
	require.True(t, discount.Discount.Equal(decimal.RequireFromString("0.8")))
	assert.Equal(t, int64(1), discount.Version)
	assert.Equal(t, model.ReasonChannelDiscountManualUpdate, discount.Reason)

	var audit model.ProviderBillingAudit
	require.NoError(t, db.Where("entity_type = ?", "channel_discount").First(&audit).Error)
	assert.Equal(t, model.ReasonChannelDiscountManualUpdate, audit.Reason)
	assert.Equal(t, 9, audit.OperatorId)

	// A still-sent legacy reason field is ignored, not stored.
	recorder = putBillingAdminJSON(router, "/discounts",
		fmt.Sprintf(`{"period_start":%d,"channel_id":31,"discount":"0.7","expected_version":1,"reason":"client typed reason"}`, period))
	require.Equal(t, 200, recorder.Code, recorder.Body.String())
	require.NoError(t, db.Where("period_start = ? AND channel_id = ?", period, 31).First(&discount).Error)
	require.True(t, discount.Discount.Equal(decimal.RequireFromString("0.7")))
	assert.Equal(t, model.ReasonChannelDiscountManualUpdate, discount.Reason)
}

func TestPutAdminUpstreamURLGroupNameValidatesAndPersists(t *testing.T) {
	db := setupBillingURLNameControllerTest(t)
	router := newBillingAdminTestRouter()

	recorder := putBillingAdminJSON(router, "/names", `{"url_key":"https://api.example.com","name":"Primary upstream"}`)
	require.Equal(t, 200, recorder.Code, recorder.Body.String())
	require.Equal(t, "Primary upstream", loadURLGroupName(t, db, "https://api.example.com"))

	// Invalid grouping keys are rejected before any write.
	recorder = putBillingAdminJSON(router, "/names", `{"url_key":"not a url","name":"x"}`)
	assert.Contains(t, recorder.Body.String(), `"success":false`)
	var rows int64
	require.NoError(t, db.Model(&model.ProviderURLGroupDisplayName{}).Count(&rows).Error)
	assert.EqualValues(t, 1, rows, "the rejected key must not persist anything")

	// An empty name clears the alias.
	recorder = putBillingAdminJSON(router, "/names", `{"url_key":"https://api.example.com","name":"  "}`)
	require.Equal(t, 200, recorder.Code, recorder.Body.String())
	assert.Empty(t, loadURLGroupName(t, db, "https://api.example.com"))

	var audits []model.ProviderBillingAudit
	require.NoError(t, db.Where("entity_type = ?", "url_group_name").Order("id").Find(&audits).Error)
	require.Len(t, audits, 2)
	assert.Equal(t, "create", audits[0].Action)
	assert.Equal(t, "delete", audits[1].Action)
}

func loadURLGroupName(t *testing.T, db *gorm.DB, urlKey string) string {
	t.Helper()
	var row model.ProviderURLGroupDisplayName
	err := db.Where("url_hash = ?", fmt.Sprintf("%x", common.Sha256Raw([]byte(urlKey)))).First(&row).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return ""
		}
		t.Fatal(err)
	}
	return row.Name
}

func TestPutAdminUpstreamURLGroupNameUnicodeLengthBoundary(t *testing.T) {
	for _, character := range []string{"A", "中", "😀"} {
		t.Run(character, func(t *testing.T) {
			db := setupBillingURLNameControllerTest(t)
			router := newBillingAdminTestRouter()
			name := strings.Repeat(character, 255)
			for _, size := range []int{255, 256} {
				body, err := common.Marshal(upstreamURLGroupNameRequest{URLKey: "channel:24", Name: "  " + strings.Repeat(character, size) + "  "})
				require.NoError(t, err)
				response := putBillingAdminJSON(router, "/names", string(body))
				var result struct {
					Success bool `json:"success"`
				}
				require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
				require.Equal(t, size == 255, result.Success, response.Body.String())
				assert.Equal(t, name, loadURLGroupName(t, db, "channel:24"))
			}
			var count int64
			require.NoError(t, db.Model(&model.ProviderBillingAudit{}).Count(&count).Error)
			assert.EqualValues(t, 1, count)
		})
	}
}
