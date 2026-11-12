package service

import (
	"context"
	"crypto/sha256"
	"fmt"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/model"
)

// The shared queue already matches the frozen report days and all filters.
// The identity proof covers the live names and search membership used by the CSV.
func adminUsageExportSourceVersion(ctx context.Context, targetID int, filters model.CustomerExportFilters) (string, error) {
	switch filters.UsageView {
	case "customers":
		customers, err := model.UsageOverviewExportIdentities(ctx, filters.UsageReportDays, filters.UsageSearch)
		if err != nil {
			return "", err
		}
		return usageExportIdentityVersion(customers), nil
	case usageAnalyticsExportTypeCustomer:
		name, err := usageExportCustomerName(targetID)
		if err != nil {
			return "", err
		}
		return usageExportIdentityVersion([]model.UsageCustomerOverviewRow{{UserId: targetID, Username: name}}), nil
	default:
		return "", nil
	}
}

func usageExportCustomerName(userID int) (string, error) {
	name, err := model.GetUsernameById(userID, true)
	if strings.TrimSpace(name) == "" {
		name = fmt.Sprintf("user-%d", userID)
	}
	return name, err
}

func usageExportIdentityVersion(customers []model.UsageCustomerOverviewRow) string {
	identities := make([]string, 0, len(customers))
	for _, customer := range customers {
		identities = append(identities, fmt.Sprintf("%d:%q", customer.UserId, customer.Username))
	}
	slices.Sort(identities)
	return fmt.Sprintf("usage-identities:%x", sha256.Sum256([]byte(strings.Join(identities, "\n"))))
}
