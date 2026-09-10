package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 证据拒绝边界错误分类回归：唯一映射输出稳定 code/状态/文案，双重故障保持
// 主因优先，既有错误身份（ErrTaskRequestEvidenceUnavailable /
// ErrTaskRequestEvidenceBodyTooLarge）与两条 cause 链不因包装而丢失。

// realJSONSyntaxError 产生真实的 *json.SyntaxError（字段未导出，无法字面构造）。
func realJSONSyntaxError(t *testing.T) *json.SyntaxError {
	t.Helper()
	var decoded any
	err := json.Unmarshal([]byte("{bad"), &decoded)
	require.Error(t, err)
	var syntaxErr *json.SyntaxError
	require.True(t, errors.As(err, &syntaxErr))
	return syntaxErr
}

func TestEvidenceRejectionClassificationTable(t *testing.T) {
	syntax := realJSONSyntaxError(t)
	dbErr := errors.New("db down: connection refused")
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{
			name:       "configuration",
			err:        evidenceUnavailableFailure(TaskRequestEvidenceCategoryConfiguration, TaskRequestEvidenceStageConfiguration, TaskRequestEvidenceSourceNone).WithCause(errors.New("storage dir is required")),
			wantStatus: 503,
			wantCode:   "evidence_configuration_error",
		},
		{
			name:       "north capacity",
			err:        evidenceTooLargeFailure(TaskRequestEvidenceStageNorthCapacity, TaskRequestEvidenceSourceNorth, 900, 100),
			wantStatus: 413,
			wantCode:   "request_body_too_large",
		},
		{
			name:       "south capacity is a processing failure, not client blame",
			err:        evidenceTooLargeFailure(TaskRequestEvidenceStageBodySnapshot, TaskRequestEvidenceSourceSouth, 900, 100),
			wantStatus: 503,
			wantCode:   "evidence_processing_failed",
		},
		{
			name:       "proven north json syntax error under json contract",
			err:        evidenceRedactionFailure(&evidenceJSONParseError{cause: syntax}, TaskRequestEvidenceSourceNorth, true),
			wantStatus: 400,
			wantCode:   "invalid_request",
		},
		{
			name:       "same syntax cause on south body stays processing",
			err:        evidenceRedactionFailure(&evidenceJSONParseError{cause: syntax}, TaskRequestEvidenceSourceSouth, true),
			wantStatus: 503,
			wantCode:   "evidence_processing_failed",
		},
		{
			name:       "sniffed json without declared json contract stays processing",
			err:        evidenceRedactionFailure(&evidenceJSONParseError{cause: syntax}, TaskRequestEvidenceSourceNorth, false),
			wantStatus: 503,
			wantCode:   "evidence_processing_failed",
		},
		{
			name:       "valid json platform remarshal failure stays processing",
			err:        evidenceRedactionFailure(errors.New("json encode failed"), TaskRequestEvidenceSourceNorth, true),
			wantStatus: 503,
			wantCode:   "evidence_processing_failed",
		},
		{
			name:       "event reservation failure",
			err:        evidenceUnavailableFailure(TaskRequestEvidenceCategoryEventDatabase, TaskRequestEvidenceStageEventReserve, TaskRequestEvidenceSourceNorth).WithCause(dbErr),
			wantStatus: 503,
			wantCode:   "evidence_database_unavailable",
		},
		{
			name:       "object storage failure",
			err:        evidenceUnavailableFailure(TaskRequestEvidenceCategoryObjectStorage, TaskRequestEvidenceStageObjectWrite, TaskRequestEvidenceSourceNorth).WithCause(errors.New("mkdir failed")),
			wantStatus: 503,
			wantCode:   "evidence_storage_unavailable",
		},
		{
			name:       "store busy",
			err:        evidenceStoreWriteFailure(ErrTaskRequestEvidenceStoreBusy, TaskRequestEvidenceSourceNorth),
			wantStatus: 503,
			wantCode:   "evidence_store_busy",
		},
		{
			name:       "write timeout",
			err:        evidenceStoreWriteFailure(ErrTaskRequestEvidenceWriteTimeout, TaskRequestEvidenceSourceNorth),
			wantStatus: 503,
			wantCode:   "evidence_write_timeout",
		},
		{
			name:       "client body stream truncated",
			err:        evidenceUnavailableFailure(TaskRequestEvidenceCategoryBodyRead, TaskRequestEvidenceStageBodyCache, TaskRequestEvidenceSourceNorth).WithCause(fmt.Errorf("read body failed: %w", io.ErrUnexpectedEOF)),
			wantStatus: 400,
			wantCode:   "invalid_request",
		},
		{
			name:       "local body cache failure is not client blame",
			err:        evidenceUnavailableFailure(TaskRequestEvidenceCategoryBodyRead, TaskRequestEvidenceStageBodyCache, TaskRequestEvidenceSourceNorth).WithCause(errors.New("body storage is closed")),
			wantStatus: 503,
			wantCode:   "evidence_storage_unavailable",
		},
		{
			name:       "send unverifiable never suggests a safe retry",
			err:        evidenceUnavailableFailure(TaskRequestEvidenceCategorySendUnverifiable, TaskRequestEvidenceStageSendVerify, TaskRequestEvidenceSourceNone),
			wantStatus: 503,
			wantCode:   "evidence_unavailable",
		},
		{
			name:       "unclassified fallback",
			err:        evidenceUnavailableFailure(TaskRequestEvidenceCategoryUnavailable, TaskRequestEvidenceStageIndexCreate, TaskRequestEvidenceSourceSouth),
			wantStatus: 503,
			wantCode:   "evidence_unavailable",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rejection, ok := ClassifyTaskRequestEvidenceRejection(tt.err)
			require.True(t, ok)
			assert.Equal(t, tt.wantStatus, rejection.Status)
			assert.Equal(t, tt.wantCode, rejection.Code)
			assert.NotEmpty(t, rejection.Message)
			assert.NotEmpty(t, rejection.Diagnostic)
		})
	}

	_, ok := ClassifyTaskRequestEvidenceRejection(errors.New("not an evidence error"))
	assert.False(t, ok)
	_, ok = ClassifyTaskRequestEvidenceRejection(nil)
	assert.False(t, ok)
}

