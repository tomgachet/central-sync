package main

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/lib/pq"
)

const datasetSchema = "central_datasets"

func connectProjectDatabase(databaseName string) (*sql.DB, error) {
	host, err := getRequiredEnv("PG_HOST")
	if err != nil {
		return nil, err
	}

	port, err := getRequiredEnv("PG_PORT")
	if err != nil {
		return nil, err
	}

	user, err := getRequiredEnv("PG_USER")
	if err != nil {
		return nil, err
	}

	password := os.Getenv("PG_PASSWORD")

	sslmode, err := getRequiredEnv("PG_SSLMODE")
	if err != nil {
		return nil, err
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

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping PostgreSQL database %s: %w", databaseName, err)
	}

	return db, nil
}

func schemaExists(db *sql.DB, schemaName string) (bool, error) {
	const query = `
		SELECT EXISTS (
			SELECT 1
			FROM information_schema.schemata
			WHERE schema_name = $1
		)
	`

	var exists bool
	err := db.QueryRow(query, schemaName).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check schema existence: %w", err)
	}

	return exists, nil
}

func requireSchema(db *sql.DB, schemaName string) error {
	exists, err := schemaExists(db, schemaName)
	if err != nil {
		return err
	}

	if !exists {
		return fmt.Errorf("required schema %q does not exist", schemaName)
	}

	return nil
}