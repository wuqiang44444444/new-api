//go:build yuan_gateway_local_dev

package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

func TestDailyReportResourceBudgetRequiresLocalDevelopmentDatabase(t *testing.T) {
	mainDB, logDB := model.DB, model.LOG_DB
	t.Cleanup(func() { model.DB, model.LOG_DB = mainDB, logDB })
	t.Setenv("PGSERVICE", "")

	const defaultDSN = "postgresql://dev:fixture@127.0.0.1:54330/yuan_gateway_dev?sslmode=disable&application_name=yuan-gateway-dev"
	for _, tc := range []struct {
		name        string
		dsn         string
		separateLog bool
		skip        bool
	}{
		{name: "default script", dsn: defaultDSN, skip: true},
		{name: "cn script", dsn: "postgresql://dev:fixture@127.0.0.1:54331/yuan_gateway_dev_cn?sslmode=disable&application_name=yuan-gateway-dev-cn", skip: true},
		{name: "development compose", dsn: "postgresql://dev:fixture@postgres:5432/yuan_gateway_dev?sslmode=disable&application_name=yuan-gateway-dev-compose", skip: true},
		{name: "remote database with local identity", dsn: defaultDSN + "&host=staging.example"},
		{name: "remote database fallback", dsn: defaultDSN + "&host=127.0.0.1,staging.example"},
		{name: "unrecognized database", dsn: "postgresql://dev:fixture@127.0.0.1:54330/staging?sslmode=disable&application_name=yuan-gateway-dev"},
		{name: "unrecognized application name", dsn: "postgresql://dev:fixture@127.0.0.1:54330/yuan_gateway_dev?sslmode=disable&application_name=yuan-gateway"},
		{name: "compose name with external host", dsn: "postgresql://dev:fixture@staging.example:5432/yuan_gateway_dev?sslmode=disable&application_name=yuan-gateway-dev-compose"},
		{name: "separate logs", dsn: defaultDSN, separateLog: true},
		{name: "unspecified database"},
		{name: "malformed database", dsn: "postgresql://%"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("SQL_DSN", tc.dsn)
			model.DB = &gorm.DB{}
			model.LOG_DB = model.DB
			if tc.separateLog {
				model.LOG_DB = &gorm.DB{}
			}
			assert.Equal(t, tc.skip, skipDailyReportResourceBudget())
		})
	}
}
