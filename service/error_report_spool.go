package service

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

const (
	errorReportSpoolBudget  = 256 * 1024 * 1024
	errorReportRecordBudget = 8 * 1024 * 1024
	errorReportBatchBytes   = 1024 * 1024
)

// Temporary, private, unlinked files are rendering scratch space, never report
// facts. The OS releases them on close or process exit. Only publication makes
// a report durable; retries rebuild an unpublished snapshot from the source.
type errorReportSpool struct {
	file  *os.File
	bytes int64
}

func newErrorReportSpool() (*errorReportSpool, error) {
	file, err := os.CreateTemp("", "yuan-error-report-*")
	if err != nil {
		return nil, errors.New("error report temporary file unavailable")
	}
	if err := os.Remove(file.Name()); err != nil {
		file.Close()
		return nil, errors.New("error report temporary file cannot be unlinked")
	}
	return &errorReportSpool{file: file}, nil
}

func (s *errorReportSpool) write(value any) error {
	data, err := common.Marshal(value)
	if err != nil {
		return err
	}
	if len(data) > errorReportRecordBudget || s.bytes+int64(len(data))+8 > errorReportSpoolBudget {
		return errors.New("error report snapshot exceeds resource budget")
	}
	if err := binary.Write(s.file, binary.LittleEndian, uint64(len(data))); err != nil {
		return errors.New("error report temporary file write failed")
	}
	if _, err := s.file.Write(data); err != nil {
		return errors.New("error report temporary file write failed")
	}
	s.bytes += int64(len(data)) + 8
	return nil
}

func (s *errorReportSpool) rewind() error {
	_, err := s.file.Seek(0, io.SeekStart)
	return err
}

func (s *errorReportSpool) read(value any) error {
	var length uint64
	if err := binary.Read(s.file, binary.LittleEndian, &length); err != nil {
		return err
	}
	if length > errorReportRecordBudget {
		return errors.New("error report snapshot record exceeds resource budget")
	}
	data := make([]byte, int(length))
	if _, err := io.ReadFull(s.file, data); err != nil {
		return err
	}
	return common.Unmarshal(data, value)
}

type errorReportSnapshotReader func(func(*model.ErrorEvent) error) (map[int]string, error)

