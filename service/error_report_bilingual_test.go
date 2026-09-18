package service

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestErrorReportBilingualLongDetailPreservesContentAndPartBudget(t *testing.T) {
	withErrorReportTestDB(t)
	h := newErrorReportHandler(nil)
	start := beijingTime(10, 0).Unix()
	view, err := h.renderErrorReportWindow(start, start+3600, []*model.ErrorEvent{{CreatedAt: start, EventType: "api_error", Status: 503, Detail: strings.Repeat("雪", 40000)}})
	require.NoError(t, err)
	require.Greater(t, len(view.Drafts), 1)
	var body strings.Builder
	for _, part := range view.Drafts {
		assert.LessOrEqual(t, len(part.BodyHTML), errorReportPartBudgetBytes)
		assert.Contains(t, part.Subject, "Operations and errors")
		body.WriteString(part.BodyHTML)
	}
	assert.Contains(t, body.String(), "错误明细 / Error details")
	assert.Equal(t, 40000, strings.Count(body.String(), "雪"))
}
