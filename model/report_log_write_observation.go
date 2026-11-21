package model

import "github.com/QuantumNous/new-api/common"

func writeObservedLog(log *Log) error {
	sample := common.BeginReportForeground(common.ReportForegroundLog)
	err := LOG_DB.Create(log).Error
	sample.Finish(err != nil)
	return err
}
