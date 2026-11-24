package service

import (
	"context"

	"github.com/QuantumNous/new-api/model"
)

func CustomerStatementDetailExportDays(ctx context.Context, actorID, userID int, start, end int64, reportVersion, statementVersion string, allowPending bool) ([]model.CustomerStatementDetailExportDay, error) {
	if len(reportVersion) > billingExportVersionMaxBytes || len(statementVersion) > 64 ||
		(reportVersion == "") == (statementVersion == "") {
		return nil, ErrCustomerExportInvalidRequest
	}
	if err := model.AuthorizeCustomerExport(ctx, actorID, userID); err != nil {
		return nil, err
	}
	if statementVersion != "" {
		filters := model.CustomerExportFilters{StatementDraftId: statementVersion, StartTimestamp: start, EndTimestamp: end}
		version, err := authorizedStatementVersionExport(ctx, actorID, userID, filters, allowPending)
		if err != nil {
			return nil, err
		}
		return model.CustomerStatementVersionDetailExportDays(ctx, version, start, end)
	}
	if !model.DailyReportSourceSupported() {
		return nil, model.ErrCustomerExportBackendUnsupported
	}
	ctx, report, err := model.ResolveBillingReport(ctx, start, end-1, reportVersion)
	if err != nil {
		return nil, err
	}
	return report.CustomerStatementDetailExportDays(ctx, userID)
}

func authorizedStatementVersionExport(ctx context.Context, actorID, userID int, filters model.CustomerExportFilters, allowPending bool) (*model.BillingStatementVersion, error) {
	version, err := model.CustomerStatementVersionForExport(ctx, model.DB, userID, filters)
	if err != nil {
		return nil, err
	}
	if version.Status == model.BillingStatementVersionPending {
		if !allowPending {
			return nil, model.ErrCustomerExportNotFound
		}
		if err := model.AuthorizeCustomerStatementsExport(ctx, actorID); err != nil {
			return nil, err
		}
	}
	return version, nil
}
