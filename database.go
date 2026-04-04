package main

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/lib/pq"
)

func connectProjectDatabase(envPath string, databaseName string) (*sql.DB, error) {
	err := loadEnvFile(envPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load .env file: %w", err)
	}

	host := os.Getenv("PG_HOST")
	port := os.Getenv("PG_PORT")
	user := os.Getenv("PG_USER")
	password := os.Getenv("PG_PASSWORD")
	sslmode := os.Getenv("PG_SSLMODE")

	if host == "" {
		return nil, fmt.Errorf("missing PG_HOST")
	}
	if port == "" {
		return nil, fmt.Errorf("missing PG_PORT")
	}
	if user == "" {
		return nil, fmt.Errorf("missing PG_USER")
	}
	if sslmode == "" {
		return nil, fmt.Errorf("missing PG_SSLMODE")
	}

	connStr := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		host,
		port,
		user,
		password,
		databaseName,
		sslmode,
	)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open PostgreSQL connection: %w", err)
	}

	err = db.Ping()
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping PostgreSQL database %s: %w", databaseName, err)
	}

	return db, nil
}