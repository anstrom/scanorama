// Package db contains unit tests for host status model changes.
package db

import (
	"context"
	"fmt"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
)

func TestMarkGoneHosts_Unit(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("marks missing hosts as gone", func(t *testing.T) {
		t.Parallel()
		db, mock := newMockDB(t)
		repo := NewHostRepository(db)

		mock.ExpectExec(`UPDATE hosts`).
			WithArgs(
				HostStatusGone,
				"192.168.1.0/24",
				HostStatusUp,
				pq.Array([]string{"192.168.1.1", "192.168.1.2"}),
			).
			WillReturnResult(sqlmock.NewResult(0, 3))

		n, err := repo.MarkGoneHosts(ctx, "192.168.1.0/24", []string{"192.168.1.1", "192.168.1.2"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if n != 3 {
			t.Errorf("got %d gone hosts, want 3", n)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet expectations: %v", err)
		}
	})

	t.Run("empty discovered list marks all up hosts gone", func(t *testing.T) {
		t.Parallel()
		db, mock := newMockDB(t)
		repo := NewHostRepository(db)

		mock.ExpectExec(`UPDATE hosts`).
			WithArgs(
				HostStatusGone,
				"10.0.0.0/8",
				HostStatusUp,
				pq.Array([]string{}),
			).
			WillReturnResult(sqlmock.NewResult(0, 10))

		n, err := repo.MarkGoneHosts(ctx, "10.0.0.0/8", []string{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if n != 10 {
			t.Errorf("got %d gone hosts, want 10", n)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet expectations: %v", err)
		}
	})

	t.Run("zero rows affected returns 0 without error", func(t *testing.T) {
		t.Parallel()
		db, mock := newMockDB(t)
		repo := NewHostRepository(db)

		mock.ExpectExec(`UPDATE hosts`).
			WithArgs(
				HostStatusGone,
				"172.16.0.0/12",
				HostStatusUp,
				pq.Array([]string{"172.16.0.1"}),
			).
			WillReturnResult(sqlmock.NewResult(0, 0))

		n, err := repo.MarkGoneHosts(ctx, "172.16.0.0/12", []string{"172.16.0.1"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if n != 0 {
			t.Errorf("got %d, want 0", n)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet expectations: %v", err)
		}
	})

	t.Run("propagates database error", func(t *testing.T) {
		t.Parallel()
		db, mock := newMockDB(t)
		repo := NewHostRepository(db)

		mock.ExpectExec(`UPDATE hosts`).
			WillReturnError(fmt.Errorf("connection reset"))

		_, err := repo.MarkGoneHosts(ctx, "192.168.0.0/16", []string{"192.168.0.1"})
		if err == nil {
			t.Fatal("expected an error, got nil")
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet expectations: %v", err)
		}
	})
}
