package runner

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"golang.org/x/sync/errgroup"

	"starrock-benchmark/internal/config"
	"starrock-benchmark/internal/generator"
	"starrock-benchmark/internal/loader"
	"starrock-benchmark/internal/metrics"
	"starrock-benchmark/internal/schema"
)

type Runner struct {
	cfg           *config.Config
	collector     *metrics.Collector
	display       *metrics.Display
	recordCounter atomic.Int64
	cleanupOnce   sync.Once
	schemaMgr     *schema.Manager
}

func New(cfg *config.Config) *Runner {
	return &Runner{cfg: cfg}
}

func (r *Runner) Run() error {
	adminDB, err := sql.Open("mysql", r.cfg.StarRocks.MySQLDSN(""))
	if err != nil {
		return fmt.Errorf("connecting to StarRocks: %w", err)
	}
	defer adminDB.Close()

	if err := adminDB.Ping(); err != nil {
		return fmt.Errorf("pinging StarRocks: %w", err)
	}
	fmt.Println("Connected to StarRocks successfully.")

	r.schemaMgr = schema.NewManager(r.cfg, adminDB)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	defer r.cleanup()

	if r.cfg.Benchmark.UseExisting {
		fmt.Println("\n=== Schema: Checking existing databases/tables ===")
		if err := r.schemaMgr.EnsureAll(); err != nil {
			return fmt.Errorf("ensuring schema: %w", err)
		}
		fmt.Println("Schema check complete.")
	} else {
		fmt.Println("\n=== Creating Schema ===")
		if err := r.schemaMgr.CreateAll(); err != nil {
			return fmt.Errorf("creating schema: %w", err)
		}
		fmt.Println("Schema created successfully.")
	}

	fmt.Println("\n=== Preflight Check: testing single batch load ===")
	if err := r.preflightCheck(); err != nil {
		return fmt.Errorf("preflight check failed: %w", err)
	}
	fmt.Println("Preflight check passed.")

	if r.cfg.Benchmark.Scenario == "update" {
		if err := r.runSeedPhase(sigCh); err != nil {
			return fmt.Errorf("seed phase: %w", err)
		}
	}

	if err := r.runBenchmark(sigCh); err != nil {
		return fmt.Errorf("benchmark: %w", err)
	}

	return nil
}

func (r *Runner) runSeedPhase(sigCh chan os.Signal) error {
	fmt.Printf("\n=== Seeding Phase: inserting %d rows per database ===\n", r.cfg.Benchmark.SeedRecords)

	collector := metrics.NewCollector(10000)
	display := metrics.NewDisplay(collector, r.cfg.Benchmark.SeedRecords*int64(r.cfg.Benchmark.Databases))

	collector.Start()
	display.Start()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		select {
		case <-sigCh:
			fmt.Println("\nReceived signal, stopping seed phase...")
			cancel()
		case <-ctx.Done():
		}
	}()

	var seedCounter atomic.Int64
	totalSeedTarget := r.cfg.Benchmark.SeedRecords * int64(r.cfg.Benchmark.Databases)

	err := r.runWorkers(ctx, cancel, collector, &seedCounter, totalSeedTarget, false)

	display.Stop()
	collector.Stop()

	if err != nil {
		return err
	}

	fmt.Printf("Seed phase complete: %d rows inserted.\n", seedCounter.Load())
	return nil
}

