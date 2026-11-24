//go:build !yuan_gateway_local_dev

package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

func TestDailyReportDefaultBuildUsesResourceBudgetEvenWithDevelopmentDatabase(t *testing.T) {
	t.Setenv("SQL_DSN", "postgresql://dev:fixture@127.0.0.1:54330/yuan_gateway_dev?sslmode=disable&application_name=yuan-gateway-dev")
	mainDB, logDB := model.DB, model.LOG_DB
	t.Cleanup(func() { model.DB, model.LOG_DB = mainDB, logDB })
	model.DB = &gorm.DB{}
	model.LOG_DB = model.DB

	assert.False(t, skipDailyReportResourceBudget())
}
