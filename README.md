# StarRocks Benchmark Tool

A high-performance Go benchmarking tool for StarRocks that tests ingestion and update throughput with configurable parallelism, table engines, and load methods.

## Features

- **Two scenarios**: Max ingestion speed (INSERT) and max update speed (UPDATE)
- **Two load methods**: SQL INSERT (MySQL protocol) and Stream Load (HTTP)
- **Configurable parallelism**: Per-database worker concurrency with semaphore-based throttling
- **Auto schema management**: Creates/drops databases and tables automatically
- **Multiple table engines**: PRIMARY_KEY, DUPLICATE_KEY, UNIQUE_KEY, AGGREGATE_KEY
- **Monthly partitioning**: Automatic RANGE partitioning by `created_at`
- **Flexible columns**: Auto-generate mixed types or define explicit column schemas
- **Real-time terminal display**: Live throughput, latency, and progress metrics
- **Detailed final report**: Per-database and per-table breakdown with latency percentiles
- **JSON export**: Optional machine-readable results for automated comparison
- **Graceful shutdown**: Ctrl+C triggers cleanup (DROP databases) via signal handler

## Prerequisites

- Go 1.22+
- Running StarRocks cluster (FE + BE)

## Build

```bash
go build -o starrock-benchmark ./cmd/benchmark
```

## Usage

```bash
# Using default config.yaml in current directory
./starrock-benchmark

# Specify a config file
./starrock-benchmark --config /path/to/config.yaml
```

## Configuration

Edit `config.yaml` to configure the benchmark. Key parameters:

### StarRocks Connection

```yaml
starrocks:
  host: "127.0.0.1"
  mysql_port: 9030      # MySQL protocol port (for SQL INSERT)
  http_port: 8030       # HTTP port (for Stream Load)
  user: "root"
  password: ""
```

### Benchmark Parameters

| Parameter | Description | Values |
|-----------|-------------|--------|
| `scenario` | Benchmark type | `ingestion` or `update` |
| `load_method` | Data loading method | `sql` or `stream_load` |
| `databases` | Number of databases to create | Integer >= 1 |
| `tables_per_db` | Tables per database | Integer >= 1 |
| `table_engine` | StarRocks table engine | `PRIMARY_KEY`, `DUPLICATE_KEY`, `UNIQUE_KEY`, `AGGREGATE_KEY` |
| `primary_key_type` | Key composition | `id` (BIGINT) or `id_created_at` (BIGINT + DATETIME) |
| `parallel_per_db` | Max concurrent workers per DB | Integer >= 1 |
| `batch_size` | Rows per batch request | Integer >= 1 |
| `bucket_count` | Hash distribution buckets | Integer >= 1 |
| `duration` | Run for this long | Duration string (e.g., `5m`, `1h`) |
| `total_records` | OR insert this many rows total | Integer (use one of duration/total_records) |

### Column Configuration

Auto-generate columns with mixed types:

```yaml
columns:
  auto: true
  count: 20    # generates col_1 through col_20 with rotating INT/BIGINT/VARCHAR/DOUBLE/DATETIME
```

Or define explicit columns:

```yaml
columns:
  auto: false
  definitions:
    - name: "user_name"
      type: "VARCHAR(256)"
    - name: "score"
      type: "DOUBLE"
    - name: "age"
      type: "INT"
```

## Architecture

### Goroutine Hierarchy

```
Main goroutine
├── Signal Handler goroutine (SIGINT/SIGTERM → cancel context)
├── Metrics Collector goroutine (aggregates events from buffered channel)
├── Display goroutine (1s ticker, ANSI terminal refresh)
└── errgroup: Database Coordinators (one per database)
    └── errgroup: Table Workers (semaphore-limited per DB)
        └── Worker loop: generate → load → report metrics
```

### Concurrency Control

- **Per-database semaphore** (`chan struct{}` sized to `parallel_per_db`): ensures total in-flight requests per DB never exceed the configured limit
- **`errgroup`** at every level: automatic cancellation on first fatal error
- **`context.Context`** propagation: timeout/cancel flows to all workers
- **`atomic.Int64`** record counter: lock-free progress tracking for record-count mode
- **Buffered metrics channel**: workers never block on metrics reporting

### Data Flow

```
Generator → Batch → Loader (SQL INSERT or Stream Load) → StarRocks
                                    ↓
                            Metrics Event → Collector → Display
```

## Output

### Real-time Display

During the benchmark, the terminal shows live-updating stats:

```
══════════════════════════════════════════════════════════════════
  Elapsed: 2m30s     | Rows: 5.23M       | Rows/s: 34.8K     | MB/s: 12.45  | Peak Rows/s: 41.2K     | Errors: 0
  Progress: [████████████████████████████░░░░░░░░░░░░░░░░░░░░░░] 56.2%  ETA: 1m57s
──────────────────────────────────────────────────────────────────
  benchmark_1          | Rows: 2.65M       | Rows/s: 17.6K     | MB/s: 6.30   | Batches: 530      | Errors: 0
  benchmark_2          | Rows: 2.58M       | Rows/s: 17.2K     | MB/s: 6.15   | Batches: 516      | Errors: 0
══════════════════════════════════════════════════════════════════
  TOTAL: 5.23M rows | 1867.50 MB
```

### Final Report

After completion, a detailed report prints with:
- Configuration summary
- Throughput: avg and peak rows/sec, MB/sec
- Latency percentiles: min, avg, P50, P95, P99, max
- Per-database and per-table breakdown
- Go runtime stats (goroutines, memory, GC)

## Examples

### Max Ingestion via Stream Load (5 minutes)

```yaml
benchmark:
  scenario: "ingestion"
  load_method: "stream_load"
  databases: 2
  tables_per_db: 5
  parallel_per_db: 10
  batch_size: 5000
  duration: "5m"
```

### Max Update via SQL (1 million records)

```yaml
benchmark:
  scenario: "update"
  load_method: "sql"
  databases: 1
  tables_per_db: 3
  parallel_per_db: 8
  batch_size: 1000
  total_records: 1000000
  seed_records: 500000
```

### Export Results to JSON

```yaml
benchmark:
  results_file: "benchmark_results.json"
```
