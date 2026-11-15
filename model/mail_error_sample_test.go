package model

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/QuantumNous/new-api/clienterrlog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestMailErrorSamplesAcrossDatabases(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			var logs *gorm.DB
			if dialect == "sqlite" {
				logs = setupErrorEventTestDB(t)
			} else {
				if os.Getenv("TEST_MAIL_PREFERENCES_DIALECT") != dialect {
					t.Skip("isolated server not configured")
				}
				logs = setupBillingStatementServerDB(t, dialect)
			}
			old := LOG_DB
			LOG_DB = logs
			t.Cleanup(func() { LOG_DB = old })
			require.NoError(t, logs.AutoMigrate(&ErrorEvent{}))
			var events []ErrorEvent
			for i, id := range []int{1, 201, 2, 202, 3, 203, 4} {
				events = append(events, ErrorEvent{CreatedAt: 155, UserId: id, EventType: clienterrlog.EventAPIError, Reason: clienterrlog.WalletQuotaInsufficient, RequestId: fmt.Sprint("sample-", i)})
			}
			events = append(events, ErrorEvent{CreatedAt: 157, UserId: 999, EventType: clienterrlog.EventAPIError, Reason: clienterrlog.WalletQuotaInsufficient, RequestId: "other-account"})
			require.NoError(t, logs.Create(&events).Error)
			ids := make([]int, 203)
			for i := range ids {
				ids[i] = i + 1
			}
			rows, err := ReadMailErrorSamples(context.Background(), MailErrorScope{Start: 150, End: 160, UserIDs: ids, Reason: clienterrlog.WalletQuotaInsufficient})
			require.NoError(t, err)
			require.Len(t, rows, 5)
			for i, row := range rows {
				assert.Equal(t, fmt.Sprint("sample-", 6-i), row.RequestID)
			}
		})
	}
}
