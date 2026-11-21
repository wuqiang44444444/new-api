package model

import (
	"context"
	"strings"
	"time"

	"gorm.io/gorm"
)

type historicalProjectionCopy struct {
	hasRows bool
	issues  int64
	resumed bool
}

// Copy exact keys from the immutable generation in short commits. The new
// generation is invisible until the normal stage and publication checks pass.
func copyDailyReportProjection(ctx context.Context, db *gorm.DB, projection any, stage dailyReportStage, limit int) (historicalProjectionCopy, error) {
	limit = dailyReportWriteBatchSize(ctx, limit)
	if lowPriorityDailyReport(ctx) {
		switch projection.(type) {
		case *dailyReportPayloadPart, *billingReportUpstreamDay, *billingReportCustomerDay, *usageReportDayRow:
			// Old projections can have larger inline payloads.
			limit = 1
		}
	}
	stmt := &gorm.Statement{DB: db}
	if err := stmt.Parse(projection); err != nil {
		return historicalProjectionCopy{}, err
	}
	columns := make([]string, 0, len(stmt.Schema.DBNames))
	values := make([]string, 0, len(stmt.Schema.DBNames))
	for _, name := range stmt.Schema.DBNames {
		columns = append(columns, stmt.Quote(name))
		if name == "generation" {
			values = append(values, "?")
		} else {
			values = append(values, stmt.Quote(name))
		}
	}
	keys := make([]string, 0, len(stmt.Schema.PrimaryFields)-1)
	for _, key := range stmt.Schema.PrimaryFields {
		if key.DBName != "generation" {
			keys = append(keys, stmt.Quote(key.DBName))
		}
	}
	_, countIssues := projection.(*billingReportLine)
	_, excludeUsageMarker := projection.(*usageReportDayRow)
	table, order := stmt.Quote(stmt.Schema.Table), strings.Join(keys, ", ")
	selectSQL := "SELECT " + order
	if countIssues {
		selectSQL += ", CASE WHEN " + dailyReportDiagnosticIssuePredicate + " THEN 1 ELSE 0 END"
	}
	selectSQL += " FROM " + table + " WHERE generation = ?"
	if excludeUsageMarker {
		selectSQL += " AND user_id <> -1"
	}
	insertSQL := "INSERT INTO " + table + " (" + strings.Join(columns, ", ") + ") SELECT " +
		strings.Join(values, ", ") + " FROM " + table + " WHERE generation = ? AND "

	if err := usageReportBeforeBatch(ctx); err != nil {
		return historicalProjectionCopy{}, err
	}
	latest := "SELECT " + order + " FROM " + table + " WHERE generation = ?"
	if excludeUsageMarker {
		latest += " AND user_id <> -1"
	}
	latest += " ORDER BY " + strings.Join(keys, " DESC, ") + " DESC LIMIT 1"
	readCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	committed, _, err := readHistoricalProjectionKeys(db.WithContext(readCtx), latest, []any{stage.Generation}, len(keys), false)
	cancel()
	if err != nil {
		return historicalProjectionCopy{}, err
	}
	var after []any
	result := historicalProjectionCopy{hasRows: len(committed) != 0, resumed: len(committed) != 0}
	if result.resumed {
		after = committed[0]
	}
	for {
		if err := usageReportBeforeBatch(ctx); err != nil {
			return historicalProjectionCopy{}, err
		}
		query, args := selectSQL, make([]any, 0, 8)
		if countIssues {
			args = append(args, true)
		}
		args = append(args, stage.Previous)
		if after != nil {
			steps := make([]string, len(keys))
			for i, key := range keys {
				equal := make([]string, 0, i+1)
				for j := 0; j < i; j++ {
					equal = append(equal, keys[j]+" = ?")
				}
				steps[i] = "(" + strings.Join(append(equal, key+" > ?"), " AND ") + ")"
				args = append(args, after[:i+1]...)
			}
			query += " AND (" + strings.Join(steps, " OR ") + ")"
		}
		readCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		batch, batchIssues, err := readHistoricalProjectionKeys(db.WithContext(readCtx), query+" ORDER BY "+order+" LIMIT ?",
			append(args, limit), len(keys), countIssues)
		cancel()
		if err != nil || len(batch) == 0 {
			return result, err
		}
		if err := beforeDailyReportWrite(ctx, len(batch)); err != nil {
			return historicalProjectionCopy{}, err
		}
		args = []any{stage.Generation, stage.Previous}
		if len(keys) == 1 {
			ids := make([]any, 0, len(batch))
			for _, row := range batch {
				ids = append(ids, row[0])
			}
			query = keys[0] + " IN ?"
			args = append(args, ids)
		} else {
			match := "(" + strings.Join(keys, " = ? AND ") + " = ?)"
			matches := make([]string, 0, len(batch))
			for _, row := range batch {
				matches = append(matches, match)
				args = append(args, row...)
			}
			query = "(" + strings.Join(matches, " OR ") + ")"
		}
		if err := executeDailyReportBatch(ctx, func(ctx context.Context) error {
			writeCtx, stop := context.WithTimeout(ctx, dailyReportBatchTimeout(ctx))
			defer stop()
			inserted := db.WithContext(writeCtx).Exec(insertSQL+query, args...)
			if inserted.Error != nil {
				return inserted.Error
			}
			if inserted.RowsAffected != int64(len(batch)) {
				return errDailyReportSourceChanged
			}
			return nil
		}); err != nil {
			return historicalProjectionCopy{}, err
		}
		result.hasRows = true
		result.issues += batchIssues
		if len(batch) < limit {
			return result, nil
		}
		after = batch[len(batch)-1]
	}
}

func readHistoricalProjectionKeys(db *gorm.DB, query string, args []any, keyCount int, countIssues bool) ([][]any, int64, error) {
	rows, err := db.Raw(query, args...).Rows()
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var batch [][]any
	var issues int64
	for rows.Next() {
		keys := make([]any, keyCount)
		dest := make([]any, 0, keyCount+1)
		for i := range keys {
			dest = append(dest, &keys[i])
		}
		var issue int64
		if countIssues {
			dest = append(dest, &issue)
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, 0, err
		}
		for i, key := range keys {
			if raw, ok := key.([]byte); ok {
				keys[i] = string(raw)
			}
		}
		batch = append(batch, keys)
		issues += issue
	}
	return batch, issues, rows.Err()
}
