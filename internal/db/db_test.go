package db

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/anstrom/scanorama/internal/config"
)

func setupTestDB(t *testing.T) (*Database, func()) {
	cfg := &config.DatabaseConfig{
		Host:     "localhost",
		Port:     5432,
		Name:     "test_scanorama",
		User:     "postgres",
		Password: "postgres",
	}

	db, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	return db, func() {
		if err := db.Close(); err != nil {
			t.Errorf("Failed to close database: %v", err)
		}
	}
}

func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		config  *config.DatabaseConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: &config.DatabaseConfig{
				Host:     "localhost",
				Port:     5432,
				Name:     "test_db",
				User:     "postgres",
				Password: "postgres",
			},
			wantErr: false,
		},
		{
			name: "invalid port",
			config: &config.DatabaseConfig{
				Host:     "localhost",
				Port:     0,
				Name:     "test_db",
				User:     "postgres",
				Password: "postgres",
			},
			wantErr: true,
		},
		{
			name: "empty host",
			config: &config.DatabaseConfig{
				Host:     "",
				Port:     5432,
				Name:     "test_db",
				User:     "postgres",
				Password: "postgres",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, err := New(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err == nil {
				if err := db.Close(); err != nil {
					t.Errorf("Failed to close database: %v", err)
				}
			}
		})
	}
}

func TestConnect(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.Connect(ctx); err != nil {
		t.Errorf("Connect() error = %v", err)
	}
}

func TestPing(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.Connect(ctx); err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}

	if err := db.Ping(ctx); err != nil {
		t.Errorf("Ping() error = %v", err)
	}
}

func TestTransaction(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.Connect(ctx); err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}

	tests := []struct {
		name    string
		txFunc  func(*sql.Tx) error
		wantErr bool
	}{
		{
			name: "successful transaction",
			txFunc: func(tx *sql.Tx) error {
				_, err := tx.Exec("CREATE TEMPORARY TABLE test (id SERIAL PRIMARY KEY)")
				return err
			},
			wantErr: false,
		},
		{
			name: "failed transaction",
			txFunc: func(tx *sql.Tx) error {
				_, err := tx.Exec("INVALID SQL")
				return err
			},
			wantErr: true,
		},
		{
			name: "panic in transaction",
			txFunc: func(tx *sql.Tx) error {
				panic("test panic")
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := db.Transaction(ctx, tt.txFunc)
			if (err != nil) != tt.wantErr {
				t.Errorf("Transaction() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestQueryRow(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.Connect(ctx); err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}

	var result int
	err := db.QueryRow(ctx, "SELECT 1").Scan(&result)
	if err != nil {
		t.Errorf("QueryRow() error = %v", err)
	}

	if result != 1 {
		t.Errorf("QueryRow() = %v, want %v", result, 1)
	}
}

func TestQuery(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.Connect(ctx); err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}

	rows, err := db.Query(ctx, "SELECT generate_series(1, 3)")
	if err != nil {
		t.Errorf("Query() error = %v", err)
		return
	}
	defer rows.Close()

	var count int
	for rows.Next() {
		count++
		var val int
		if err := rows.Scan(&val); err != nil {
			t.Errorf("rows.Scan() error = %v", err)
		}
		if val != count {
			t.Errorf("Query() row %d = %v, want %v", count, val, count)
		}
	}

	if err := rows.Err(); err != nil {
		t.Errorf("rows.Err() = %v", err)
	}

	if count != 3 {
		t.Errorf("Query() row count = %v, want %v", count, 3)
	}
}

func TestExec(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.Connect(ctx); err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}

	result, err := db.Exec(ctx, "CREATE TEMPORARY TABLE test (id SERIAL PRIMARY KEY)")
	if err != nil {
		t.Errorf("Exec() error = %v", err)
		return
	}

	affected, err := result.RowsAffected()
	if err != nil {
		t.Errorf("RowsAffected() error = %v", err)
	}

	if affected != 0 {
		t.Errorf("RowsAffected() = %v, want %v", affected, 0)
	}
}