// buildErrorReport freezes one source cursor on disk, summarizes bounded maps,
// then renders bounded batches and stores complete part content on disk. The
// final count is known before any subject/body is published.
func (h *errorReportHandler) buildErrorReport(ctx context.Context, start, end int64, read errorReportSnapshotReader) (*errorReportWindowView, *errorReportSpool, error) {
	snapshot, err := newErrorReportSpool()
	if err != nil {
		return nil, nil, err
	}
	defer snapshot.file.Close()
	counts, byType, byReason := map[string]int{}, map[string]int{}, map[string]int{}
	byChannelID := map[int]int{}
	total := 0
	summaryBytes := 0
	names, err := read(func(event *model.ErrorEvent) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := snapshot.write(event); err != nil {
			return err
		}
		total++
		counts[classifyErrorEvent(event)]++
		eventType := event.EventType
		if eventType == "" {
			eventType = "api_error"
		}
		if byType[eventType] == 0 {
			summaryBytes += len(eventType)
		}
		reason := emptyDash(event.Reason)
		if byReason[reason] == 0 {
			summaryBytes += len(reason)
		}
		if summaryBytes > 4*1024*1024 {
			return errors.New("error report summary exceeds resource budget")
		}
		byType[eventType]++
		byReason[reason]++
		byChannelID[event.ChannelId]++
		if len(byType) > model.ErrorReportMaxSummaryGroups || len(byReason) > model.ErrorReportMaxSummaryGroups || len(byChannelID) > model.ErrorReportMaxSummaryGroups {
			return errors.New("error report summary exceeds resource budget")
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	byChannel := map[string]int{}
	for id, count := range byChannelID {
		name := names[id]
		if name == "" && id > 0 {
			name = fmt.Sprintf("渠道 / Channel #%d", id)
		}
		name = emptyDash(name)
		if byChannel[name] == 0 {
			summaryBytes += len(name)
		}
		if summaryBytes > 4*1024*1024 {
			return nil, nil, errors.New("error report summary exceeds resource budget")
		}
		byChannel[name] += count
	}
	view, units, err := h.renderErrorReportSummary(ctx, start, end, total, counts, byType, byReason, byChannel)
	if err != nil {
		return nil, nil, err
	}
	parts, err := newErrorReportSpool()
	if err != nil {
		return nil, nil, err
	}
	success := false
	defer func() {
		if !success {
			parts.file.Close()
		}
	}()
	var content strings.Builder
	detailRows := 0
	flush := func() error {
		if content.Len() == 0 {
			return nil
		}
		if err := parts.write(errorReportPartDraft{BodyHTML: content.String(), DetailRows: detailRows}); err != nil {
			return err
		}
		if parts.bytes+snapshot.bytes > errorReportSpoolBudget {
			return errors.New("error report snapshot exceeds resource budget")
		}
		view.TotalParts++
		content.Reset()
		detailRows = 0
		return nil
	}
	appendUnits := func(units []errorReportUnit) error {
		for _, fragment := range splitErrorReportUnits(units) {
			if err := ctx.Err(); err != nil {
				return err
			}
			if len(fragment.html) > errorReportPartBudgetBytes-errorReportPartSkeletonReserve {
				return errors.New("error report display row exceeds part budget")
			}
			if content.Len()+len(fragment.html) > errorReportPartBudgetBytes-errorReportPartSkeletonReserve {
				if err := flush(); err != nil {
					return err
				}
			}
			content.WriteString(fragment.html)
			detailRows += fragment.detailRows
		}
		return nil
	}
	if err := appendUnits(units); err != nil {
		return nil, nil, err
	}
	if err := snapshot.rewind(); err != nil {
		return nil, nil, err
	}
	offset := 0
	for offset < total {
		batch := make([]*model.ErrorEvent, 0, 200)
		size := int64(0)
		for len(batch) < 200 && size < errorReportBatchBytes && offset+len(batch) < total {
			var event model.ErrorEvent
			before, _ := snapshot.file.Seek(0, io.SeekCurrent)
			if err := snapshot.read(&event); err != nil {
				return nil, nil, err
			}
			after, _ := snapshot.file.Seek(0, io.SeekCurrent)
			size += after - before
			event.ChannelName = names[event.ChannelId]
			batch = append(batch, &event)
		}
		if err := appendUnits(h.renderDetailUnits(batch, errorReportLocation(), offset)); err != nil {
			return nil, nil, err
		}
		offset += len(batch)
	}
	if total == 0 {
		if err := appendUnits(h.renderDetailUnits(nil, errorReportLocation(), 0)); err != nil {
			return nil, nil, err
		}
	}
	if err := flush(); err != nil {
		return nil, nil, err
	}
	if err := parts.rewind(); err != nil {
		return nil, nil, err
	}
	success = true
	return view, parts, nil
}

func (s *errorReportSpool) nextPart(view *errorReportWindowView, partNo int) (*model.ErrorReportPart, error) {
	var draft errorReportPartDraft
	err := s.read(&draft)
	if partNo > view.TotalParts {
		if errors.Is(err, io.EOF) {
			return nil, io.EOF
		}
		return nil, errors.New("error report part count mismatch")
	}
	if errors.Is(err, io.EOF) {
		return nil, io.ErrUnexpectedEOF
	}
	if err != nil {
		return nil, err
	}
	body := wrapErrorReportPart(view, partNo, view.TotalParts, draft.BodyHTML)
	if len(body) > errorReportPartBudgetBytes {
		return nil, errors.New("error report body exceeds part budget")
	}
	return &model.ErrorReportPart{ReportID: view.ReportID, PartNo: partNo, Subject: errorReportSubject(view, partNo, view.TotalParts), BodyHTML: body, DetailRows: draft.DetailRows}, nil
}
