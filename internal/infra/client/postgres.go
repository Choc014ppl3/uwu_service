package client

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresClient wraps the pgxpool.Pool.
type PostgresClient struct {
	Pool *pgxpool.Pool
}

// NewPostgresClient creates a new PostgreSQL client.
func NewPostgresClient(ctx context.Context, connectionString string) (*PostgresClient, error) {
	config, err := pgxpool.ParseConfig(connectionString)
	if err != nil {
		return nil, fmt.Errorf("failed to parse postgres config: %w", err)
	}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to postgres: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping postgres: %w", err)
	}

	return &PostgresClient{Pool: pool}, nil
}

// Close closes the database connection pool.
func (c *PostgresClient) Close() {
	c.Pool.Close()
}
