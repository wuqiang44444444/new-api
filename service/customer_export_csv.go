package service

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/QuantumNous/new-api/model"
)

// Customer export CSV streaming writer. Rows are written sequentially to
// bounded temp shards (plan 7.3.6); a new file starts at a row boundary once
// the shard cap is reached, and the file list is only published after every
// shard finished. Usage exports keep versioned machine columns; customer
// statement details use a localized projection over those same safe values.

const (
	customerExportFieldVersion          = 28
	customerStatementExportFieldVersion = 28
	customerExportCsvBOM                = "\xEF\xBB\xBF"
	// Plan-suggested starting cap for one CSV shard (100 MiB).
	customerExportCsvShardBytes = 100 << 20
)

type customerExportScopeColumns struct {
	Language       string
	QuotaPerUnit   float64
	Currency       string
	CurrencyRate   float64
	JobID          string
	ExportType     string
	GeneratedAt    int64
	PeriodStart    int64
	PeriodEnd      int64
	Timezone       string
	CustomerId     int
	CustomerName   string
	ReportCoverage *model.BillingExportCoverage
}

// customerExportCsvWriter writes rows sequentially to bounded temp shards.
type customerExportCsvWriter struct {
	header            []string
	statementLanguage *string
	dir               string
	prefix            string
	maxShardBytes     int64
	statementPart     bool
	files             []model.CustomerExportArtifactFile
	writer            *csv.Writer
	buffered          *bufio.Writer
	file              *os.File
	path              string
	bytesInShard      int64
	shardIndex        int
	shardLines        int64
	totalLines        int64
	totalBytes        int64
	paths             []string
}

func newCustomerExportCsvWriter(dir string, prefix string, maxShardBytes int64) (*customerExportCsvWriter, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &customerExportCsvWriter{
		dir:           dir,
		prefix:        prefix,
		maxShardBytes: maxShardBytes,
	}, nil
}

func (w *customerExportCsvWriter) currentShardName() string {
	if w.shardIndex == 0 {
		return w.prefix + ".csv"
	}
	return fmt.Sprintf("%s.part%d.csv", w.prefix, w.shardIndex+1)
}

func (w *customerExportCsvWriter) ensureShard() error {
	if w.file != nil {
		return nil
	}
	file, err := os.CreateTemp(w.dir, "shard-*")
	if err != nil {
		return err
	}
	w.file = file
	w.path = file.Name()
	w.buffered = bufio.NewWriter(file)
	// UTF-8 BOM so spreadsheet applications detect the encoding.
	if _, err := w.buffered.WriteString(customerExportCsvBOM); err != nil {
		return err
	}
	w.writer = csv.NewWriter(w.buffered)
	if err := w.writer.Write(w.header); err != nil {
		return err
	}
	var header bytes.Buffer
	hw := csv.NewWriter(&header)
	_ = hw.Write(w.header)
	hw.Flush()
	w.bytesInShard = int64(len(customerExportCsvBOM) + header.Len())
	w.shardLines = 0
	return nil
}

func (w *customerExportCsvWriter) rotateShard() error {
	if err := w.closeShard(); err != nil {
		return err
	}
	w.shardIndex++
	return nil
}

func (w *customerExportCsvWriter) closeShard() error {
	if w.file == nil {
		return nil
	}
	w.writer.Flush()
	if err := w.writer.Error(); err != nil {
		_ = w.file.Close()
		return err
	}
	if err := w.buffered.Flush(); err != nil {
		_ = w.file.Close()
		return err
	}
	if err := w.file.Close(); err != nil {
		return err
	}
	info, err := os.Stat(w.path)
	if err != nil {
		return err
	}
	digest, err := streamFileSha256(w.path)
	if err != nil {
		return err
	}
	w.files = append(w.files, model.CustomerExportArtifactFile{
		ObjectKey: "", FileName: w.currentShardName(), SizeBytes: info.Size(),
		LineCount: w.shardLines, Sha256: digest,
	})
	w.paths = append(w.paths, w.path)
	w.totalBytes += info.Size()
	w.file = nil
	w.buffered = nil
	w.writer = nil
	w.path = ""
	w.bytesInShard = 0
	w.shardLines = 0
	return nil
}

