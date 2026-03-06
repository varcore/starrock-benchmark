package metrics

import (
	"fmt"
	"strings"
	"sync/atomic"
	"time"
)

type Display struct {
	collector    *Collector
	ticker       *time.Ticker
	done         chan struct{}
	linesWritten atomic.Int32
	totalRecords int64
}

func NewDisplay(collector *Collector, totalRecords int64) *Display {
	return &Display{
		collector:    collector,
		done:         make(chan struct{}),
		totalRecords: totalRecords,
	}
}

func (d *Display) Start() {
	d.ticker = time.NewTicker(1 * time.Second)
	go d.renderLoop()
}

func (d *Display) Stop() {
	d.ticker.Stop()
	close(d.done)
	d.render()
	fmt.Println()
}

func (d *Display) renderLoop() {
	for {
		select {
		case <-d.done:
			return
		case <-d.ticker.C:
			d.render()
		}
	}
}

func (d *Display) render() {
	lines := d.linesWritten.Load()
	if lines > 0 {
		fmt.Printf("\033[%dA", lines)
	}

	var sb strings.Builder
	elapsed := d.collector.Elapsed()
	totalRows := d.collector.TotalRows()
	totalBytes := d.collector.TotalBytes()
	totalErrs := d.collector.TotalErrors()
	rps := d.collector.CurrentRowsPerSec()
	mbps := d.collector.CurrentMBPerSec()
	peakRPS := d.collector.PeakRowsPerSec()

	header := fmt.Sprintf(
		"  Elapsed: %-10s | Rows: %-12s | Rows/s: %-10s | MB/s: %-8.2f | Peak Rows/s: %-10s | Errors: %d",
		formatDuration(elapsed), formatNumber(totalRows), formatNumber(rps),
		mbps, formatNumber(peakRPS), totalErrs,
	)

	sb.WriteString("\033[2K")
	sb.WriteString(strings.Repeat("=", min(len(header)+2, 140)))
	sb.WriteByte('\n')

	sb.WriteString("\033[2K")
	sb.WriteString(header)
	sb.WriteByte('\n')

	if d.totalRecords > 0 {
		pct := float64(totalRows) / float64(d.totalRecords) * 100
		if pct > 100 {
			pct = 100
		}
		barWidth := 50
		filled := int(pct / 100 * float64(barWidth))
		bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)

		var eta string
		if rps > 0 {
			remaining := d.totalRecords - totalRows
			if remaining > 0 {
				etaDur := time.Duration(float64(remaining) / float64(rps) * float64(time.Second))
				eta = formatDuration(etaDur)
			} else {
				eta = "done"
			}
		} else {
			eta = "calculating..."
		}

		sb.WriteString("\033[2K")
		sb.WriteString(fmt.Sprintf("  Progress: [%s] %.1f%%  ETA: %s", bar, pct, eta))
		sb.WriteByte('\n')
	}

	sb.WriteString("\033[2K")
	sb.WriteString(strings.Repeat("-", min(len(header)+2, 140)))
	sb.WriteByte('\n')

	dbSnaps := d.collector.DatabaseSnapshots()
	for _, ds := range dbSnaps {
		elapsedSec := elapsed.Seconds()
		dbRPS := int64(0)
		if elapsedSec > 0 {
			dbRPS = int64(float64(ds.TotalRows) / elapsedSec)
		}
		dbMBPS := float64(0)
		if elapsedSec > 0 {
			dbMBPS = float64(ds.TotalBytes) / (1024 * 1024) / elapsedSec
		}
		sb.WriteString("\033[2K")
		sb.WriteString(fmt.Sprintf("  %-20s | Rows: %-12s | Rows/s: %-10s | MB/s: %-8.2f | Batches: %-8d | Errors: %d",
			ds.Database, formatNumber(ds.TotalRows), formatNumber(dbRPS),
			dbMBPS, ds.TotalBatches, ds.ErrorCount))
		sb.WriteByte('\n')
	}

	sb.WriteString("\033[2K")
	sb.WriteString(strings.Repeat("=", min(len(header)+2, 140)))
	sb.WriteByte('\n')

	totalBytes_ := float64(totalBytes) / (1024 * 1024)
	sb.WriteString("\033[2K")
	sb.WriteString(fmt.Sprintf("  TOTAL: %s rows | %.2f MB", formatNumber(totalRows), totalBytes_))
	sb.WriteByte('\n')

	output := sb.String()
	fmt.Print(output)

	lineCount := int32(strings.Count(output, "\n"))
	d.linesWritten.Store(lineCount)
}

func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh%02dm%02ds", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%dm%02ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

func formatNumber(n int64) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	if n < 1_000_000 {
		return fmt.Sprintf("%.1fK", float64(n)/1000)
	}
	if n < 1_000_000_000 {
		return fmt.Sprintf("%.2fM", float64(n)/1_000_000)
	}
	return fmt.Sprintf("%.2fB", float64(n)/1_000_000_000)
}
