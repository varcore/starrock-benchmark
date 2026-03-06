package metrics

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/jedib0t/go-pretty/v6/table"
)

type ReportConfig struct {
	Scenario       string
	LoadMethod     string
	Databases      int
	TablesPerDB    int
	TableEngine    string
	BatchSize      int
	Parallel       int
	MaxConnections int
	ConnsPerDB     int
	Duration       string
	Records        int64
}

type ReportResult struct {
	Config          ReportConfig     `json:"config"`
	Elapsed         string           `json:"elapsed"`
	TotalRows       int64            `json:"total_rows"`
	TotalBytesMB    float64          `json:"total_bytes_mb"`
	AvgRowsPerSec   int64            `json:"avg_rows_per_sec"`
	PeakRowsPerSec  int64            `json:"peak_rows_per_sec"`
	AvgMBPerSec     float64          `json:"avg_mb_per_sec"`
	TotalErrors     int64            `json:"total_errors"`
	AvgLatency      string           `json:"avg_latency"`
	P50Latency      string           `json:"p50_latency"`
	P95Latency      string           `json:"p95_latency"`
	P99Latency      string           `json:"p99_latency"`
	MinLatency      string           `json:"min_latency"`
	MaxLatency      string           `json:"max_latency"`
	DatabaseResults []DatabaseResult `json:"database_results"`
	TableResults    []TableResult    `json:"table_results"`
	RuntimeStats    RuntimeStats     `json:"runtime_stats"`
}

type DatabaseResult struct {
	Database     string  `json:"database"`
	TotalRows    int64   `json:"total_rows"`
	TotalBytesMB float64 `json:"total_bytes_mb"`
	ErrorCount   int64   `json:"error_count"`
}

type TableResult struct {
	Database     string  `json:"database"`
	Table        string  `json:"table"`
	TotalRows    int64   `json:"total_rows"`
	TotalBytesMB float64 `json:"total_bytes_mb"`
	ErrorCount   int64   `json:"error_count"`
	AvgLatency   string  `json:"avg_latency"`
	P50Latency   string  `json:"p50_latency"`
	P95Latency   string  `json:"p95_latency"`
	P99Latency   string  `json:"p99_latency"`
}

type RuntimeStats struct {
	Goroutines  int     `json:"goroutines"`
	HeapAllocMB float64 `json:"heap_alloc_mb"`
	SysMemMB    float64 `json:"sys_mem_mb"`
	NumGC       uint32  `json:"num_gc"`
}

func GenerateReport(c *Collector, cfg ReportConfig, outputFile string) {
	elapsed := c.Elapsed()
	totalRows := c.TotalRows()
	totalBytes := c.TotalBytes()

	allSnaps := c.AllTableSnapshots()
	var allLatencies []time.Duration
	for _, s := range allSnaps {
		allLatencies = append(allLatencies, s.Latencies...)
	}
	aggregated := StatsSnapshot{Latencies: allLatencies}

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	result := ReportResult{
		Config:         cfg,
		Elapsed:        formatDuration(elapsed),
		TotalRows:      totalRows,
		TotalBytesMB:   float64(totalBytes) / (1024 * 1024),
		AvgRowsPerSec:  c.CurrentRowsPerSec(),
		PeakRowsPerSec: c.PeakRowsPerSec(),
		AvgMBPerSec:    c.CurrentMBPerSec(),
		TotalErrors:    c.TotalErrors(),
		AvgLatency:     aggregated.AvgLatency().String(),
		P50Latency:     aggregated.LatencyPercentile(0.50).String(),
		P95Latency:     aggregated.LatencyPercentile(0.95).String(),
		P99Latency:     aggregated.LatencyPercentile(0.99).String(),
		MinLatency:     aggregated.MinLatency().String(),
		MaxLatency:     aggregated.MaxLatency().String(),
		RuntimeStats: RuntimeStats{
			Goroutines:  runtime.NumGoroutine(),
			HeapAllocMB: float64(memStats.HeapAlloc) / (1024 * 1024),
			SysMemMB:    float64(memStats.Sys) / (1024 * 1024),
			NumGC:       memStats.NumGC,
		},
	}

	dbSnaps := c.DatabaseSnapshots()
	for _, ds := range dbSnaps {
		result.DatabaseResults = append(result.DatabaseResults, DatabaseResult{
			Database:     ds.Database,
			TotalRows:    ds.TotalRows,
			TotalBytesMB: float64(ds.TotalBytes) / (1024 * 1024),
			ErrorCount:   ds.ErrorCount,
		})
	}

	for _, ts := range allSnaps {
		result.TableResults = append(result.TableResults, TableResult{
			Database:     ts.Database,
			Table:        ts.Table,
			TotalRows:    ts.TotalRows,
			TotalBytesMB: float64(ts.TotalBytes) / (1024 * 1024),
			ErrorCount:   ts.ErrorCount,
			AvgLatency:   ts.AvgLatency().String(),
			P50Latency:   ts.LatencyPercentile(0.50).String(),
			P95Latency:   ts.LatencyPercentile(0.95).String(),
			P99Latency:   ts.LatencyPercentile(0.99).String(),
		})
	}

	printReport(result)

	if outputFile != "" {
		writeJSON(result, outputFile)
	}
}

