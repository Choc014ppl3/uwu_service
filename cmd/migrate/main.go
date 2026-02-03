package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

func main() {
	var (
		direction string
		steps     int
		dbURL     string
		path      string
	)

	flag.StringVar(&direction, "direction", "up", "Migration direction: up, down, or force")
	flag.IntVar(&steps, "steps", 0, "Number of migrations to run (0 = all)")
	flag.StringVar(&dbURL, "db", "", "Database URL (or set DATABASE_URL env var)")
	flag.StringVar(&path, "path", "migrations", "Path to migration files")
	flag.Parse()

	// Get database URL from flag or environment
	if dbURL == "" {
		dbURL = os.Getenv("DATABASE_URL")
	}
	if dbURL == "" {
		log.Fatal("Database URL is required. Set -db flag or DATABASE_URL env var")
	}

	// Create migrate instance
	m, err := migrate.New(
		fmt.Sprintf("file://%s", path),
		dbURL,
	)
	if err != nil {
		log.Fatalf("Failed to create migrate instance: %v", err)
	}
	defer m.Close()

	// Run migration based on direction
	switch direction {
	case "up":
		if steps > 0 {
			err = m.Steps(steps)
		} else {
			err = m.Up()
		}
	case "down":
		if steps > 0 {
			err = m.Steps(-steps)
		} else {
			err = m.Down()
		}
	case "force":
		// Force a specific version (useful for fixing dirty state)
		if steps == 0 {
			log.Fatal("Force requires -steps to specify version")
		}
		err = m.Force(steps)
	default:
		log.Fatalf("Unknown direction: %s (use up, down, or force)", direction)
	}

	if err != nil && err != migrate.ErrNoChange {
		log.Fatalf("Migration failed: %v", err)
	}

	version, dirty, _ := m.Version()
	if err == migrate.ErrNoChange {
		fmt.Printf("No migrations to apply. Current version: %d\n", version)
	} else {
		fmt.Printf("Migration successful! Version: %d, Dirty: %v\n", version, dirty)
	}
}
