package readmodel

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

type Reader struct {
	Pool *pgxpool.Pool
}

func (r Reader) queries() *gen.Queries { return gen.New(r.Pool) }
