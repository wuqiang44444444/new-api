package service

import (
	"context"
	"errors"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
)

// Slice adapters exist only in tests. Both fixtures and production exercise the
// same bounded rendering/publication implementation.
type errorReportTestView struct {
	*errorReportWindowView
	Drafts []errorReportPartDraft
}

func (h *errorReportHandler) renderErrorReportWindow(start, end int64, events []*model.ErrorEvent) (*errorReportTestView, error) {
	view, spool, err := h.buildErrorReport(context.Background(), start, end, func(visit func(*model.ErrorEvent) error) (map[int]string, error) {
		names := map[int]string{}
		for _, event := range events {
			names[event.ChannelId] = event.ChannelName
			if err := visit(event); err != nil {
				return nil, err
			}
		}
		return names, nil
	})
	if err != nil {
		return nil, err
	}
	defer spool.file.Close()
	result := &errorReportTestView{errorReportWindowView: view}
	for i := 1; i <= view.TotalParts; i++ {
		part, err := spool.nextPart(view, i)
		if err != nil {
			return nil, err
		}
		result.Drafts = append(result.Drafts, errorReportPartDraft{Subject: part.Subject, BodyHTML: part.BodyHTML, DetailRows: part.DetailRows})
	}
	return result, nil
}

func publishErrorReportTestParts(reportID string, parts []*model.ErrorReportPart, recipients []string, totalEvents int, summary, dataNote string) error {
	index := 0
	return model.PublishErrorReportParts(context.Background(), reportID, func() (*model.ErrorReportPart, error) {
		if index == len(parts) {
			return nil, io.EOF
		}
		part := parts[index]
		index++
		return part, nil
	}, recipients, totalEvents, summary, dataNote)
}

func TestErrorReportCursorAndSpoolPreserveAllRows(t *testing.T) {
	db := withErrorReportTestDB(t)
	conn, err := db.DB()
	require.NoError(t, err)
	conn.SetMaxOpenConns(1)
	require.NoError(t, db.Create(&model.Channel{Id: 9, Name: "fixture-channel"}).Error)
	start := beijingTime(10, 0).Unix()
	rows := make([]model.ErrorEvent, 601)
	for i := range rows {
		rows[i] = model.ErrorEvent{CreatedAt: start, ChannelId: 9, RequestId: "same-key", TaskId: fmt.Sprintf("row-%04d", i), Detail: `{"safe":"<value>"}`}
	}
	require.NoError(t, db.CreateInBatches(&rows, 50).Error)
	require.NoError(t, db.Create(&model.ErrorEvent{CreatedAt: start + 3600, TaskId: "outside-window"}).Error)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	h := newErrorReportHandler(nil)
	view, spool, err := h.buildErrorReport(ctx, start, start+3600, func(visit func(*model.ErrorEvent) error) (map[int]string, error) {
		return model.WalkErrorEventsForReport(ctx, start, start+3600, visit)
	})
	require.NoError(t, err)
	defer spool.file.Close()
	assert.Equal(t, len(rows), view.TotalEvents)
	var combined strings.Builder
	sum := 0
	for i := 1; i <= view.TotalParts; i++ {
		part, err := spool.nextPart(view, i)
		require.NoError(t, err)
		assert.LessOrEqual(t, len(part.BodyHTML), errorReportPartBudgetBytes)
		assert.Contains(t, part.Subject, fmt.Sprintf("Part %d/%d", i, view.TotalParts))
		combined.WriteString(part.BodyHTML)
		sum += part.DetailRows
	}
	html := combined.String()
	assert.Equal(t, 601, sum)
	assert.Equal(t, 601, strings.Count(html, "same-key"))
	for i := range rows {
		assert.Equal(t, 1, strings.Count(html, fmt.Sprintf("row-%04d", i)))
	}
	assert.Contains(t, html, "<td>601</td>")
	assert.Contains(t, html, "fixture-channel")
	assert.NotContains(t, html, "outside-window")
	assert.Contains(t, html, "&lt;value&gt;")
}

