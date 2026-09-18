package service

import (
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestErrorReportFairClaimsPreserveRecipientPartOrder(t *testing.T) {
	db := withErrorReportTestDB(t)
	require.NoError(t, db.Create(&model.ErrorReportSchedule{Scope: model.ErrorReportScheduleScopeGlobal, Enabled: true, Periods: `[{"start":3600,"end":0}]`}).Error)
	for _, window := range []int64{3600, 7200} {
		report := model.ErrorReport{ReportID: strings.Repeat("r", int(window/3600)), WindowStart: window, WindowEnd: window + 3600, Status: model.ErrorReportStatusBuilding}
		inserted, err := model.InsertErrorReportBuilding(&report)
		require.NoError(t, err)
		require.True(t, inserted)
		require.NoError(t, publishErrorReportTestParts(report.ReportID, []*model.ErrorReportPart{{ReportID: report.ReportID, PartNo: 1, Subject: "one", BodyHTML: "one"}, {ReportID: report.ReportID, PartNo: 2, Subject: "two", BodyHTML: "two"}}, []string{"a@example.com", "b@example.com"}, 0, "{}", "snapshot"))
	}
	claims, err := model.ClaimErrorReportDeliveries(10800, "first", 10900, 1)
	require.NoError(t, err)
	require.Len(t, claims, 1)
	a := claims[0]
	assert.Equal(t, "a@example.com", a.Recipient)
	assert.Equal(t, 1, a.PartNo)
	_, err = model.FinishErrorReportDelivery(a.ID, "first", model.ErrorReportDeliveryRetryWait, "unavailable", "", 12000, 0)
	require.NoError(t, err)
	for _, expected := range []struct {
		report string
		part   int
	}{{"r", 1}, {"r", 2}, {"rr", 1}, {"rr", 2}} {
		claims, err = model.ClaimErrorReportDeliveries(10800, "next", 10900, 1)
		require.NoError(t, err)
		require.Len(t, claims, 1)
		b := claims[0]
		assert.Equal(t, "b@example.com", b.Recipient)
		assert.Equal(t, expected.report, b.ReportID)
		assert.Equal(t, expected.part, b.PartNo)
		_, err = model.FinishErrorReportDelivery(b.ID, "next", model.ErrorReportDeliveryAccepted, "", "", 0, 10800)
		require.NoError(t, err)
	}
	claims, err = model.ClaimErrorReportDeliveries(10800, "waiting", 10900, 1)
	require.NoError(t, err)
	assert.Empty(t, claims, "later parts must wait for the first retry")
	claims, err = model.ClaimErrorReportDeliveries(12000, "retry", 12100, 1)
	require.NoError(t, err)
	require.Len(t, claims, 1)
	assert.Equal(t, a.ID, claims[0].ID)
}

func TestErrorReportRedactsLegacyTaskFailureDetails(t *testing.T) {
	rows := (&errorReportHandler{}).renderDetailRow(1, &model.ErrorEvent{
		EventType: "task_failure", Detail: `{"platform":"suno","fail_reason":"{\"message\":\"fixture-private-response\",\"api_key\":\"fixture-key\",\"url\":\"https://upstream.example/file?signature=fixture-secret\"}"}`,
	}, errorReportLocation())
	html := strings.Join(rows, "")
	assert.Contains(t, html, "suno")
	for _, forbidden := range []string{"fixture-key", "fixture-secret", "upstream.example", "fixture-private-response"} {
		assert.NotContains(t, html, forbidden)
	}
}
