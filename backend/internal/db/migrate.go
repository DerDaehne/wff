package db

import (
	"embed"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	pgx5migrate "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
)

// Migrate applies all pending migrations from fsys (an embed.FS containing a
// "migrations" directory) to pool. Safe to call on every startup — a no-op
// once the schema is already current.
func Migrate(pool *pgxpool.Pool, fsys embed.FS) error {
	sqlDB := stdlib.OpenDBFromPool(pool)
	defer sqlDB.Close() // does not close pool, see jackc/pgx/v5/stdlib docs

	driver, err := pgx5migrate.WithInstance(sqlDB, &pgx5migrate.Config{})
	if err != nil {
		return fmt.Errorf("migrate driver: %w", err)
	}

	src, err := iofs.New(fsys, "migrations")
	if err != nil {
		return fmt.Errorf("migrate source: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", src, "pgx5", driver)
	if err != nil {
		return fmt.Errorf("migrate init: %w", err)
	}

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrate up: %w", err)
	}
	return nil
}