func (w *customerExportCsvWriter) AppendRow(row model.CustomerExportRow, scope customerExportScopeColumns) error {
	if w.statementLanguage != nil {
		return w.AppendRecord(customerStatementDetailRecord(*w.statementLanguage, row, scope, false))
	}
	if w.header == nil {
		w.header = csvLabels("zh", scope.Currency, adminUsageRecordColumns)
	}
	return w.AppendRecord(customerStatementDetailRecord("zh", row, scope, true))
}

// AppendRecord applies the shared size, sharding and durability limits to a CSV schema.
func (w *customerExportCsvWriter) AppendRecord(record []string) error {
	if err := w.ensureShard(); err != nil {
		return err
	}
	var encoded bytes.Buffer
	encoder := csv.NewWriter(&encoded)
	if err := encoder.Write(record); err != nil {
		return err
	}
	encoder.Flush()
	if err := encoder.Error(); err != nil {
		return err
	}
	rowBytes := int64(encoded.Len())
	if rowBytes > 1<<20 || (!w.statementPart && w.totalBytes+w.bytesInShard+rowBytes > customerExportTotalBytes) {
		return errCustomerExportBudgetExceeded
	}
	if (w.statementPart || w.shardLines > 0) && w.bytesInShard+rowBytes > w.maxShardBytes {
		if w.statementPart {
			return errCustomerStatementExportPartFull
		}
		if err := w.rotateShard(); err != nil {
			return err
		}
		if err := w.ensureShard(); err != nil {
			return err
		}
	}
	if !w.statementPart && w.totalBytes+w.bytesInShard+rowBytes > customerExportTotalBytes {
		return errCustomerExportBudgetExceeded
	}
	if err := w.writer.Write(record); err != nil {
		return err
	}
	w.shardLines++
	w.totalLines++
	w.bytesInShard += rowBytes
	return nil
}

// errCustomerExportNoFiles 表示范围内没有匹配行；这不是失败。
var errCustomerExportNoFiles = errors.New("export produced no files")

func (w *customerExportCsvWriter) Finish() ([]model.CustomerExportArtifactFile, []string, int64, int64, error) {
	if err := w.closeShard(); err != nil {
		return nil, nil, 0, 0, err
	}
	if len(w.files) == 0 {
		return nil, nil, 0, 0, errCustomerExportNoFiles
	}
	return w.files, w.paths, w.totalLines, w.totalBytes, nil
}

// Cleanup removes the temp directory; used on failure and after successful upload.
func (w *customerExportCsvWriter) Cleanup() {
	if w.file != nil {
		_ = w.file.Close()
		w.file = nil
	}
	if w.dir != "" {
		_ = os.RemoveAll(w.dir)
	}
}

func exportYesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func exportRatio(ratio *float64) string {
	if ratio == nil {
		return ""
	}
	return strconv.FormatFloat(*ratio, 'f', -1, 64)
}

func formatExportTimestamp(timestamp int64) string {
	return time.Unix(timestamp, 0).In(billingExportTimezone()).Format("2006-01-02 15:04:05")
}

func billingExportTimezone() *time.Location {
	return time.FixedZone("Asia/Shanghai", 8*60*60)
}

func streamFileSha256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", digest.Sum(nil)), nil
}

// exportCsvCellGuard mirrors the frontend escapeCsvCell formula-injection
// defense: a dangerous leading character (after whitespace) gets a leading
// apostrophe so spreadsheets treat the cell as text.
func exportCsvCellGuard(value string) string {
	trimmed := strings.TrimLeftFunc(value, unicode.IsSpace)
	if trimmed == "" {
		return value
	}
	switch trimmed[0] {
	case '=', '+', '@', '-':
		return "'" + value
	}
	return value
}
