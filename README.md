# StarRocks Benchmark Tool

A high-performance Go CLI tool for benchmarking StarRocks ingestion and update throughput with configurable parallelism, table engines, and load methods.

---

## Table of Contents

- [Features](#features)
- [Install on Server (Remote)](#install-on-server-remote)
- [Local Development](#local-development)
- [Configuration Reference](#configuration-reference)
- [Usage](#usage)
- [Release Workflow](#release-workflow)
- [Project Structure](#project-structure)
- [Architecture](#architecture)
- [Output](#output)
- [Examples](#examples)
- [Troubleshooting](#troubleshooting)

---

## Features

- **Two scenarios**: Max ingestion speed (INSERT) and max update speed (UPDATE)
- **Two load methods**: SQL INSERT (MySQL protocol) and Stream Load (HTTP)
- **Configurable parallelism**: `parallel_per_db` controls total concurrent batch goroutines per database
- **Auto schema management**: Creates/drops `benchmark_X` databases and `bench_table_N` tables automatically
- **Reuse existing schema**: Optionally skip creation if matching databases/tables already exist
- **Multiple table engines**: PRIMARY_KEY, DUPLICATE_KEY, UNIQUE_KEY, AGGREGATE_KEY
- **Monthly partitioning**: Automatic RANGE partitioning by `created_at`
- **Flexible columns**: Auto-generate mixed types or define explicit column schemas
- **Partial update modes**: Row-level or column-level partial updates for Stream Load
- **Real-time terminal display**: Live throughput, latency, and progress metrics
- **Detailed final report**: Per-database and per-table breakdown with latency percentiles
- **JSON export**: Optional machine-readable results for automated comparison
- **Graceful shutdown**: Ctrl+C triggers cleanup via signal handler
- **Configurable cleanup**: Choose to drop or keep benchmark databases after completion
- **Version tracking**: `--version` flag shows version, commit, and build time

---

## Install on Server (Remote)

### Option 1: One-liner install (recommended)

```bash
# Install latest version
curl -fsSL "https://raw.githubusercontent.com/varcore/starrock-benchmark/main/scripts/install.sh" | sudo bash

# Install specific version
curl -fsSL "https://raw.githubusercontent.com/varcore/starrock-benchmark/main/scripts/install.sh" | sudo bash -s -- --version 0.2.0
```

### Option 2: Manual download from GitHub Releases

```bash
# Go to releases page and download the .deb for your architecture
wget https://github.com/varcore/starrock-benchmark/releases/download/v0.1.0/starrock-benchmark_0.1.0_amd64.deb

# Install
sudo apt install ./starrock-benchmark_0.1.0_amd64.deb
```

### Option 3: Direct binary (no package manager)

```bash
# Download the binary directly
wget https://github.com/varcore/starrock-benchmark/releases/download/v0.1.0/starrock-benchmark-linux-amd64
chmod +x starrock-benchmark-linux-amd64
sudo mv starrock-benchmark-linux-amd64 /usr/local/bin/starrock-benchmark
```

### After install

```bash
# Edit the config with your StarRocks connection details
sudo nano /etc/starrock-benchmark/config.yaml

# Run
starrock-benchmark --config /etc/starrock-benchmark/config.yaml

# Check version
starrock-benchmark --version

# Upgrade (re-run the install command — it overwrites the binary)
curl -fsSL ... | sudo bash
port STARROCK_BENCH_REPO="varcore/starrock-benchmark
# Uninstall
sudo apt remove starrock-benchmark
```

---

## Local Development

### Prerequisites

- Go 1.22+
- `dpkg-deb` for packaging (macOS: `brew install dpkg`, Linux: pre-installed)
- `gh` (GitHub CLI) for releases: [https://cli.github.com/](https://cli.github.com/)

### Clone and build

```bash
git clone git@github.com:varcore/starrock-benchmark.git
cd starrock-benchmark

# Build for your machine
make build
./bin/starrock-benchmark --config config.yaml

# Build for Linux servers
make build-linux          # linux/amd64
make build-linux-arm64    # linux/arm64
make build-all            # all platforms
```

### Make targets quick reference


| Command                  | What it does                                                         |
| ------------------------ | -------------------------------------------------------------------- |
| `make build`             | Build binary for current OS/arch → `bin/starrock-benchmark`          |
| `make build-linux`       | Cross-compile for linux/amd64 → `bin/starrock-benchmark-linux-amd64` |
| `make build-linux-arm64` | Cross-compile for linux/arm64 → `bin/starrock-benchmark-linux-arm64` |
| `make build-all`         | Build for all platforms                                              |
| `make package-deb`       | Build linux/amd64 binary + `.deb` package → `dist/`                  |
| `make package-deb-arm64` | Build linux/arm64 binary + `.deb` package → `dist/`                  |
| `make install`           | Build and install to `/usr/local/bin/` (local machine)               |
| `make uninstall`         | Remove from `/usr/local/bin/`                                        |
| `make test`              | Run tests                                                            |
| `make vet`               | Run go vet                                                           |
| `make clean`             | Remove `bin/` and `dist/` directories                                |
| `make version`           | Show current version, commit, build time                             |
| `make bump-patch`        | Bump version: 0.1.0 → 0.1.1                                          |
| `make bump-minor`        | Bump version: 0.1.0 → 0.2.0                                          |
| `make bump-major`        | Bump version: 0.1.0 → 1.0.0                                          |
| `make release`           | Build, package, tag, and upload to GitHub Releases                   |
| `make release-patch`     | Bump patch + release                                                 |
| `make release-minor`     | Bump minor + release                                                 |
| `make release-major`     | Bump major + release                                                 |


---

## Release Workflow

This is the workflow you'll repeat every time you push a new version:

```bash
# 1. Make your code changes
#    ... edit files ...

# 2. Commit
git add -A && git commit -m "feat: add new feature"

# 3. Release (bumps version, builds, packages, uploads to GitHub)
make release-patch    # for bug fixes (0.1.0 → 0.1.1)
make release-minor    # for new features (0.1.0 → 0.2.0)
make release-major    # for breaking changes (0.1.0 → 1.0.0)

# 4. On your server, upgrade
curl -fsSL "https://raw.githubusercontent.com/varcore/starrock-benchmark/main/scripts/install.sh" | sudo bash
```

### What `make release` does behind the scenes

1. Bumps `VERSION` file (if `release-patch/minor/major`)
2. Cross-compiles binaries for linux/amd64 and linux/arm64
3. Packages both into `.deb` files
4. Creates a git tag `vX.Y.Z`
5. Pushes the tag to GitHub
6. Creates a GitHub Release with release notes
7. Uploads all 4 artifacts (.deb + binary for both architectures)

### Manual release (without make)

```bash
# Bump version
./scripts/bump-version.sh patch

# Build and release
./scripts/release.sh
```

---

## Configuration Reference

Copy `config.yaml.example` to `config.yaml` and edit:

```bash
cp config.yaml.example config.yaml
nano config.yaml
```

### StarRocks Connection

```yaml
starrocks:
  host: "192.168.1.100"       # StarRocks FE host (IP or hostname)
  mysql_port: 9030             # MySQL protocol port
  http_port: 8030              # HTTP port (for Stream Load)
  user: "root"                 # StarRocks user
  password: "your_password"    # StarRocks password
```

### Benchmark Parameters


| Parameter             | Type   | Default         | Description                                                                         |
| --------------------- | ------ | --------------- | ----------------------------------------------------------------------------------- |
| `scenario`            | string | `ingestion`     | `ingestion` (INSERT new rows) or `update` (UPDATE existing rows)                    |
| `load_method`         | string | `sql`           | `sql` (MySQL INSERT) or `stream_load` (HTTP Stream Load)                            |
| `databases`           | int    | `1`             | Number of databases (`benchmark_1`, `benchmark_2`, ...)                             |
| `tables_per_db`       | int    | `1`             | Tables per database (`bench_table_1`, `bench_table_2`, ...)                         |
| `table_engine`        | string | `DUPLICATE_KEY` | `PRIMARY_KEY`, `DUPLICATE_KEY`, `UNIQUE_KEY`, `AGGREGATE_KEY`                       |
| `primary_key_type`    | string | `id`            | `id` (BIGINT only) or `id_created_at` (BIGINT + DATETIME)                           |
| `parallel_per_db`     | int    | `4`             | Total concurrent batch goroutines per database                                      |
| `batch_size`          | int    | `5000`          | Rows per batch INSERT / Stream Load request                                         |
| `bucket_count`        | int    | `10`            | Hash distribution buckets for tables                                                |
| `duration`            | string | `1m`            | Run for this long (e.g. `30s`, `5m`, `1h`). Mutually exclusive with `total_records` |
| `total_records`       | int    | —               | Stop after this many total rows. Mutually exclusive with `duration`                 |
| `warmup_duration`     | string | `10s`           | Warm-up period before collecting metrics                                            |
| `seed_records`        | int    | `100000`        | Rows to pre-load per DB before update benchmark                                     |
| `partial_update_mode` | string | auto            | `row` or `column`. Auto-selects based on column count if empty                      |
| `max_retries`         | int    | `3`             | Max retries with exponential backoff for transient failures                         |
| `use_existing`        | bool   | `false`         | Reuse existing `benchmark_X` databases/tables if schema matches                     |
| `delete_data`         | bool   | `true`          | Drop benchmark databases after completion                                           |
| `results_file`        | string | —               | Path to write JSON results (optional)                                               |


### Parallelism explained

`parallel_per_db` controls the total number of concurrent batch requests per database. Workers are distributed evenly across tables.

Example: `parallel_per_db=10`, `tables_per_db=2`, `batch_size=5000`

- 5 workers per table (10 / 2)
- Each worker sends 5,000 rows per batch
- 10 batches in-flight simultaneously = 50,000 rows in-flight per database

### Column Configuration

Auto-generate columns with mixed types:

```yaml
columns:
  auto: true
  count: 20    # col_1 through col_20 with rotating INT/BIGINT/VARCHAR/DOUBLE/DATETIME
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

---

## Usage

```bash
# Run with default config.yaml in current directory
starrock-benchmark

# Specify config file path
starrock-benchmark --config /path/to/config.yaml

# Check version
starrock-benchmark --version
```

---

## Project Structure

```
starrock-benchmark/
├── cmd/benchmark/
│   └── main.go              # CLI entry point, flag parsing, config display
├── internal/
│   ├── config/
│   │   └── config.go        # YAML config structs, loading, validation, defaults
│   ├── schema/
│   │   └── schema.go        # CREATE/DROP databases and tables, reuse logic
│   ├── generator/
│   │   └── generator.go     # Random data generation for all column types
│   ├── loader/
│   │   ├── loader.go        # Loader interface
│   │   ├── sql_loader.go    # SQL INSERT via MySQL protocol
│   │   └── stream_loader.go # HTTP Stream Load with redirect handling
│   ├── metrics/
│   │   ├── collector.go     # Event aggregation, per-DB/table stats
│   │   ├── display.go       # Real-time ANSI terminal rendering
│   │   └── report.go        # Final summary report + JSON export
│   └── runner/
│       └── runner.go        # Benchmark orchestrator, worker spawning, cleanup
├── scripts/
│   ├── build-deb.sh         # Builds .deb package from compiled binary
│   ├── bump-version.sh      # Bumps VERSION file (patch/minor/major)
│   ├── install.sh           # Remote install script (curl | bash)
│   └── release.sh           # Full release: build + package + GitHub Release
├── config.yaml.example      # Sample config (safe to commit, no credentials)
├── Makefile                  # Build, package, release, install targets
├── VERSION                   # Current version (single source of truth)
├── go.mod
├── go.sum
└── .gitignore
```

---

## Architecture

### Goroutine Hierarchy

```
Main goroutine
├── Signal Handler (SIGINT/SIGTERM → cancel context)
├── Metrics Collector (aggregates events from buffered channel)
├── Display (1s ticker, ANSI terminal refresh)
└── errgroup: Database Coordinators (one per database)
    └── Workers (parallel_per_db goroutines, distributed across tables)
        └── Worker loop: generate batch → load → report metrics → repeat
```

### Concurrency Model

- `**parallel_per_db**` goroutines per database, evenly distributed across tables
- `**errgroup**` at every level for automatic cancellation on first fatal error
- `**context.Context**` propagation: timeout/cancel flows to all workers
- `**atomic.Int64**` record counter: lock-free progress tracking for record-count mode
- **Buffered metrics channel**: workers never block on metrics reporting

### Data Flow

```
Generator → Batch ([][]interface{}) → Loader (SQL or Stream Load) → StarRocks
                                             ↓
                                      Metrics Event → Collector → Terminal Display
                                                                → Final Report
                                                                → JSON File
```

---

## Output

### Real-time Terminal Display

```
══════════════════════════════════════════════════════════════════
  Elapsed: 2m30s  | Rows: 5.23M    | Rows/s: 34.8K  | MB/s: 12.45
  Peak Rows/s: 41.2K  | Errors: 0
  Progress: [████████████████████████████░░░░░░░░░░░░░░] 56.2%  ETA: 1m57s
──────────────────────────────────────────────────────────────────
  benchmark_1  | Rows: 2.65M  | Rows/s: 17.6K  | Batches: 530  | Errors: 0
  benchmark_2  | Rows: 2.58M  | Rows/s: 17.2K  | Batches: 516  | Errors: 0
══════════════════════════════════════════════════════════════════
```

### Final Report

After completion, a detailed report prints with:

- Configuration summary
- Throughput: avg and peak rows/sec, MB/sec
- Latency percentiles: min, avg, P50, P95, P99, max
- Per-database and per-table breakdown
- Go runtime stats (goroutines, memory, GC)

---

## Examples

### Max ingestion via Stream Load, 5 minutes

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

### Max update via SQL, 1 million records

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

### Keep data after benchmark for inspection

```yaml
benchmark:
  delete_data: false
  use_existing: true     # reuse on next run
```

### Export results to JSON

```yaml
benchmark:
  results_file: "benchmark_results.json"
```

---

## Troubleshooting

### `context deadline exceeded` during preflight

The preflight check sends a small test batch before the real benchmark. If it times out, your StarRocks cluster may be slow to respond or unreachable. The tool uses a 2-minute timeout with generous MySQL DSN timeouts (60s connect, 300s read/write). Verify network connectivity to your StarRocks FE host.

### `Error 1295: This command is not supported in the prepared statement protocol`

This is handled automatically. The SQL loader inlines values into the query string instead of using `?` placeholders, which avoids StarRocks' prepared statement limitations.

### Stream Load redirects to internal hostname

StarRocks FE redirects Stream Load requests (307) to a BE node. If the BE hostname is internal/unreachable, the tool automatically rewrites the redirect URL to use the FE host IP with the BE port. No configuration needed.

### `dpkg-deb: command not found` when packaging on macOS

```bash
brew install dpkg
```

### `gh: command not found` when releasing

Install the GitHub CLI: [https://cli.github.com/](https://cli.github.com/)

```bash
# macOS
brew install gh

# Linux
sudo apt install gh

# Then authenticate
gh auth login
```

