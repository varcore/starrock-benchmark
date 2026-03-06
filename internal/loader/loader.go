package loader

import (
	"context"
	"starrock-benchmark/internal/generator"
)

type Loader interface {
	Load(ctx context.Context, schema generator.TableSchema, batch *generator.Batch) (rowsLoaded int, err error)
	Close() error
}