func (r *Runner) runBenchmark(sigCh chan os.Signal) error {
	b := &r.cfg.Benchmark
	fmt.Printf("\n=== Benchmark Phase: scenario=%s, method=%s ===\n", b.Scenario, b.LoadMethod)

	if b.ParsedWarmupDuration > 0 {
		fmt.Printf("Warming up for %s...\n", b.ParsedWarmupDuration)
		time.Sleep(b.ParsedWarmupDuration)
	}

	r.collector = metrics.NewCollector(10000)
	r.display = metrics.NewDisplay(r.collector, b.TotalRecords)

	r.collector.Start()
	r.display.Start()

	ctx, cancel := context.WithCancel(context.Background())
	if b.ParsedDuration > 0 {
		ctx, cancel = context.WithTimeout(context.Background(), b.ParsedDuration)
	}
	defer cancel()

	go func() {
		select {
		case <-sigCh:
			fmt.Println("\nReceived signal, stopping benchmark...")
			cancel()
		case <-ctx.Done():
		}
	}()

	r.recordCounter.Store(0)
	isUpdate := b.Scenario == "update"

	err := r.runWorkers(ctx, cancel, r.collector, &r.recordCounter, b.TotalRecords, isUpdate)

	r.display.Stop()
	r.collector.Stop()

	reportCfg := metrics.ReportConfig{
		Scenario:    b.Scenario,
		LoadMethod:  b.LoadMethod,
		Databases:   b.Databases,
		TablesPerDB: b.TablesPerDB,
		TableEngine: b.TableEngine,
		BatchSize:   b.BatchSize,
		Parallel:    b.ParallelPerDB,
		Duration:    b.Duration,
		Records:     b.TotalRecords,
	}
	metrics.GenerateReport(r.collector, reportCfg, b.ResultsFile)

	return err
}

func (r *Runner) runWorkers(
	ctx context.Context,
	cancel context.CancelFunc,
	collector *metrics.Collector,
	recordCounter *atomic.Int64,
	targetRecords int64,
	isUpdate bool,
) error {
	b := &r.cfg.Benchmark
	columns := r.cfg.ResolvedColumns()
	eventCh := collector.EventChan()

	totalWorkers := b.Databases * b.ParallelPerDB

	// Distribute parallel_per_db workers evenly across tables.
	// E.g. parallel_per_db=10, tables=3 -> tables get [4, 3, 3] workers
	workersPerTable := make([]int, b.TablesPerDB)
	for i := 0; i < b.ParallelPerDB; i++ {
		workersPerTable[i%b.TablesPerDB]++
	}

	fmt.Printf("  Concurrency: %d parallel requests/DB x %d DBs = %d total goroutines\n",
		b.ParallelPerDB, b.Databases, totalWorkers)
	fmt.Printf("  Per-table distribution: %v workers across %d tables (batch_size=%d)\n",
		workersPerTable, b.TablesPerDB, b.BatchSize)
	fmt.Printf("  Rows in-flight per DB: %d x %d = %d\n",
		b.ParallelPerDB, b.BatchSize, b.ParallelPerDB*b.BatchSize)

	g, gCtx := errgroup.WithContext(ctx)

	workerID := 0

	for dbIdx := 0; dbIdx < b.Databases; dbIdx++ {
		dbName := r.cfg.DatabaseName(dbIdx)

		var dbLoader loader.Loader
		var loaderErr error

		if b.LoadMethod == "sql" {
			dbLoader, loaderErr = loader.NewSQLLoader(r.cfg.StarRocks, dbName, b.ParallelPerDB)
			if loaderErr != nil {
				return fmt.Errorf("creating SQL loader for %s: %w", dbName, loaderErr)
			}
		} else {
			dbLoader = loader.NewStreamLoader(r.cfg.StarRocks, isUpdate, b.PartialUpdateMode)
		}

		dbG, _ := errgroup.WithContext(gCtx)

		for tblIdx := 0; tblIdx < b.TablesPerDB; tblIdx++ {
			tblName := r.cfg.TableName(tblIdx)
			tblSchema := generator.TableSchema{
				Database: dbName,
				Table:    tblName,
				Columns:  columns,
			}

			for w := 0; w < workersPerTable[tblIdx]; w++ {
				wID := workerID
				workerID++

				dbG.Go(func() error {
					gen := generator.NewGenerator(wID, totalWorkers)
					return workerLoop(gCtx, tblSchema, dbLoader, gen,
						eventCh, recordCounter, targetRecords, isUpdate, b.BatchSize, b.MaxRetries)
				})
			}
		}

		currentLoader := dbLoader
		g.Go(func() error {
			err := dbG.Wait()
			currentLoader.Close()
			return err
		})
	}

	err := g.Wait()
	if ctx.Err() != nil {
		return nil
	}
	return err
}

