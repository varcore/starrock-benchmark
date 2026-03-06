package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type StarRocksConfig struct {
	Host      string `yaml:"host"`
	MySQLPort int    `yaml:"mysql_port"`
	HTTPPort  int    `yaml:"http_port"`
	User      string `yaml:"user"`
	Password  string `yaml:"password"`
}

func (s StarRocksConfig) MySQLDSN(database string) string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?timeout=60s&readTimeout=300s&writeTimeout=300s&interpolateParams=true",
		s.User, s.Password, s.Host, s.MySQLPort, database)
}

func (s StarRocksConfig) StreamLoadURL(database, table string) string {
	return fmt.Sprintf("http://%s:%d/api/%s/%s/_stream_load",
		s.Host, s.HTTPPort, database, table)
}

type ColumnDef struct {
	Name string `yaml:"name"`
	Type string `yaml:"type"`
}

type ColumnsConfig struct {
	Auto        bool        `yaml:"auto"`
	Count       int         `yaml:"count"`
	Definitions []ColumnDef `yaml:"definitions"`
}

type BenchmarkConfig struct {
	Scenario       string        `yaml:"scenario"`
	LoadMethod     string        `yaml:"load_method"`
	Databases      int           `yaml:"databases"`
	TablesPerDB    int           `yaml:"tables_per_db"`
	TableEngine    string        `yaml:"table_engine"`
	PrimaryKeyType string        `yaml:"primary_key_type"`
	ParallelPerDB  int           `yaml:"parallel_per_db"`
	BatchSize      int           `yaml:"batch_size"`
	BucketCount    int           `yaml:"bucket_count"`
	Duration       string        `yaml:"duration"`
	TotalRecords   int64         `yaml:"total_records"`
	Columns        ColumnsConfig `yaml:"columns"`
	WarmupDuration string        `yaml:"warmup_duration"`
	SeedRecords        int64    `yaml:"seed_records"`
	PartialUpdateMode  string   `yaml:"partial_update_mode"`
	MaxRetries         int      `yaml:"max_retries"`
	UseExisting    bool          `yaml:"use_existing"`
	DeleteData     *bool         `yaml:"delete_data"`
	ResultsFile    string        `yaml:"results_file"`

	ParsedDuration       time.Duration `yaml:"-"`
	ParsedWarmupDuration time.Duration `yaml:"-"`
}

