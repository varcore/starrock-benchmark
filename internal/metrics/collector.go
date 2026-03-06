package metrics

import (
	"fmt"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

type Event struct {
	Database string
	Table    string
	Rows     int
	Bytes    int64
	Latency  time.Duration
	IsError  bool
	ErrorMsg string
}

type TableStats struct {
	Database     string
	Table        string
	TotalRows    atomic.Int64
	TotalBytes   atomic.Int64
	TotalBatches atomic.Int64
	ErrorCount   atomic.Int64

	mu        sync.Mutex
	latencies []time.Duration
}

func (ts *TableStats) RecordSuccess(rows int, bytes int64, latency time.Duration) {
	ts.TotalRows.Add(int64(rows))
	ts.TotalBytes.Add(bytes)
	ts.TotalBatches.Add(1)

	ts.mu.Lock()
	ts.latencies = append(ts.latencies, latency)
	ts.mu.Unlock()
}

func (ts *TableStats) RecordError(latency time.Duration) {
	ts.ErrorCount.Add(1)
	ts.TotalBatches.Add(1)

	ts.mu.Lock()
	ts.latencies = append(ts.latencies, latency)
	ts.mu.Unlock()
}

func (ts *TableStats) Snapshot() StatsSnapshot {
	ts.mu.Lock()
	lats := make([]time.Duration, len(ts.latencies))
	copy(lats, ts.latencies)
	ts.mu.Unlock()

	return StatsSnapshot{
		Database:     ts.Database,
		Table:        ts.Table,
		TotalRows:    ts.TotalRows.Load(),
		TotalBytes:   ts.TotalBytes.Load(),
		TotalBatches: ts.TotalBatches.Load(),
		ErrorCount:   ts.ErrorCount.Load(),
		Latencies:    lats,
	}
}

type StatsSnapshot struct {
	Database     string
	Table        string
	TotalRows    int64
	TotalBytes   int64
	TotalBatches int64
	ErrorCount   int64
	Latencies    []time.Duration
}

func (s StatsSnapshot) LatencyPercentile(p float64) time.Duration {
	if len(s.Latencies) == 0 {
		return 0
	}
	sorted := make([]time.Duration, len(s.Latencies))
	copy(sorted, s.Latencies)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	idx := int(float64(len(sorted)-1) * p)
	return sorted[idx]
}

func (s StatsSnapshot) AvgLatency() time.Duration {
	if len(s.Latencies) == 0 {
		return 0
	}
	var total time.Duration
	for _, l := range s.Latencies {
		total += l
	}
	return total / time.Duration(len(s.Latencies))
}

func (s StatsSnapshot) MinLatency() time.Duration {
	if len(s.Latencies) == 0 {
		return 0
	}
	min := s.Latencies[0]
	for _, l := range s.Latencies[1:] {
		if l < min {
			min = l
		}
	}
	return min
}

func (s StatsSnapshot) MaxLatency() time.Duration {
	if len(s.Latencies) == 0 {
		return 0
	}
	max := s.Latencies[0]
	for _, l := range s.Latencies[1:] {
		if l > max {
			max = l
		}
	}
	return max
}

type Collector struct {
	eventCh   chan Event
	tables    sync.Map // key: "db.table" -> *TableStats
	startTime time.Time
	done      chan struct{}

	totalRows  atomic.Int64
	totalBytes atomic.Int64
	totalErrs  atomic.Int64

	peakRowsPerSec atomic.Int64
	lastSnapRows   atomic.Int64

	errorLogMu    sync.Mutex
	loggedErrors  int
	maxLogErrors  int
	uniqueErrors  map[string]bool
}

func NewCollector(bufferSize int) *Collector {
	return &Collector{
		eventCh:      make(chan Event, bufferSize),
		done:         make(chan struct{}),
		maxLogErrors: 10,
		uniqueErrors: make(map[string]bool),
	}
}

func (c *Collector) EventChan() chan<- Event {
	return c.eventCh
}

func (c *Collector) Start() {
	c.startTime = time.Now()
	go c.collectLoop()
	go c.peakTracker()
}

func (c *Collector) Stop() {
	close(c.eventCh)
	<-c.done
}

func (c *Collector) collectLoop() {
	for ev := range c.eventCh {
		key := ev.Database + "." + ev.Table
		val, _ := c.tables.LoadOrStore(key, &TableStats{
			Database: ev.Database,
			Table:    ev.Table,
		})
		ts := val.(*TableStats)

		if ev.IsError {
			ts.RecordError(ev.Latency)
			c.totalErrs.Add(1)
			c.logError(ev)
		} else {
			ts.RecordSuccess(ev.Rows, ev.Bytes, ev.Latency)
			c.totalRows.Add(int64(ev.Rows))
			c.totalBytes.Add(ev.Bytes)
		}
	}
	close(c.done)
}

func (c *Collector) logError(ev Event) {
	if ev.ErrorMsg == "" {
		return
	}
	c.errorLogMu.Lock()
	defer c.errorLogMu.Unlock()

	if c.loggedErrors >= c.maxLogErrors {
		return
	}
	if c.uniqueErrors[ev.ErrorMsg] {
		return
	}
	c.uniqueErrors[ev.ErrorMsg] = true
	c.loggedErrors++

	fmt.Fprintf(os.Stderr, "\n[ERROR %d/%d] %s.%s: %s\n",
		c.loggedErrors, c.maxLogErrors, ev.Database, ev.Table, ev.ErrorMsg)
}

func (c *Collector) peakTracker() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-c.done:
			return
		case <-ticker.C:
			current := c.totalRows.Load()
			last := c.lastSnapRows.Swap(current)
			rps := current - last
			for {
				peak := c.peakRowsPerSec.Load()
				if rps <= peak {
					break
				}
				if c.peakRowsPerSec.CompareAndSwap(peak, rps) {
					break
				}
			}
		}
	}
}

