package generator

import (
	"fmt"
	"math/rand/v2"
	"strings"
	"sync/atomic"
	"time"

	"starrock-benchmark/internal/config"
)

const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

type TableSchema struct {
	Database string
	Table    string
	Columns  []config.ColumnDef
}

type Batch struct {
	Rows      [][]interface{}
	SizeBytes int64
}

type Generator struct {
	workerID     int
	totalWorkers int
	idCounter    atomic.Int64
	rng          *rand.Rand
}

func NewGenerator(workerID int, totalWorkers int) *Generator {
	return &Generator{
		workerID:     workerID,
		totalWorkers: totalWorkers,
		rng:          rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), uint64(workerID))),
	}
}

func (g *Generator) nextID() int64 {
	seq := g.idCounter.Add(1)
	return int64(g.workerID)*1_000_000_000 + seq
}

func (g *Generator) GenerateBatch(schema TableSchema, batchSize int) *Batch {
	batch := &Batch{
		Rows: make([][]interface{}, 0, batchSize),
	}

	for i := 0; i < batchSize; i++ {
		id := g.nextID()
		createdAt := g.randomCreatedAt()

		row := make([]interface{}, 0, 2+len(schema.Columns))
		row = append(row, id, createdAt)

		var rowSize int64 = 8 + 19 // id (bigint) + created_at (datetime)

		for _, col := range schema.Columns {
			val, size := g.generateValue(col.Type)
			row = append(row, val)
			rowSize += size
		}

		batch.Rows = append(batch.Rows, row)
		batch.SizeBytes += rowSize
	}

	return batch
}

func (g *Generator) GenerateUpdateBatch(schema TableSchema, batchSize int, maxID int64) *Batch {
	batch := &Batch{
		Rows: make([][]interface{}, 0, batchSize),
	}

	for i := 0; i < batchSize; i++ {
		id := g.rng.Int64N(maxID) + 1
		createdAt := g.randomCreatedAt()

		row := make([]interface{}, 0, 2+len(schema.Columns))
		row = append(row, id, createdAt)

		var rowSize int64 = 8 + 19

		for _, col := range schema.Columns {
			val, size := g.generateValue(col.Type)
			row = append(row, val)
			rowSize += size
		}

		batch.Rows = append(batch.Rows, row)
		batch.SizeBytes += rowSize
	}

	return batch
}

func (g *Generator) generateValue(colType string) (interface{}, int64) {
	upper := strings.ToUpper(colType)

	switch {
	case upper == "INT":
		return g.rng.IntN(1_000_001), int64(8)
	case upper == "BIGINT":
		return g.rng.Int64N(1_000_000_001), int64(8)
	case strings.HasPrefix(upper, "VARCHAR"):
		s := g.randomString(10 + g.rng.IntN(41))
		return s, int64(len(s))
	case upper == "DOUBLE":
		return g.rng.Float64() * 100_000, int64(8)
	case upper == "DATETIME":
		return g.randomDatetimeLastYear(), int64(19)
	default:
		return nil, 0
	}
}

func (g *Generator) randomString(length int) string {
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[g.rng.IntN(len(charset))]
	}
	return string(b)
}

func (g *Generator) randomCreatedAt() string {
	now := time.Now()
	start := now.AddDate(0, -6, 0)
	end := now.AddDate(0, 6, 0)
	delta := end.Sub(start)
	offset := time.Duration(g.rng.Int64N(int64(delta)))
	return start.Add(offset).Format("2006-01-02 15:04:05")
}

func (g *Generator) randomDatetimeLastYear() string {
	now := time.Now()
	start := now.AddDate(-1, 0, 0)
	delta := now.Sub(start)
	offset := time.Duration(g.rng.Int64N(int64(delta)))
	return start.Add(offset).Format("2006-01-02 15:04:05")
}

func FormatRow(row []interface{}) string {
	parts := make([]string, len(row))
	for i, v := range row {
		switch val := v.(type) {
		case string:
			parts[i] = fmt.Sprintf("'%s'", val)
		default:
			parts[i] = fmt.Sprintf("%v", val)
		}
	}
	return "(" + strings.Join(parts, ", ") + ")"
}
