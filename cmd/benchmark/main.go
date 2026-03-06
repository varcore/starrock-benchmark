package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"starrock-benchmark/internal/config"
	"starrock-benchmark/internal/runner"
)

var (
	version   = "dev"
	commit    = "unknown"
	buildTime = "unknown"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to configuration file")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("starrock-benchmark %s (commit: %s, built: %s)\n", version, commit, buildTime)
		os.Exit(0)
	}

	fmt.Printf("╔══════════════════════════════════════════════════════════════════╗\n")
	fmt.Printf("║         StarRocks Benchmark Tool  v%-28s║\n", version)
	fmt.Printf("╚══════════════════════════════════════════════════════════════════╝\n")
	fmt.Println()

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	printConfig(cfg)

	runtime.GOMAXPROCS(runtime.NumCPU())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		fmt.Fprintf(os.Stderr, "\nReceived %v, shutting down gracefully... (press Ctrl+C again to force exit)\n", sig)
		cancel()

		<-sigCh
		fmt.Fprintf(os.Stderr, "\nForced exit.\n")
		os.Exit(1)
	}()

	r := runner.New(cfg)
	if err := r.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "\nBenchmark failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("\nBenchmark completed successfully.")
}

func printConfig(cfg *config.Config) {
	b := &cfg.Benchmark
	fmt.Printf("Configuration:\n")
	fmt.Printf("  StarRocks:     %s:%d (HTTP: %d)\n", cfg.StarRocks.Host, cfg.StarRocks.MySQLPort, cfg.StarRocks.HTTPPort)
	fmt.Printf("  Scenario:      %s\n", b.Scenario)
	fmt.Printf("  Load Method:   %s\n", b.LoadMethod)
	fmt.Printf("  Databases:     %d\n", b.Databases)
	fmt.Printf("  Tables/DB:     %d\n", b.TablesPerDB)
	fmt.Printf("  Table Engine:  %s\n", b.TableEngine)
	fmt.Printf("  PK Type:       %s\n", b.PrimaryKeyType)
	fmt.Printf("  Parallel/DB:   %d (= %d batches x %d rows = %d rows in-flight/DB)\n",
		b.ParallelPerDB, b.ParallelPerDB, b.BatchSize, b.ParallelPerDB*b.BatchSize)
	fmt.Printf("  Batch Size:    %d\n", b.BatchSize)
	fmt.Printf("  Buckets:       %d\n", b.BucketCount)

	if b.Duration != "" {
		fmt.Printf("  Duration:      %s\n", b.Duration)
	} else {
		fmt.Printf("  Total Records: %d\n", b.TotalRecords)
	}

	cols := cfg.ResolvedColumns()
	fmt.Printf("  Columns:       %d (+ id, created_at)\n", len(cols))

	if b.Scenario == "update" {
		fmt.Printf("  Seed Records:  %d\n", b.SeedRecords)
		fmt.Printf("  Update Mode:   %s\n", b.PartialUpdateMode)
	}

	fmt.Printf("  Warmup:        %s\n", b.ParsedWarmupDuration)
	fmt.Printf("  Max Retries:   %d\n", b.MaxRetries)
	fmt.Printf("  Use Existing:  %v\n", b.UseExisting)
	fmt.Printf("  Delete Data:   %v\n", cfg.ShouldDeleteData())

	if b.ResultsFile != "" {
		fmt.Printf("  Results File:  %s\n", b.ResultsFile)
	}
	fmt.Println()
}