type Config struct {
	StarRocks StarRocksConfig `yaml:"starrocks"`
	Benchmark BenchmarkConfig `yaml:"benchmark"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("config validation: %w", err)
	}

	return cfg, nil
}

func (c *Config) validate() error {
	sr := &c.StarRocks
	if sr.Host == "" {
		sr.Host = "127.0.0.1"
	}
	if sr.MySQLPort == 0 {
		sr.MySQLPort = 9030
	}
	if sr.HTTPPort == 0 {
		sr.HTTPPort = 8030
	}
	if sr.User == "" {
		sr.User = "root"
	}

	b := &c.Benchmark
	b.Scenario = strings.ToLower(b.Scenario)
	if b.Scenario != "ingestion" && b.Scenario != "update" {
		return fmt.Errorf("scenario must be 'ingestion' or 'update', got %q", b.Scenario)
	}

	b.LoadMethod = strings.ToLower(b.LoadMethod)
	if b.LoadMethod != "sql" && b.LoadMethod != "stream_load" {
		return fmt.Errorf("load_method must be 'sql' or 'stream_load', got %q", b.LoadMethod)
	}

	if b.Databases < 1 {
		return fmt.Errorf("databases must be >= 1")
	}
	if b.TablesPerDB < 1 {
		return fmt.Errorf("tables_per_db must be >= 1")
	}

	b.TableEngine = strings.ToUpper(b.TableEngine)
	validEngines := map[string]bool{
		"PRIMARY_KEY":   true,
		"DUPLICATE_KEY": true,
		"UNIQUE_KEY":    true,
		"AGGREGATE_KEY": true,
	}
	if !validEngines[b.TableEngine] {
		return fmt.Errorf("table_engine must be one of PRIMARY_KEY, DUPLICATE_KEY, UNIQUE_KEY, AGGREGATE_KEY, got %q", b.TableEngine)
	}

	b.PrimaryKeyType = strings.ToLower(b.PrimaryKeyType)
	if b.PrimaryKeyType == "" {
		b.PrimaryKeyType = "id"
	}
	if b.PrimaryKeyType != "id" && b.PrimaryKeyType != "id_created_at" {
		return fmt.Errorf("primary_key_type must be 'id' or 'id_created_at', got %q", b.PrimaryKeyType)
	}

	if b.ParallelPerDB < 1 {
		b.ParallelPerDB = 4
	}
	if b.BatchSize < 1 {
		b.BatchSize = 1000
	}
	if b.BucketCount < 1 {
		b.BucketCount = 10
	}

	if b.Duration == "" && b.TotalRecords == 0 {
		return fmt.Errorf("either duration or total_records must be specified")
	}
	if b.Duration != "" && b.TotalRecords > 0 {
		return fmt.Errorf("specify only one of duration or total_records, not both")
	}

	if b.Duration != "" {
		d, err := time.ParseDuration(b.Duration)
		if err != nil {
			return fmt.Errorf("invalid duration %q: %w", b.Duration, err)
		}
		if d <= 0 {
			return fmt.Errorf("duration must be positive")
		}
		b.ParsedDuration = d
	}

	if b.WarmupDuration != "" {
		d, err := time.ParseDuration(b.WarmupDuration)
		if err != nil {
			return fmt.Errorf("invalid warmup_duration %q: %w", b.WarmupDuration, err)
		}
		b.ParsedWarmupDuration = d
	} else {
		b.ParsedWarmupDuration = 10 * time.Second
	}

	if b.MaxRetries < 0 {
		b.MaxRetries = 0
	}
	if b.MaxRetries == 0 {
		b.MaxRetries = 3
	}

	if b.DeleteData == nil {
		defaultTrue := true
		b.DeleteData = &defaultTrue
	}

	if b.Scenario == "update" && b.SeedRecords == 0 {
		b.SeedRecords = 100000
	}

	if b.Scenario == "update" {
		b.PartialUpdateMode = strings.ToLower(b.PartialUpdateMode)
		if b.PartialUpdateMode == "" {
			// Auto-select: "column" mode is better when updating few columns
			// out of many; "row" mode is better when updating most columns.
			totalCols := 2 + len(c.ResolvedColumns()) // id + created_at + user columns
			if totalCols > 10 {
				b.PartialUpdateMode = "column"
			} else {
				b.PartialUpdateMode = "row"
			}
		}
		if b.PartialUpdateMode != "row" && b.PartialUpdateMode != "column" {
			return fmt.Errorf("partial_update_mode must be 'row' or 'column', got %q", b.PartialUpdateMode)
		}
	}

	if err := c.validateColumns(); err != nil {
		return err
	}

	return nil
}

func (c *Config) validateColumns() error {
	cols := &c.Benchmark.Columns
	if cols.Auto {
		if cols.Count < 1 {
			return fmt.Errorf("columns.count must be >= 1 when auto is true")
		}
	} else {
		if len(cols.Definitions) == 0 {
			return fmt.Errorf("columns.definitions must not be empty when auto is false")
		}
		for i, d := range cols.Definitions {
			if d.Name == "" {
				return fmt.Errorf("columns.definitions[%d].name must not be empty", i)
			}
			if d.Type == "" {
				return fmt.Errorf("columns.definitions[%d].type must not be empty", i)
			}
		}
	}
	return nil
}

func (c *Config) ResolvedColumns() []ColumnDef {
	if !c.Benchmark.Columns.Auto {
		return c.Benchmark.Columns.Definitions
	}

	autoTypes := []string{"INT", "BIGINT", "VARCHAR(256)", "DOUBLE", "DATETIME"}
	cols := make([]ColumnDef, c.Benchmark.Columns.Count)
	for i := range cols {
		t := autoTypes[i%len(autoTypes)]
		cols[i] = ColumnDef{
			Name: fmt.Sprintf("col_%d", i+1),
			Type: t,
		}
	}
	return cols
}

func (c *Config) ShouldDeleteData() bool {
	return c.Benchmark.DeleteData == nil || *c.Benchmark.DeleteData
}

func (c *Config) DatabaseName(index int) string {
	return fmt.Sprintf("benchmark_%d", index+1)
}

func (c *Config) TableName(index int) string {
	return fmt.Sprintf("bench_table_%d", index+1)
}