func (c *Collector) Elapsed() time.Duration {
	return time.Since(c.startTime)
}

func (c *Collector) TotalRows() int64 {
	return c.totalRows.Load()
}

func (c *Collector) TotalBytes() int64 {
	return c.totalBytes.Load()
}

func (c *Collector) TotalErrors() int64 {
	return c.totalErrs.Load()
}

func (c *Collector) PeakRowsPerSec() int64 {
	return c.peakRowsPerSec.Load()
}

func (c *Collector) CurrentRowsPerSec() int64 {
	elapsed := c.Elapsed().Seconds()
	if elapsed < 1 {
		return 0
	}
	return int64(float64(c.totalRows.Load()) / elapsed)
}

func (c *Collector) CurrentMBPerSec() float64 {
	elapsed := c.Elapsed().Seconds()
	if elapsed < 1 {
		return 0
	}
	return float64(c.totalBytes.Load()) / (1024 * 1024) / elapsed
}

func (c *Collector) AllTableSnapshots() []StatsSnapshot {
	var snapshots []StatsSnapshot
	c.tables.Range(func(key, value any) bool {
		ts := value.(*TableStats)
		snapshots = append(snapshots, ts.Snapshot())
		return true
	})
	sort.Slice(snapshots, func(i, j int) bool {
		if snapshots[i].Database != snapshots[j].Database {
			return snapshots[i].Database < snapshots[j].Database
		}
		return snapshots[i].Table < snapshots[j].Table
	})
	return snapshots
}

type DatabaseSnapshot struct {
	Database     string
	TotalRows    int64
	TotalBytes   int64
	TotalBatches int64
	ErrorCount   int64
}

func (c *Collector) DatabaseSnapshots() []DatabaseSnapshot {
	dbMap := make(map[string]*DatabaseSnapshot)
	c.tables.Range(func(key, value any) bool {
		ts := value.(*TableStats)
		db := ts.Database
		ds, ok := dbMap[db]
		if !ok {
			ds = &DatabaseSnapshot{Database: db}
			dbMap[db] = ds
		}
		ds.TotalRows += ts.TotalRows.Load()
		ds.TotalBytes += ts.TotalBytes.Load()
		ds.TotalBatches += ts.TotalBatches.Load()
		ds.ErrorCount += ts.ErrorCount.Load()
		return true
	})

	result := make([]DatabaseSnapshot, 0, len(dbMap))
	for _, ds := range dbMap {
		result = append(result, *ds)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Database < result[j].Database
	})
	return result
}

func (c *Collector) Reset() {
	c.tables.Range(func(key, value any) bool {
		c.tables.Delete(key)
		return true
	})
	c.totalRows.Store(0)
	c.totalBytes.Store(0)
	c.totalErrs.Store(0)
	c.peakRowsPerSec.Store(0)
	c.lastSnapRows.Store(0)
	c.startTime = time.Now()
}