func TestEvidenceErrorIdentityAndCauseChains(t *testing.T) {
	syntax := realJSONSyntaxError(t)
	dbErr := errors.New("disk I/O error")

	// 单故障：北向正文超限保持超限错误身份。
	tooLarge := evidenceTooLargeFailure(TaskRequestEvidenceStageNorthCapacity, TaskRequestEvidenceSourceNorth, 900, 100)
	require.True(t, errors.Is(tooLarge, ErrTaskRequestEvidenceBodyTooLarge))
	require.False(t, errors.Is(tooLarge, ErrTaskRequestEvidenceUnavailable))
	assert.Equal(t, int64(900), tooLarge.Observed)
	assert.Equal(t, int64(100), tooLarge.Limit)

	// 双重故障：主因为已证明的客户 JSON 语法错误，次因为事件完成更新失败。
	// 公开类别由主因决定；两条 cause 链与既有身份同时保留。
	double := evidenceRedactionFailure(&evidenceJSONParseError{cause: syntax}, TaskRequestEvidenceSourceNorth, true)
	double.Secondary = dbErr
	rejection, ok := ClassifyTaskRequestEvidenceRejection(double)
	require.True(t, ok)
	assert.Equal(t, 400, rejection.Status)
	assert.Equal(t, "invalid_request", rejection.Code)
	require.True(t, errors.Is(double, ErrTaskRequestEvidenceUnavailable))
	var recoveredSyntax *json.SyntaxError
	require.True(t, errors.As(double, &recoveredSyntax))
	require.True(t, errors.Is(double, dbErr))
	assert.Contains(t, rejection.Diagnostic, "secondary=")
	assert.Contains(t, rejection.Diagnostic, common.SanitizeTaskDiagnostic(dbErr.Error()))

	// 双重故障：对象写入失败为主因，完成更新失败为次因，公开类别仍是存储。
	storagePrimary := evidenceStoreWriteFailure(errors.New("mkdir failed"), TaskRequestEvidenceSourceNorth)
	storagePrimary.Secondary = dbErr
	rejection, ok = ClassifyTaskRequestEvidenceRejection(storagePrimary)
	require.True(t, ok)
	assert.Equal(t, 503, rejection.Status)
	assert.Equal(t, "evidence_storage_unavailable", rejection.Code)
	require.True(t, errors.Is(storagePrimary, dbErr))
	assert.Contains(t, rejection.Diagnostic, common.SanitizeTaskDiagnostic(dbErr.Error()))

	// 前序处理成功、仅完成更新失败：数据库故障为主因。
	commitOnly := evidenceUnavailableFailure(TaskRequestEvidenceCategoryEventDatabase, TaskRequestEvidenceStageEventCommit, TaskRequestEvidenceSourceNorth).WithCause(dbErr)
	rejection, ok = ClassifyTaskRequestEvidenceRejection(commitOnly)
	require.True(t, ok)
	assert.Equal(t, 503, rejection.Status)
	assert.Equal(t, "evidence_database_unavailable", rejection.Code)
}