func TestErrorReportPublicationReadFailureRollsBack(t *testing.T) {
	db := withErrorReportTestDB(t)
	withErrorReportSetting(t, true, "ops@example.com")
	start := beijingTime(10, 0).Unix()
	report := model.ErrorReport{ReportID: errorReportID(start), WindowStart: start, WindowEnd: start + 3600, Status: model.ErrorReportStatusBuilding}
	_, err := model.InsertErrorReportBuilding(&report)
	require.NoError(t, err)
	calls := 0
	failure := errors.New("fixture spool read failed")
	err = model.PublishErrorReportParts(context.Background(), report.ReportID, func() (*model.ErrorReportPart, error) {
		calls++
		if calls == 2 {
			return nil, failure
		}
		return &model.ErrorReportPart{ReportID: report.ReportID, PartNo: 1, BodyHTML: "first"}, nil
	}, []string{"ops@example.com"}, 1, "{}", "")
	require.ErrorIs(t, err, failure)
	for _, table := range []any{&model.ErrorReportPart{}, &model.ErrorReportDelivery{}} {
		var count int64
		require.NoError(t, db.Model(table).Count(&count).Error)
		assert.Zero(t, count)
	}
	state, err := model.GetErrorReportByWindowStart(start)
	require.NoError(t, err)
	assert.Equal(t, model.ErrorReportStatusBuilding, state.Status)
	h := newErrorReportHandler(nil)
	h.markWindowFailed(start, start+3600, failure)
	state, err = model.GetErrorReportByWindowStart(start)
	require.NoError(t, err)
	assert.Equal(t, model.ErrorReportStatusFailed, state.Status, "existing building rows must record the failure")
	require.NoError(t, h.processWindow(context.Background(), start, nil))
	state, err = model.GetErrorReportByWindowStart(start)
	require.NoError(t, err)
	assert.Equal(t, model.ErrorReportStatusReady, state.Status)
}

func TestErrorReportResourceFailureDoesNotPublishOrAdvance(t *testing.T) {
	db := withErrorReportTestDB(t)
	withErrorReportSetting(t, true, "ops@example.com")
	start := beijingTime(10, 0).Unix()
	row := &model.ErrorEvent{CreatedAt: start, Detail: strings.Repeat("x", errorReportRecordBudget)}
	require.NoError(t, db.Create(row).Error)
	h := newErrorReportHandler(nil)
	require.ErrorContains(t, h.processWindow(context.Background(), start, nil), "resource budget")
	report, err := model.GetErrorReportByWindowStart(start)
	require.NoError(t, err)
	assert.Equal(t, model.ErrorReportStatusFailed, report.Status)
	var count int64
	require.NoError(t, db.Model(&model.ErrorReportDelivery{}).Count(&count).Error)
	assert.Zero(t, count)
	next, err := model.NextErrorReportWindow(context.Background())
	require.NoError(t, err)
	assert.Equal(t, start, next)
	require.NoError(t, db.Delete(row).Error)
	require.NoError(t, h.processWindow(context.Background(), start, nil))
}

func TestErrorReportMissingWholePartCannotEndPublication(t *testing.T) {
	spool, err := newErrorReportSpool()
	require.NoError(t, err)
	defer spool.file.Close()
	require.NoError(t, spool.write(errorReportPartDraft{BodyHTML: "only first part"}))
	require.NoError(t, spool.rewind())
	view := &errorReportWindowView{ReportID: "fixture", TotalParts: 2}
	_, err = spool.nextPart(view, 1)
	require.NoError(t, err)
	_, err = spool.nextPart(view, 2)
	assert.ErrorIs(t, err, io.ErrUnexpectedEOF, "EOF before frozen total must roll back publication")
}

func TestErrorReportResumesBuildingReportAfterCrash(t *testing.T) {
	db := withErrorReportTestDB(t)
	withErrorReportSetting(t, true, "ops@example.com")
	start := beijingTime(10, 0).Unix()
	report := model.ErrorReport{ReportID: errorReportID(start), WindowStart: start, WindowEnd: start + 3600, Status: model.ErrorReportStatusBuilding}
	_, err := model.InsertErrorReportBuilding(&report)
	require.NoError(t, err)
	// Simulate a process exiting after inserting building, before publication.
	h := newErrorReportHandler(nil)
	require.NoError(t, h.processWindow(context.Background(), start, nil))
	resumed, err := model.GetErrorReportByWindowStart(start)
	require.NoError(t, err)
	assert.Equal(t, model.ErrorReportStatusReady, resumed.Status)
	var reports int64
	require.NoError(t, db.Model(&model.ErrorReport{}).Count(&reports).Error)
	assert.EqualValues(t, 1, reports)
	next, err := model.NextErrorReportWindow(context.Background())
	require.NoError(t, err)
	assert.Equal(t, start+3600, next)
}
