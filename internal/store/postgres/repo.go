package postgres

import (
	"paymatch/internal/config"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Repo struct {
	db  *pgxpool.Pool
	cfg config.Cfg
}

func NewRepo(db *pgxpool.Pool, cfg config.Cfg) *Repo { return &Repo{db: db, cfg: cfg} }

// Expose the underlying pool for read-only helpers (e.g., replayByWindow).
func (r *Repo) DB() *pgxpool.Pool { return r.db }