func TestEvidenceErrorTextDoesNotLeakCause(t *testing.T) {
	err := evidenceUnavailableFailure(
		TaskRequestEvidenceCategoryEventDatabase, TaskRequestEvidenceStageEventReserve, TaskRequestEvidenceSourceNorth,
	).WithCause(errors.New("SQLITE_BUSY: database is locked with secret-details"))
	assert.NotContains(t, err.Error(), "SQLITE_BUSY")
	assert.NotContains(t, err.Error(), "secret-details")
	assert.Contains(t, err.Error(), "event_database")
}

func TestEvidenceErrorRejectsNonClassifiedWrapping(t *testing.T) {
	// 文案与诊断不包含原始 cause；调用方 err.Error() 默认格式化不会泄漏。
	err := evidenceRedactionFailure(&evidenceJSONParseError{cause: realJSONSyntaxError(t)}, TaskRequestEvidenceSourceNorth, true)
	assert.NotContains(t, err.Error(), "invalid character")
	assert.False(t, strings.Contains(err.Error(), "{"))
}

// 双重故障集成：已证明的客户 JSON 语法错误为主因，事件完成更新失败为次因。
// 公开类别保持主因 400，事件不伪造完整状态，两条 cause 链可解包。
func TestEvidenceDoubleFailureKeepsPrimaryCategory(t *testing.T) {
	env := setupEvidenceTestEnv(t)
	c := newEvidenceTestContext(t, []byte(`{"model":`))
	c.Request.Header.Set("Content-Type", "application/json")

	sqlDB, err := env.db.DB()
	require.NoError(t, err)
	_, err = sqlDB.Exec("CREATE TRIGGER block_event_commit BEFORE UPDATE ON task_request_evidence_events BEGIN SELECT RAISE(ABORT, 'commit blocked by test trigger'); END;")
	require.NoError(t, err)

	err = BeginTaskRequestEvidence(c, model.TaskRequestEvidenceKindVideoTask)
	require.Error(t, err)

	rejection, ok := ClassifyTaskRequestEvidenceRejection(err)
	require.True(t, ok)
	assert.Equal(t, 400, rejection.Status)
	assert.Equal(t, "invalid_request", rejection.Code)
	assert.Contains(t, rejection.Diagnostic, "secondary=")
	assert.Contains(t, rejection.Diagnostic, "commit blocked by test trigger")

	require.True(t, errors.Is(err, ErrTaskRequestEvidenceUnavailable))
	var syntaxErr *json.SyntaxError
	require.True(t, errors.As(err, &syntaxErr))
	var evidenceErr *TaskRequestEvidenceError
	require.True(t, errors.As(err, &evidenceErr))
	require.NotNil(t, evidenceErr.Secondary)

	var events []model.TaskRequestEvidenceEvent
	require.NoError(t, env.db.Find(&events).Error)
	require.Len(t, events, 1)
	assert.Equal(t, model.TaskRequestEvidencePhaseUnavailable, events[0].Phase)
	assert.False(t, events[0].Complete)
}
