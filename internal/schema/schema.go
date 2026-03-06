package schema

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"starrock-benchmark/internal/config"
)

type Manager struct {
	cfg *config.Config
	db  *sql.DB
}

func NewManager(cfg *config.Config, db *sql.DB) *Manager {
	return &Manager{cfg: cfg, db: db}
}

func (m *Manager) CreateAll() error {
	for i := 0; i < m.cfg.Benchmark.Databases; i++ {
		dbName := m.cfg.DatabaseName(i)
		fmt.Printf("Creating database: %s\n", dbName)
		if _, err := m.db.Exec(fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s`", dbName)); err != nil {
			return fmt.Errorf("creating database %s: %w", dbName, err)
		}

		for j := 0; j < m.cfg.Benchmark.TablesPerDB; j++ {
			tableName := m.cfg.TableName(j)
			ddl := m.buildCreateTableDDL(dbName, tableName)
			fmt.Printf("Creating table: %s.%s\n", dbName, tableName)
			if _, err := m.db.Exec(ddl); err != nil {
				return fmt.Errorf("creating table %s.%s: %w", dbName, tableName, err)
			}
		}
	}
	return nil
}

func (m *Manager) EnsureAll() error {
	expectedCols := m.expectedColumnNames()

	for i := 0; i < m.cfg.Benchmark.Databases; i++ {
		dbName := m.cfg.DatabaseName(i)

		dbExists, err := m.databaseExists(dbName)
		if err != nil {
			return fmt.Errorf("checking database %s: %w", dbName, err)
		}

		if !dbExists {
			fmt.Printf("Database %s does not exist, creating...\n", dbName)
			if _, err := m.db.Exec(fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s`", dbName)); err != nil {
				return fmt.Errorf("creating database %s: %w", dbName, err)
			}
		} else {
			fmt.Printf("Database %s already exists, reusing.\n", dbName)
		}

		for j := 0; j < m.cfg.Benchmark.TablesPerDB; j++ {
			tableName := m.cfg.TableName(j)

			tblExists, err := m.tableExists(dbName, tableName)
			if err != nil {
				return fmt.Errorf("checking table %s.%s: %w", dbName, tableName, err)
			}

			if !tblExists {
				fmt.Printf("Table %s.%s does not exist, creating...\n", dbName, tableName)
				ddl := m.buildCreateTableDDL(dbName, tableName)
				if _, err := m.db.Exec(ddl); err != nil {
					return fmt.Errorf("creating table %s.%s: %w", dbName, tableName, err)
				}
				continue
			}

			actualCols, err := m.getTableColumns(dbName, tableName)
			if err != nil {
				return fmt.Errorf("reading columns for %s.%s: %w", dbName, tableName, err)
			}

			if err := m.validateColumns(dbName, tableName, expectedCols, actualCols); err != nil {
				return err
			}

			fmt.Printf("Table %s.%s exists with matching schema, reusing.\n", dbName, tableName)
		}
	}
	return nil
}

