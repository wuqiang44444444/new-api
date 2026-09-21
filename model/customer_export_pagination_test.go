package model

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestExportJobPagesCountOnlyVisibleOwnedJobs(t *testing.T) {
	setupCustomerExportTestDB(t)
	for _, item := range []struct {
		owner, target int
		kind          string
	}{
		{1, 1, CustomerExportJobTypeUsageLogs}, {1, 2, CustomerExportJobTypeUsageLogs},
		{2, 2, CustomerExportJobTypeUsageLogs}, {1, 1, CustomerExportJobTypeStatementVersion},
		{1, 1, CustomerExportJobTypeUsageLogs}, {1, 1, CustomerExportJobTypeUsageLogs},
	} {
		job := CustomerExportJob{JobID: fmt.Sprintf("page-%d-%d-%s", item.owner, item.target, item.kind), UserId: item.owner, TargetUserId: item.target, JobType: item.kind, Status: CustomerExportJobStatusSucceeded}
		// A new unique public ID without relying on the clock.
		var count int64
		require.NoError(t, DB.Model(&CustomerExportJob{}).Count(&count).Error)
		job.JobID = fmt.Sprintf("page-%d", count)
		require.NoError(t, DB.Create(&job).Error)
	}
	first, total, err := ListCustomerExportJobs(context.Background(), 1, 1, 2)
	require.NoError(t, err)
	require.Len(t, first, 2)
	assert.EqualValues(t, 3, total)
	second, total, err := ListCustomerExportJobs(context.Background(), 1, 2, 2)
	require.NoError(t, err)
	require.Len(t, second, 1)
	assert.EqualValues(t, 3, total)
	assert.Greater(t, first[1].ID, second[0].ID)
	empty, total, err := ListCustomerExportJobs(context.Background(), 1, 3, 2)
	require.NoError(t, err)
	assert.Empty(t, empty)
	assert.EqualValues(t, 3, total)
}
