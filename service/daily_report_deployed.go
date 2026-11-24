//go:build !yuan_gateway_local_dev

package service

func skipDailyReportResourceBudget() bool { return false }