func (m *Manager) databaseExists(dbName string) (bool, error) {
	var count int
	err := m.db.QueryRow(
		"SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name = ?", dbName,
	).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (m *Manager) tableExists(dbName, tableName string) (bool, error) {
	var count int
	err := m.db.QueryRow(
		"SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = ? AND table_name = ?",
		dbName, tableName,
	).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

type columnInfo struct {
	Name     string
	DataType string
}

func (m *Manager) getTableColumns(dbName, tableName string) ([]columnInfo, error) {
	rows, err := m.db.Query(
		"SELECT column_name, column_type FROM information_schema.columns WHERE table_schema = ? AND table_name = ? ORDER BY ordinal_position",
		dbName, tableName,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cols []columnInfo
	for rows.Next() {
		var c columnInfo
		if err := rows.Scan(&c.Name, &c.DataType); err != nil {
			return nil, err
		}
		cols = append(cols, c)
	}
	return cols, rows.Err()
}

func (m *Manager) expectedColumnNames() []string {
	names := []string{"id", "created_at"}
	for _, col := range m.cfg.ResolvedColumns() {
		names = append(names, col.Name)
	}
	return names
}

func (m *Manager) validateColumns(dbName, tableName string, expected []string, actual []columnInfo) error {
	actualNames := make(map[string]bool, len(actual))
	for _, c := range actual {
		actualNames[strings.ToLower(c.Name)] = true
	}

	var missing []string
	for _, name := range expected {
		if !actualNames[strings.ToLower(name)] {
			missing = append(missing, name)
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf(
			"table %s.%s exists but is missing expected columns: [%s]. "+
				"Either drop the table and let the tool recreate it, or adjust your column config to match",
			dbName, tableName, strings.Join(missing, ", "),
		)
	}

	return nil
}

func (m *Manager) DropAll() error {
	for i := 0; i < m.cfg.Benchmark.Databases; i++ {
		dbName := m.cfg.DatabaseName(i)
		fmt.Printf("Dropping database: %s\n", dbName)
		if _, err := m.db.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", dbName)); err != nil {
			return fmt.Errorf("dropping database %s: %w", dbName, err)
		}
	}
	return nil
}

func (m *Manager) buildCreateTableDDL(database, table string) string {
	b := &m.cfg.Benchmark
	columns := m.cfg.ResolvedColumns()
	isAggregate := b.TableEngine == "AGGREGATE_KEY"
	compoundKey := b.PrimaryKeyType == "id_created_at"

	var columnLines []string
	columnLines = append(columnLines, "    `id` BIGINT")

	if isAggregate && !compoundKey {
		columnLines = append(columnLines, "    `created_at` DATETIME REPLACE")
	} else {
		columnLines = append(columnLines, "    `created_at` DATETIME")
	}

	for _, col := range columns {
		if isAggregate {
			columnLines = append(columnLines, fmt.Sprintf("    `%s` %s %s", col.Name, col.Type, aggregateFunc(col.Type)))
		} else {
			columnLines = append(columnLines, fmt.Sprintf("    `%s` %s", col.Name, col.Type))
		}
	}

	var keyColumns string
	if compoundKey {
		keyColumns = "`id`, `created_at`"
	} else {
		keyColumns = "`id`"
	}

	engineKW := engineKeyword(b.TableEngine)
	partitions := buildPartitionClause()

	var sb strings.Builder
	fmt.Fprintf(&sb, "CREATE TABLE `%s`.`%s` (\n", database, table)
	fmt.Fprintf(&sb, "%s\n", strings.Join(columnLines, ",\n"))
	fmt.Fprintf(&sb, ")\n")
	fmt.Fprintf(&sb, "ENGINE = OLAP\n")
	fmt.Fprintf(&sb, "%s (%s)\n", engineKW, keyColumns)
	fmt.Fprintf(&sb, "PARTITION BY RANGE(`created_at`) (\n")
	fmt.Fprintf(&sb, "%s\n", partitions)
	fmt.Fprintf(&sb, ")\n")
	fmt.Fprintf(&sb, "DISTRIBUTED BY HASH(`id`) BUCKETS %d", b.BucketCount)

	return sb.String()
}

func engineKeyword(tableEngine string) string {
	switch tableEngine {
	case "PRIMARY_KEY":
		return "PRIMARY KEY"
	case "UNIQUE_KEY":
		return "UNIQUE KEY"
	case "DUPLICATE_KEY":
		return "DUPLICATE KEY"
	case "AGGREGATE_KEY":
		return "AGGREGATE KEY"
	default:
		return "DUPLICATE KEY"
	}
}

func aggregateFunc(colType string) string {
	upper := strings.ToUpper(colType)
	switch {
	case strings.HasPrefix(upper, "BIGINT"),
		strings.HasPrefix(upper, "INT"),
		strings.HasPrefix(upper, "SMALLINT"),
		strings.HasPrefix(upper, "TINYINT"),
		strings.HasPrefix(upper, "LARGEINT"),
		strings.HasPrefix(upper, "FLOAT"),
		strings.HasPrefix(upper, "DOUBLE"),
		strings.HasPrefix(upper, "DECIMAL"):
		return "SUM"
	default:
		return "REPLACE"
	}
}

func buildPartitionClause() string {
	now := time.Now()
	var partitions []string
	for i := -6; i <= 6; i++ {
		t := now.AddDate(0, i, 0)
		monthStart := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
		nextMonth := monthStart.AddDate(0, 1, 0)
		name := fmt.Sprintf("p%d%02d", monthStart.Year(), monthStart.Month())
		partitions = append(partitions, fmt.Sprintf("    PARTITION %s VALUES LESS THAN (\"%s\")", name, nextMonth.Format("2006-01-02")))
	}
	return strings.Join(partitions, ",\n")
}