// workerLoop is the core loop for a single worker goroutine.
// Each worker = 1 goroutine = 1 concurrent batch request at a time.
// parallel_per_db workers per database means parallel_per_db batches in-flight.
func workerLoop(
	ctx context.Context,
	schema generator.TableSchema,
	ldr loader.Loader,
	gen *generator.Generator,
	eventCh chan<- metrics.Event,
	recordCounter *atomic.Int64,
	targetRecords int64,
	isUpdate bool,
	batchSize int,
	maxRetries int,
) error {
	for {
		if ctx.Err() != nil {
			return nil
		}

		if targetRecords > 0 {
			current := recordCounter.Load()
			if current >= targetRecords {
				return nil
			}
		}

		var batch *generator.Batch
		if isUpdate {
			maxID := recordCounter.Load()
			if maxID < 1 {
				maxID = 1
			}
			batch = gen.GenerateUpdateBatch(schema, batchSize, maxID)
		} else {
			batch = gen.GenerateBatch(schema, batchSize)
		}

		var (
			n       int
			err     error
			latency time.Duration
		)

		for attempt := 0; attempt <= maxRetries; attempt++ {
			start := time.Now()
			n, err = ldr.Load(ctx, schema, batch)
			latency = time.Since(start)

			if err == nil || ctx.Err() != nil {
				break
			}

			if attempt < maxRetries {
				backoff := time.Duration(1<<uint(attempt)) * 100 * time.Millisecond
				select {
				case <-ctx.Done():
					return nil
				case <-time.After(backoff):
				}
			}
		}

		if ctx.Err() != nil {
			return nil
		}

		if err != nil {
			select {
			case eventCh <- metrics.Event{
				Database: schema.Database,
				Table:    schema.Table,
				Latency:  latency,
				IsError:  true,
				ErrorMsg: err.Error(),
			}:
			default:
			}
			continue
		}

		if targetRecords > 0 {
			recordCounter.Add(int64(n))
		}

		select {
		case eventCh <- metrics.Event{
			Database: schema.Database,
			Table:    schema.Table,
			Rows:     n,
			Bytes:    batch.SizeBytes,
			Latency:  latency,
		}:
		default:
		}
	}
}

func (r *Runner) preflightCheck() error {
	b := &r.cfg.Benchmark
	dbName := r.cfg.DatabaseName(0)
	tblName := r.cfg.TableName(0)
	columns := r.cfg.ResolvedColumns()

	tblSchema := generator.TableSchema{
		Database: dbName,
		Table:    tblName,
		Columns:  columns,
	}

	gen := generator.NewGenerator(9999, 10000)
	batch := gen.GenerateBatch(tblSchema, 5)

	fmt.Printf("  Creating %s loader for %s...\n", b.LoadMethod, dbName)
	var ldr loader.Loader
	if b.LoadMethod == "sql" {
		sqlLdr, err := loader.NewSQLLoader(r.cfg.StarRocks, dbName, 1)
		if err != nil {
			return fmt.Errorf("creating SQL loader: %w", err)
		}
		defer sqlLdr.Close()
		ldr = sqlLdr
	} else {
		streamLdr := loader.NewStreamLoader(r.cfg.StarRocks, false, "")
		defer streamLdr.Close()
		ldr = streamLdr
	}

	fmt.Printf("  Sending test batch (5 rows) to %s.%s...\n", dbName, tblName)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	start := time.Now()
	n, err := ldr.Load(ctx, tblSchema, batch)
	elapsed := time.Since(start)

	if err != nil {
		return fmt.Errorf("test load to %s.%s failed (took %s): %w", dbName, tblName, elapsed, err)
	}

	fmt.Printf("  Test load successful: %d rows into %s.%s in %s\n", n, dbName, tblName, elapsed)
	return nil
}

func (r *Runner) cleanup() {
	r.cleanupOnce.Do(func() {
		if r.schemaMgr == nil {
			return
		}
		if !r.cfg.ShouldDeleteData() {
			fmt.Println("\n=== Cleanup skipped: delete_data is set to false, keeping benchmark databases ===")
			return
		}
		fmt.Println("\n=== Cleanup: Dropping benchmark databases ===")
		if err := r.schemaMgr.DropAll(); err != nil {
			fmt.Printf("Warning: cleanup failed: %v\n", err)
		} else {
			fmt.Println("Cleanup complete.")
		}
	})
}
