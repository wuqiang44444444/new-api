package controller

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupBillingVersionStatusTest(t *testing.T) (*gorm.DB, int64, string) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	raw, err := db.DB()
	require.NoError(t, err)
	previous := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = previous; require.NoError(t, raw.Close()) })
	require.NoError(t, db.AutoMigrate(&model.BillingStatementMonth{}, &model.BillingStatementVersion{}, &model.BillingStatementRetention{}))
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, billingSettlementLocation)
	query := fmt.Sprintf("/?user_id=11&start_timestamp=%d&end_timestamp=%d", start.Unix(), start.AddDate(0, 1, 0).Unix()-1)
	return db, start.Unix(), query
}

func TestBillingVersionHistoryDistinguishesMissingMonthFromReadFailure(t *testing.T) {
	for _, failRead := range []bool{false, true} {
		t.Run(fmt.Sprintf("read_failure=%t", failRead), func(t *testing.T) {
			db, _, query := setupBillingVersionStatusTest(t)
			if failRead {
				require.NoError(t, db.Callback().Query().Before("gorm:query").Register("test:month_read_failure", func(tx *gorm.DB) {
					tx.AddError(errors.New("month read unavailable"))
				}))
			}
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodGet, query, nil)
			GetAdminBillingStatementVersionHistory(c)
			var response struct {
				Success  bool  `json:"success"`
				Versions []any `json:"versions"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			assert.Equal(t, !failRead, response.Success)
			if !failRead {
				assert.NotNil(t, response.Versions)
				assert.Empty(t, response.Versions)
			}
		})
	}
}

func TestBillingVersionMonthStatusRejectsUnreadableReferences(t *testing.T) {
	for _, admin := range []bool{false, true} {
		for _, state := range []string{"empty", "current", "draft", "current read error", "draft read error", "missing current", "missing draft"} {
			t.Run(fmt.Sprintf("admin=%t/%s", admin, state), func(t *testing.T) {
				db, start, query := setupBillingVersionStatusTest(t)
				if state != "empty" {
					version := model.BillingStatementVersion{UserId: 11, PeriodStart: start, DraftPublicId: "status-test", Status: model.BillingStatementVersionConfirmed}
					if state == "draft" || state == "draft read error" || state == "missing draft" {
						version.Status = model.BillingStatementVersionPending
					}
					require.NoError(t, db.Create(&version).Error)
					month := model.BillingStatementMonth{UserId: 11, PeriodStart: start}
					if version.Status == model.BillingStatementVersionConfirmed {
						month.CurrentVersionId = &version.ID
					} else {
						month.ActiveDraftId = &version.ID
					}
					require.NoError(t, db.Create(&month).Error)
					if state == "missing current" || state == "missing draft" {
						require.NoError(t, db.Delete(&version).Error)
					}
				}
				if state == "current read error" || state == "draft read error" {
					require.NoError(t, db.Callback().Query().Before("gorm:query").Register("test:version_read_failure", func(tx *gorm.DB) {
						if _, singleVersion := tx.Statement.Dest.(*model.BillingStatementVersion); singleVersion {
							tx.AddError(errors.New("version read unavailable"))
						}
					}))
				}
				recorder := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(recorder)
				c.Request = httptest.NewRequest(http.MethodGet, query, nil)
				c.Set("id", 11)
				if admin {
					GetAdminBillingStatementVersionMonthStatus(c)
				} else {
					GetSelfBillingStatementVersionMonthStatus(c)
				}
				var response struct {
					Success bool `json:"success"`
					Data    *struct {
						CurrentVersion any  `json:"current_version"`
						ActiveDraft    any  `json:"active_draft"`
						HasActiveDraft bool `json:"has_active_draft"`
					} `json:"data"`
				}
				require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
				if state != "empty" && state != "current" && state != "draft" {
					assert.False(t, response.Success, recorder.Body.String())
					assert.Nil(t, response.Data, "failed reads must not publish an unconfirmed state")
					return
				}
				require.True(t, response.Success, recorder.Body.String())
				require.NotNil(t, response.Data)
				assert.Equal(t, state == "current", response.Data.CurrentVersion != nil)
				assert.Equal(t, state == "draft", response.Data.HasActiveDraft)
				assert.Equal(t, admin && state == "draft", response.Data.ActiveDraft != nil)
			})
		}
	}
}
