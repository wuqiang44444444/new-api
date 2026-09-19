package model

import (
	"fmt"
	"slices"

	"gorm.io/gorm"
)

// managedIndex records enough metadata to avoid dropping a deployment's custom
// unique, partial, expression, prefix, descending or differently collated index.
// GORM's PostgreSQL GetIndexes does not preserve column order, so inspect the
// catalogs explicitly. These queries also work on MySQL 5.7 and PostgreSQL 9.6.
type managedIndex struct {
	columns []string
	unique  bool
	plain   bool
}

func inspectManagedIndex(db *gorm.DB, value any, name string) (*managedIndex, error) {
	stmt := &gorm.Statement{DB: db}
	if err := stmt.Parse(value); err != nil {
		return nil, err
	}
	var rows []struct {
		ColumnName string
		IsUnique   bool
		IsPlain    bool
	}
	var err error
	switch db.Dialector.Name() {
	case "sqlite":
		err = db.Raw(`SELECT x.name AS column_name, l."unique" AS is_unique,
 l.partial = 0 AND x.desc = 0 AND x.coll = 'BINARY' AND x.cid >= 0 AS is_plain
 FROM pragma_index_list(?) l JOIN pragma_index_xinfo(l.name) x ON x.key = 1
 WHERE l.name = ? ORDER BY x.seqno`, stmt.Table, name).Scan(&rows).Error
	case "mysql":
		err = db.Raw(`SELECT column_name, non_unique = 0 AS is_unique,
 index_type = 'BTREE' AND sub_part IS NULL AND collation = 'A' AS is_plain
 FROM information_schema.statistics
 WHERE table_schema = DATABASE() AND table_name = ? AND index_name = ?
 ORDER BY seq_in_index`, stmt.Table, name).Scan(&rows).Error
	case "postgres":
		err = db.Raw(`SELECT COALESCE(a.attname, '') AS column_name, i.indisunique AS is_unique,
 i.indisvalid AND i.indisready AND NOT i.indisprimary
 AND i.indpred IS NULL AND i.indexprs IS NULL AND am.amname = 'btree'
 AND i.indoption[k.n] = 0 AND i.indcollation[k.n] = a.attcollation AS is_plain
 FROM pg_index i JOIN pg_class c ON c.oid = i.indexrelid
 JOIN pg_am am ON am.oid = c.relam
 CROSS JOIN LATERAL generate_subscripts(i.indkey, 1) k(n)
 LEFT JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = i.indkey[k.n]
 WHERE i.indrelid = to_regclass(?) AND c.relname = ? ORDER BY k.n`, stmt.Table, name).Scan(&rows).Error
	default:
		return nil, fmt.Errorf("unsupported index migration dialect %q", db.Dialector.Name())
	}
	if err != nil {
		return nil, fmt.Errorf("inspect index %s: %w", name, err)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	index := &managedIndex{unique: rows[0].IsUnique, plain: true}
	for _, row := range rows {
		index.columns = append(index.columns, row.ColumnName)
		index.plain = index.plain && row.IsPlain
	}
	return index, nil
}

// retireRedundantIndex only handles explicitly reviewed pairs, never discovers
// or deletes arbitrary prefix indexes. The replacement must already be usable.
func retireRedundantIndex(db *gorm.DB, value any, oldName string, oldColumns []string, replacementName string, replacementColumns []string, replacementUnique bool) error {
	old, err := inspectManagedIndex(db, value, oldName)
	if err != nil || old == nil {
		return err
	}
	if old.unique || !old.plain || !slices.Equal(old.columns, oldColumns) {
		return fmt.Errorf("refusing to retire unexpected index %s", oldName)
	}
	replacement, err := inspectManagedIndex(db, value, replacementName)
	if err != nil {
		return err
	}
	if replacement == nil || !replacement.plain || replacement.unique != replacementUnique || !slices.Equal(replacement.columns, replacementColumns) {
		return fmt.Errorf("replacement index %s is missing or unexpected", replacementName)
	}
	return db.Migrator().DropIndex(value, oldName)
}

func migrateCasbinRuleIndex(db *gorm.DB) error {
	columns := []string{"ptype", "v0", "v1", "v2", "v3", "v4", "v5"}
	return retireRedundantIndex(db, &CasbinRule{}, "idx_casbin_rule", columns, "idx_casbin_rule_unique", columns, true)
}

// Metadata only; this temporary index keeps the sort path available across
// MySQL's implicit DDL commits. A restart resumes after any interrupted step.
type logSortRepairIndex struct {
	CreatedAt int64 `gorm:"index:idx_logs_sort_repair,priority:1"`
	ID        int   `gorm:"index:idx_logs_sort_repair,priority:2"`
}

func migrateLogReadWriteIndexes(db *gorm.DB) error {
	const name = "idx_created_at_id"
	const repairName = "idx_logs_sort_repair"
	columns := []string{"created_at", "id"}
	index, err := inspectManagedIndex(db, &Log{}, name)
	if err != nil {
		return err
	}
	if index != nil && (index.unique || !index.plain || (!slices.Equal(index.columns, columns) && !slices.Equal(index.columns, []string{"id", "created_at"}))) {
		return fmt.Errorf("refusing to replace unexpected index %s", name)
	}
	if index != nil && !slices.Equal(index.columns, columns) {
		stmt := &gorm.Statement{DB: db}
		if err := stmt.Parse(&Log{}); err != nil {
			return err
		}
		repair, err := inspectManagedIndex(db, &Log{}, repairName)
		if err != nil {
			return err
		}
		if repair == nil {
			if err := db.Table(stmt.Table).Migrator().CreateIndex(&logSortRepairIndex{}, repairName); err != nil {
				return err
			}
		} else if repair.unique || !repair.plain || !slices.Equal(repair.columns, columns) {
			return fmt.Errorf("unexpected repair index %s", repairName)
		}
		if err := db.Migrator().DropIndex(&Log{}, name); err != nil {
			return err
		}
		index = nil
	}
	if index == nil {
		if err := db.Migrator().CreateIndex(&Log{}, name); err != nil {
			return err
		}
	}
	if err := retireRedundantIndex(db, &Log{}, repairName, columns, name, columns, false); err != nil {
		return err
	}
	if err := retireRedundantIndex(db, &Log{}, "idx_logs_user_id", []string{"user_id"}, "idx_user_id_id", []string{"user_id", "id"}, false); err != nil {
		return err
	}
	return retireRedundantIndex(db, &Log{}, "idx_logs_model_name", []string{"model_name"}, "index_username_model_name", []string{"model_name", "username"}, false)
}
