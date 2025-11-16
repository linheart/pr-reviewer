package migrations

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed init.sql
var initSQL string

func Run(ctx context.Context, pool *pgxpool.Pool) error {
	if initSQL == "" {
		return fmt.Errorf("init migration is empty")
	}

	if _, err := pool.Exec(ctx, initSQL); err != nil {
		return fmt.Errorf("exec init migration: %w", err)
	}

	return nil
}