func printReport(r ReportResult) {
	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════════════════════════╗")
	fmt.Println("║                    BENCHMARK RESULTS                            ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════════╝")
	fmt.Println()

	cfgTable := table.NewWriter()
	cfgTable.SetOutputMirror(os.Stdout)
	cfgTable.SetTitle("Configuration")
	cfgTable.AppendHeader(table.Row{"Parameter", "Value"})
	cfgTable.AppendRows([]table.Row{
		{"Scenario", r.Config.Scenario},
		{"Load Method", r.Config.LoadMethod},
		{"Databases", r.Config.Databases},
		{"Tables/DB", r.Config.TablesPerDB},
		{"Table Engine", r.Config.TableEngine},
		{"Batch Size", r.Config.BatchSize},
		{"Parallel/DB", r.Config.Parallel},
		{"Max Connections", r.Config.MaxConnections},
		{"Connections/DB", r.Config.ConnsPerDB},
		{"Duration/Records", fmt.Sprintf("%s / %d", r.Config.Duration, r.Config.Records)},
	})
	cfgTable.Render()
	fmt.Println()

	summaryTable := table.NewWriter()
	summaryTable.SetOutputMirror(os.Stdout)
	summaryTable.SetTitle("Summary")
	summaryTable.AppendHeader(table.Row{"Metric", "Value"})
	summaryTable.AppendRows([]table.Row{
		{"Elapsed", r.Elapsed},
		{"Total Rows", formatNumber(r.TotalRows)},
		{"Total Data", fmt.Sprintf("%.2f MB", r.TotalBytesMB)},
		{"Avg Rows/sec", formatNumber(r.AvgRowsPerSec)},
		{"Peak Rows/sec", formatNumber(r.PeakRowsPerSec)},
		{"Avg MB/sec", fmt.Sprintf("%.2f", r.AvgMBPerSec)},
		{"Total Errors", r.TotalErrors},
	})
	summaryTable.Render()
	fmt.Println()

	latencyTable := table.NewWriter()
	latencyTable.SetOutputMirror(os.Stdout)
	latencyTable.SetTitle("Latency")
	latencyTable.AppendHeader(table.Row{"Percentile", "Value"})
	latencyTable.AppendRows([]table.Row{
		{"Min", r.MinLatency},
		{"Avg", r.AvgLatency},
		{"P50", r.P50Latency},
		{"P95", r.P95Latency},
		{"P99", r.P99Latency},
		{"Max", r.MaxLatency},
	})
	latencyTable.Render()
	fmt.Println()

	if len(r.DatabaseResults) > 0 {
		dbTable := table.NewWriter()
		dbTable.SetOutputMirror(os.Stdout)
		dbTable.SetTitle("Per-Database Results")
		dbTable.AppendHeader(table.Row{"Database", "Rows", "Data (MB)", "Errors"})
		for _, dr := range r.DatabaseResults {
			dbTable.AppendRow(table.Row{dr.Database, formatNumber(dr.TotalRows), fmt.Sprintf("%.2f", dr.TotalBytesMB), dr.ErrorCount})
		}
		dbTable.Render()
		fmt.Println()
	}

	if len(r.TableResults) > 0 {
		tblTable := table.NewWriter()
		tblTable.SetOutputMirror(os.Stdout)
		tblTable.SetTitle("Per-Table Results")
		tblTable.AppendHeader(table.Row{"Database", "Table", "Rows", "Data (MB)", "Errors", "Avg Lat", "P50", "P95", "P99"})
		for _, tr := range r.TableResults {
			tblTable.AppendRow(table.Row{
				tr.Database, tr.Table,
				formatNumber(tr.TotalRows), fmt.Sprintf("%.2f", tr.TotalBytesMB),
				tr.ErrorCount, tr.AvgLatency, tr.P50Latency, tr.P95Latency, tr.P99Latency,
			})
		}
		tblTable.Render()
		fmt.Println()
	}

	runtimeTable := table.NewWriter()
	runtimeTable.SetOutputMirror(os.Stdout)
	runtimeTable.SetTitle("Runtime Stats")
	runtimeTable.AppendHeader(table.Row{"Metric", "Value"})
	runtimeTable.AppendRows([]table.Row{
		{"Goroutines", r.RuntimeStats.Goroutines},
		{"Heap Alloc", fmt.Sprintf("%.2f MB", r.RuntimeStats.HeapAllocMB)},
		{"Sys Memory", fmt.Sprintf("%.2f MB", r.RuntimeStats.SysMemMB)},
		{"GC Runs", r.RuntimeStats.NumGC},
	})
	runtimeTable.Render()
	fmt.Println()
}

func writeJSON(r ReportResult, path string) {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		fmt.Printf("Warning: failed to marshal results to JSON: %v\n", err)
		return
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		fmt.Printf("Warning: failed to write results to %s: %v\n", path, err)
		return
	}
	fmt.Printf("Results written to: %s\n", path)
}
