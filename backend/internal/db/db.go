package db

import (
	"database/sql"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	_ "github.com/lib/pq"
)

// containsIgnoreCase returns true if s contains substr (case-insensitive)
func containsIgnoreCase(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

// DB wraps the database connection
type DB struct {
	*sql.DB
}

// New creates a new database connection from the provided connection string
func New(connectionString string) (*DB, error) {
	if connectionString == "" {
		return nil, fmt.Errorf("database connection string is required")
	}

	sqlDB, err := sql.Open("postgres", connectionString)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := sqlDB.Ping(); err != nil {
		// Try with SSL disabled if connection fails and SSL mode not specified
		if connectionString != "" && !containsIgnoreCase(connectionString, "sslmode") {
			log.Println("retrying database connection with SSL disabled")
			sqlDB.Close()
			sslDisabledConnection := connectionString
			if strings.Contains(connectionString, "?") {
				sslDisabledConnection += "&sslmode=disable"
			} else {
				sslDisabledConnection += "?sslmode=disable"
			}
			var err2 error
			sqlDB, err2 = sql.Open("postgres", sslDisabledConnection)
			if err2 != nil {
				return nil, fmt.Errorf("failed to open database: %w", err2)
			}
		}
		if err := sqlDB.Ping(); err != nil {
			return nil, fmt.Errorf("failed to ping database: %w", err)
		}
	}

	// Set connection pool settings
	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(5)

	return &DB{DB: sqlDB}, nil
}

// HealthCheck verifies the database connection is healthy
func (db *DB) HealthCheck() error {
	return db.Ping()
}

// Close closes the database connection
func (db *DB) Close() error {
	return db.DB.Close()
}

// RunMigrations executes all SQL migration files in the migrations directory
func (db *DB) RunMigrations(migrationsDir string) error {
	migrations, err := readMigrations(migrationsDir)
	if err != nil {
		return fmt.Errorf("failed to read migrations: %w", err)
	}

	if len(migrations) == 0 {
		log.Println("no migrations found")
		return nil
	}

	// Ensure migration tracking table exists
	if err := db.createMigrationTable(); err != nil {
		return fmt.Errorf("failed to create migration table: %w", err)
	}

	for _, migration := range migrations {
		// Check if migration has already been applied
		applied, err := db.isMigrationApplied(migration.Number)
		if err != nil {
			return fmt.Errorf("failed to check migration status: %w", err)
		}

		if applied {
			log.Printf("migration %d already applied, skipping", migration.Number)
			continue
		}

		log.Printf("applying migration %d: %s", migration.Number, migration.Name)

		// Execute migration in a transaction
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("failed to begin transaction: %w", err)
		}

		if _, err := tx.Exec(migration.SQL); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to execute migration %d: %w", migration.Number, err)
		}

		// Record migration in tracking table
		if _, err := tx.Exec(
			"INSERT INTO schema_migrations (version, name) VALUES ($1, $2)",
			migration.Number,
			migration.Name,
		); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to record migration: %w", err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit migration: %w", err)
		}

		log.Printf("migration %d applied successfully", migration.Number)
	}

	return nil
}

// Migration represents a single migration file
type Migration struct {
	Number int
	Name   string
	SQL    string
}

// readMigrations reads all migration files from the migrations directory
func readMigrations(migrationsDir string) ([]Migration, error) {
	var migrations []Migration

	err := filepath.WalkDir(migrationsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() || !strings.HasSuffix(path, ".sql") {
			return nil
		}

		filename := d.Name()
		// Parse migration number from filename (e.g., "001_initial_schema.sql" -> 1)
		parts := strings.Split(filename, "_")
		if len(parts) < 2 {
			return nil
		}

		number, err := strconv.Atoi(parts[0])
		if err != nil {
			return nil
		}

		sqlBytes, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read migration file %s: %w", filename, err)
		}

		name := strings.TrimSuffix(strings.Join(parts[1:], "_"), ".sql")

		migrations = append(migrations, Migration{
			Number: number,
			Name:   name,
			SQL:    string(sqlBytes),
		})

		return nil
	})
	if err != nil {
		return nil, err
	}

	// Sort migrations by number
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Number < migrations[j].Number
	})

	return migrations, nil
}

// createMigrationTable creates the table that tracks which migrations have been applied
func (db *DB) createMigrationTable() error {
	createTableSQL := `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at TIMESTAMP DEFAULT NOW()
		)
	`
	_, err := db.Exec(createTableSQL)
	return err
}

// isMigrationApplied checks if a migration with the given number has been applied
func (db *DB) isMigrationApplied(number int) (bool, error) {
	var count int
	err := db.QueryRow(
		"SELECT COUNT(*) FROM schema_migrations WHERE version = $1",
		number,
	).Scan(&count)
	if err != nil {
		return false, err
	}

	return count > 0, nil
}
