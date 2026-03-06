package loader

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/go-sql-driver/mysql"

	"starrock-benchmark/internal/config"
	"starrock-benchmark/internal/generator"
)

type SQLLoader struct {
	db *sql.DB
}

func NewSQLLoader(cfg config.StarRocksConfig, database string, maxConns int) (*SQLLoader, error) {
	db, err := sql.Open("mysql", cfg.MySQLDSN(database))
	if err != nil {
		return nil, fmt.Errorf("open sql connection: %w", err)
	}
	db.SetMaxOpenConns(maxConns)
	db.SetMaxIdleConns(maxConns)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping starrocks: %w", err)
	}

	return &SQLLoader{db: db}, nil
}

func (l *SQLLoader) Load(ctx context.Context, schema generator.TableSchema, batch *generator.Batch) (int, error) {
	if len(batch.Rows) == 0 {
		return 0, nil
	}

	var colList strings.Builder
	colList.WriteString("id, created_at")
	for _, c := range schema.Columns {
		colList.WriteString(", ")
		colList.WriteString(c.Name)
	}

	var query strings.Builder
	query.WriteString("INSERT INTO ")
	query.WriteString(schema.Table)
	query.WriteString(" (")
	query.WriteString(colList.String())
	query.WriteString(") VALUES ")

	for i, row := range batch.Rows {
		if i > 0 {
			query.WriteString(", ")
		}
		query.WriteByte('(')
		for j, v := range row {
			if j > 0 {
				query.WriteString(", ")
			}
			query.WriteString(sqlValue(v))
		}
		query.WriteByte(')')
	}

	_, err := l.db.ExecContext(ctx, query.String())
	if err != nil {
		return 0, fmt.Errorf("exec batch insert: %w", err)
	}

	return len(batch.Rows), nil
}

func (l *SQLLoader) Close() error {
	return l.db.Close()
}

func sqlValue(v interface{}) string {
	switch val := v.(type) {
	case nil:
		return "NULL"
	case int:
		return fmt.Sprintf("%d", val)
	case int8:
		return fmt.Sprintf("%d", val)
	case int16:
		return fmt.Sprintf("%d", val)
	case int32:
		return fmt.Sprintf("%d", val)
	case int64:
		return fmt.Sprintf("%d", val)
	case float32:
		return fmt.Sprintf("%g", val)
	case float64:
		return fmt.Sprintf("%g", val)
	case string:
		return "'" + sqlEscape(val) + "'"
	default:
		return "'" + sqlEscape(fmt.Sprintf("%v", val)) + "'"
	}
}

func sqlEscape(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '\'':
			b.WriteString("\\'")
		case '\\':
			b.WriteString("\\\\")
		case '\x00':
			b.WriteString("\\0")
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}
