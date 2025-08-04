//go:build test
// +build test

package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	_ "github.com/lib/pq"
)

func main() {
	// Get database configuration from environment variables
	host := getEnv("SCANORAMA_DB_HOST", "localhost")
	port := getEnv("SCANORAMA_DB_PORT", "5432")
	dbname := getEnv("SCANORAMA_DB_NAME", "scanorama")
	user := getEnv("SCANORAMA_DB_USER", "scanorama")
	password := getEnv("SCANORAMA_DB_PASSWORD", "")
	sslmode := getEnv("SCANORAMA_DB_SSLMODE", "disable")

	// Build connection string
	connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		host, port, user, password, dbname, sslmode)

	fmt.Printf("Testing database connection to %s:%s/%s as %s...\n", host, port, dbname, user)

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Open database connection
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Fatalf("Failed to open database connection: %v", err)
	}
	defer db.Close()

	// Test the connection
	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("Failed to ping database: %v", err)
	}

	fmt.Println("✓ Database connection successful")

	// Test basic query
	var version string
	err = db.QueryRowContext(ctx, "SELECT version()").Scan(&version)
	if err != nil {
		log.Fatalf("Failed to query database version: %v", err)
	}

	fmt.Printf("✓ Database version: %s\n", version)

	// Test table existence (if migrations have run)
	var tableCount int
	query := `SELECT COUNT(*) FROM information_schema.tables
			  WHERE table_schema = 'public' AND table_name IN
			  ('scan_targets', 'hosts', 'scan_jobs', 'port_scans')`

	err = db.QueryRowContext(ctx, query).Scan(&tableCount)
	if err != nil {
		log.Printf("Warning: Failed to check table existence: %v", err)
	} else {
		fmt.Printf("✓ Found %d core tables in database\n", tableCount)
	}

	// Test write operation (create a test table and drop it)
	testTableName := fmt.Sprintf("test_connection_%d", time.Now().Unix())

	createQuery := fmt.Sprintf("CREATE TEMPORARY TABLE %s (id SERIAL PRIMARY KEY, test_data TEXT)", testTableName)
	_, err = db.ExecContext(ctx, createQuery)
	if err != nil {
		log.Fatalf("Failed to create test table: %v", err)
	}

	fmt.Println("✓ Database write permissions verified")

	// Test insert operation
	insertQuery := fmt.Sprintf("INSERT INTO %s (test_data) VALUES ($1)", testTableName)
	_, err = db.ExecContext(ctx, insertQuery, "test connection data")
	if err != nil {
		log.Fatalf("Failed to insert test data: %v", err)
	}

	fmt.Println("✓ Database insert operation verified")

	// Test read operation
	var testData string
	selectQuery := fmt.Sprintf("SELECT test_data FROM %s LIMIT 1", testTableName)
	err = db.QueryRowContext(ctx, selectQuery).Scan(&testData)
	if err != nil {
		log.Fatalf("Failed to read test data: %v", err)
	}

	if testData != "test connection data" {
		log.Fatalf("Data integrity check failed: expected 'test connection data', got '%s'", testData)
	}

	fmt.Println("✓ Database read operation verified")
	fmt.Println("✓ All database connection tests passed!")
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
