package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

func MustOpen(ctx context.Context, dsn string) *pgxpool.Pool {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatal().Err(err).Msg("db connect fail")
	}
	if err := pool.Ping(ctx); err != nil {
		log.Fatal().Err(err).Msg("db ping fail")
	}
	return pool
}
