// Package db — unit tests for PortRepository using sqlmock.
package db

import (
	"context"
	"fmt"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anstrom/scanorama/internal/errors"
)

var portCols = []string{"port", "protocol", "service", "description", "category", "os_families", "is_standard"}

func makePortRow(
	port int, protocol, service, description, category string,
	osFamilies interface{}, isStandard bool,
) *sqlmock.Rows {
	return sqlmock.NewRows(portCols).
		AddRow(port, protocol, service, description, category, osFamilies, isStandard)
}

// ── NewPortRepository ──────────────────────────────────────────────────────────

func TestPortRepository_New(t *testing.T) {
	db, _ := newMockDB(t)
	repo := NewPortRepository(db)
	require.NotNil(t, repo)
}

// ── ListPortDefinitions ────────────────────────────────────────────────────────

func TestPortRepository_ListPortDefinitions_NoFilters(t *testing.T) {
	db, mock := newMockDB(t)

	mock.ExpectQuery("SELECT COUNT").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))

	// os_families is nil here; parsePostgreSQLArray behavior with real
	// PostgreSQL arrays is exercised via the existing parsePostgreSQLArray tests.
	rows := sqlmock.NewRows(portCols).
		AddRow(80, "tcp", "http", "Hypertext Transfer Protocol", "web", nil, true).
		AddRow(443, "tcp", "https", "HTTP over TLS", "web", nil, true)
	mock.ExpectQuery("SELECT port").WillReturnRows(rows)

	repo := NewPortRepository(db)
	ports, total, err := repo.ListPortDefinitions(context.Background(), PortFilters{}, 0, 20)

	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	require.Len(t, ports, 2)
	assert.Equal(t, 80, ports[0].Port)
	assert.Equal(t, "http", ports[0].Service)
	assert.Nil(t, ports[0].OSFamilies)
	assert.Equal(t, 443, ports[1].Port)
	assert.Nil(t, ports[1].OSFamilies)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPortRepository_ListPortDefinitions_SearchFilter(t *testing.T) {
	db, mock := newMockDB(t)

	mock.ExpectQuery("SELECT COUNT").
		WithArgs("%http%", "%http%", "http").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	rows := sqlmock.NewRows(portCols).
		AddRow(80, "tcp", "http", "Hypertext Transfer Protocol", "web", nil, true)
	mock.ExpectQuery("SELECT port").
		WithArgs("%http%", "%http%", "http", 20, 0).
		WillReturnRows(rows)

	repo := NewPortRepository(db)
	filters := PortFilters{Search: "http"}
	ports, total, err := repo.ListPortDefinitions(context.Background(), filters, 0, 20)

	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, ports, 1)
	assert.Equal(t, "http", ports[0].Service)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPortRepository_ListPortDefinitions_CategoryFilter(t *testing.T) {
	db, mock := newMockDB(t)

	mock.ExpectQuery("SELECT COUNT").
		WithArgs("database").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	rows := sqlmock.NewRows(portCols).
		AddRow(5432, "tcp", "postgresql", "PostgreSQL database", "database", nil, true)
	mock.ExpectQuery("SELECT port").
		WithArgs("database", 20, 0).
		WillReturnRows(rows)

	repo := NewPortRepository(db)
	filters := PortFilters{Category: "database"}
	ports, total, err := repo.ListPortDefinitions(context.Background(), filters, 0, 20)

	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, ports, 1)
	assert.Equal(t, 5432, ports[0].Port)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPortRepository_ListPortDefinitions_ProtocolFilter(t *testing.T) {
	db, mock := newMockDB(t)

	mock.ExpectQuery("SELECT COUNT").
		WithArgs("udp").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	rows := sqlmock.NewRows(portCols).
		AddRow(53, "udp", "domain", "DNS", "infrastructure", nil, true)
	mock.ExpectQuery("SELECT port").
		WithArgs("udp", 20, 0).
		WillReturnRows(rows)

	repo := NewPortRepository(db)
	filters := PortFilters{Protocol: "udp"}
	ports, total, err := repo.ListPortDefinitions(context.Background(), filters, 0, 20)

	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, ports, 1)
	assert.Equal(t, "udp", ports[0].Protocol)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPortRepository_ListPortDefinitions_SortByService(t *testing.T) {
	db, mock := newMockDB(t)

	mock.ExpectQuery("SELECT COUNT").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))

	rows := sqlmock.NewRows(portCols).
		AddRow(21, "tcp", "ftp", "File Transfer Protocol", "file", nil, true).
		AddRow(80, "tcp", "http", "Hypertext Transfer Protocol", "web", nil, true)
	mock.ExpectQuery("SELECT port").WillReturnRows(rows)

	repo := NewPortRepository(db)
	filters := PortFilters{SortBy: "service", SortOrder: "asc"}
	ports, total, err := repo.ListPortDefinitions(context.Background(), filters, 0, 20)

	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	require.Len(t, ports, 2)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPortRepository_ListPortDefinitions_CountError(t *testing.T) {
	db, mock := newMockDB(t)

	mock.ExpectQuery("SELECT COUNT").
		WillReturnError(fmt.Errorf("connection reset"))

	repo := NewPortRepository(db)
	_, _, err := repo.ListPortDefinitions(context.Background(), PortFilters{}, 0, 20)

	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPortRepository_ListPortDefinitions_QueryError(t *testing.T) {
	db, mock := newMockDB(t)

	mock.ExpectQuery("SELECT COUNT").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(5))
	mock.ExpectQuery("SELECT port").
		WillReturnError(fmt.Errorf("connection refused"))

	repo := NewPortRepository(db)
	_, _, err := repo.ListPortDefinitions(context.Background(), PortFilters{}, 0, 20)

	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── GetPortDefinition ──────────────────────────────────────────────────────────

func TestPortRepository_GetPortDefinition_Found(t *testing.T) {
	db, mock := newMockDB(t)

	mock.ExpectQuery("SELECT port").
		WithArgs(80, "tcp").
		WillReturnRows(makePortRow(80, "tcp", "http", "Hypertext Transfer Protocol", "web", nil, true))

	repo := NewPortRepository(db)
	def, err := repo.GetPortDefinition(context.Background(), 80, "tcp")

	require.NoError(t, err)
	require.NotNil(t, def)
	assert.Equal(t, 80, def.Port)
	assert.Equal(t, "tcp", def.Protocol)
	assert.Equal(t, "http", def.Service)
	assert.Equal(t, "Hypertext Transfer Protocol", def.Description)
	assert.Equal(t, "web", def.Category)
	assert.True(t, def.IsStandard)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPortRepository_GetPortDefinition_NotFound(t *testing.T) {
	db, mock := newMockDB(t)

	mock.ExpectQuery("SELECT port").
		WithArgs(9999, "tcp").
		WillReturnRows(sqlmock.NewRows(portCols))

	repo := NewPortRepository(db)
	def, err := repo.GetPortDefinition(context.Background(), 9999, "tcp")

	require.Error(t, err)
	assert.Nil(t, def)
	assert.True(t, errors.IsNotFound(err), "expected not-found error, got: %v", err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPortRepository_GetPortDefinition_QueryError(t *testing.T) {
	db, mock := newMockDB(t)

	mock.ExpectQuery("SELECT port").
		WithArgs(80, "tcp").
		WillReturnError(fmt.Errorf("connection reset"))

	repo := NewPortRepository(db)
	_, err := repo.GetPortDefinition(context.Background(), 80, "tcp")

	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── LookupPort ────────────────────────────────────────────────────────────────

func TestPortRepository_LookupPort_Found(t *testing.T) {
	db, mock := newMockDB(t)

	mock.ExpectQuery("SELECT service").
		WithArgs(80, "tcp").
		WillReturnRows(sqlmock.NewRows([]string{"service"}).AddRow("http"))

	repo := NewPortRepository(db)
	service := repo.LookupPort(context.Background(), 80, "tcp")

	assert.Equal(t, "http", service)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPortRepository_LookupPort_NotFound(t *testing.T) {
	db, mock := newMockDB(t)

	mock.ExpectQuery("SELECT service").
		WithArgs(9999, "tcp").
		WillReturnRows(sqlmock.NewRows([]string{"service"}))

	repo := NewPortRepository(db)
	service := repo.LookupPort(context.Background(), 9999, "tcp")

	assert.Equal(t, "", service)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPortRepository_LookupPort_DBError(t *testing.T) {
	db, mock := newMockDB(t)

	mock.ExpectQuery("SELECT service").
		WithArgs(80, "tcp").
		WillReturnError(fmt.Errorf("connection refused"))

	repo := NewPortRepository(db)
	service := repo.LookupPort(context.Background(), 80, "tcp")

	assert.Equal(t, "", service)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── ListCategories ─────────────────────────────────────────────────────────────

func TestPortRepository_ListCategories_Found(t *testing.T) {
	db, mock := newMockDB(t)

	mock.ExpectQuery("SELECT DISTINCT category").
		WillReturnRows(sqlmock.NewRows([]string{"category"}).
			AddRow("database").
			AddRow("web").
			AddRow("windows"))

	repo := NewPortRepository(db)
	cats, err := repo.ListCategories(context.Background())

	require.NoError(t, err)
	require.Len(t, cats, 3)
	assert.Equal(t, []string{"database", "web", "windows"}, cats)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPortRepository_ListCategories_Empty(t *testing.T) {
	db, mock := newMockDB(t)

	mock.ExpectQuery("SELECT DISTINCT category").
		WillReturnRows(sqlmock.NewRows([]string{"category"}))

	repo := NewPortRepository(db)
	cats, err := repo.ListCategories(context.Background())

	require.NoError(t, err)
	assert.Nil(t, cats)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPortRepository_ListCategories_QueryError(t *testing.T) {
	db, mock := newMockDB(t)

	mock.ExpectQuery("SELECT DISTINCT category").
		WillReturnError(fmt.Errorf("connection reset"))

	repo := NewPortRepository(db)
	_, err := repo.ListCategories(context.Background())

	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
