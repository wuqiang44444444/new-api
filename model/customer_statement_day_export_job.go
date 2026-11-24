package model

import "github.com/QuantumNous/new-api/common"

var ErrCustomerStatementExportCoverageChanged = common.NewMessage("Billing coverage changed. Review the selected date and try again.")

// CreateCustomerStatementDayExportJob checks observed coverage for new jobs
// after matching an already accepted submission in the same transaction.
// Regeneration supplies the original frozen filters and no expected coverage.
func CreateCustomerStatementDayExportJob(actorID int, filters CustomerExportFilters, expectedIncomplete *bool) (*CustomerExportJob, bool, error) {
	if filters.BillingDate == "" || filters.BillingReportCoverage == nil {
		return nil, false, ErrCustomerExportStateConflict
	}
	var newJobError error
	if expectedIncomplete != nil && *expectedIncomplete != (filters.BillingReportCoverage.IncompleteDays > 0) {
		newJobError = ErrCustomerStatementExportCoverageChanged
	}
	return createCustomerExportJob(actorID, 0, CustomerExportJobTypeCustomerStatements, filters,
		CustomerExportReuse{RequireExactActiveFilters: expectedIncomplete == nil}, newJobError)
}

func sameActiveCustomerStatementDayFilters(existing, submitted string) bool {
	var left, right CustomerExportFilters
	if common.UnmarshalJsonStr(existing, &left) != nil || common.UnmarshalJsonStr(submitted, &right) != nil ||
		left.BillingDate == "" || right.BillingDate == "" {
		return false
	}
	left.BillingReportCoverage, right.BillingReportCoverage = nil, nil
	a, err := common.Marshal(left)
	if err != nil {
		return false
	}
	b, err := common.Marshal(right)
	return err == nil && string(a) == string(b)
}
