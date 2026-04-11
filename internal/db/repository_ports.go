// Package db provides typed repository for port definition database operations.
package db

import (
	"context"
	"database/sql"
	stderrors "errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/lib/pq"

	"github.com/anstrom/scanorama/internal/errors"
)

// PortRepository handles port definition queries.
type PortRepository struct {
	db *DB
}

// NewPortRepository creates a new PortRepository.
func NewPortRepository(db *DB) *PortRepository {
	return &PortRepository{db: db}
}

// validPortSortColumns maps safe sort keys to SQL column names.
var validPortSortColumns = map[string]string{
	"port":     "port",
	"service":  "service",
	"category": "category",
	"protocol": "protocol",
}

// scanPortRow scans a single port_definitions row.
func scanPortRow(rows *sql.Rows) (*PortDefinition, error) {
	p := &PortDefinition{}
	var description sql.NullString
	var category sql.NullString
	var osFamilies pq.StringArray

	err := rows.Scan(
		&p.Port,
		&p.Protocol,
		&p.Service,
		&description,
		&category,
		&osFamilies,
		&p.IsStandard,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to scan port row: %w", err)
	}

	if description.Valid {
		p.Description = description.String
	}
	if category.Valid {
		p.Category = category.String
	}
	p.OSFamilies = []string(osFamilies)

	return p, nil
}

// ListPortDefinitions returns port definitions filtered by the given criteria.
func (r *PortRepository) ListPortDefinitions(
	ctx context.Context, filters PortFilters, offset, limit int,
) ([]*PortDefinition, int64, error) {
	var where []string
	var args []interface{}
	argIdx := 1

	if filters.Search != "" {
		// Match port number or service/description text.
		where = append(where, fmt.Sprintf(
			"(service ILIKE $%d OR description ILIKE $%d OR port::text = $%d)",
			argIdx, argIdx+1, argIdx+2))
		pattern := "%" + filters.Search + "%"
		args = append(args, pattern, pattern, filters.Search)
		argIdx += 3
	}
	if filters.Category != "" {
		where = append(where, fmt.Sprintf("category = $%d", argIdx))
		args = append(args, filters.Category)
		argIdx++
	}
	if filters.Protocol != "" {
		where = append(where, fmt.Sprintf("protocol = $%d", argIdx))
		args = append(args, filters.Protocol)
		argIdx++
	}

	whereClause := ""
	if len(where) > 0 {
		whereClause = "WHERE " + strings.Join(where, " AND ")
	}

	var total int64
	countQ := fmt.Sprintf("SELECT COUNT(*) FROM port_definitions %s", whereClause)
	if err := r.db.QueryRowContext(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, sanitizeDBError("count port definitions", err)
	}

	orderBy := "ORDER BY port ASC, protocol ASC"
	if filters.SortBy != "" {
		if col, ok := validPortSortColumns[filters.SortBy]; ok {
			dir := sortOrderASC
			if strings.EqualFold(filters.SortOrder, sortOrderDESC) {
				dir = sortOrderDESC
			}
			orderBy = fmt.Sprintf("ORDER BY %s %s", col, dir)
		}
	}

	listQ := fmt.Sprintf(
		`SELECT port, protocol, service, description, category, os_families, is_standard
		 FROM port_definitions %s %s LIMIT $%d OFFSET $%d`,
		whereClause, orderBy, argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := r.db.QueryContext(ctx, listQ, args...)
	if err != nil {
		return nil, 0, sanitizeDBError("list port definitions", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("error closing rows", "error", err)
		}
	}()

	var ports []*PortDefinition
	for rows.Next() {
		p, err := scanPortRow(rows)
		if err != nil {
			return nil, 0, err
		}
		ports = append(ports, p)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("failed to iterate port rows: %w", err)
	}

	return ports, total, nil
}

// GetPortDefinition retrieves a single port definition by port + protocol.
func (r *PortRepository) GetPortDefinition(ctx context.Context, port int, protocol string) (*PortDefinition, error) {
	query := `
		SELECT port, protocol, service, description, category, os_families, is_standard
		FROM port_definitions
		WHERE port = $1 AND protocol = $2`

	rows, err := r.db.QueryContext(ctx, query, port, protocol)
	if err != nil {
		return nil, sanitizeDBError("get port definition", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("error closing rows", "error", err)
		}
	}()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, sanitizeDBError("get port definition", err)
		}
		return nil, errors.ErrNotFound("port definition")
	}

	return scanPortRow(rows)
}

// LookupPort returns the known service name for a port+protocol, or empty string.
// This is a lightweight helper for enrichment use; it tolerates a missing row.
func (r *PortRepository) LookupPort(ctx context.Context, port int, protocol string) string {
	var service string
	err := r.db.QueryRowContext(ctx,
		"SELECT service FROM port_definitions WHERE port = $1 AND protocol = $2",
		port, protocol).Scan(&service)
	if err != nil {
		if stderrors.Is(err, sql.ErrNoRows) {
			return ""
		}
		slog.Warn("port lookup failed", "port", port, "protocol", protocol, "error", err)
		return ""
	}
	return service
}

// ListCategories returns the distinct category values present in the table.
func (r *PortRepository) ListCategories(ctx context.Context) ([]string, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT DISTINCT category FROM port_definitions WHERE category IS NOT NULL ORDER BY category")
	if err != nil {
		return nil, sanitizeDBError("list categories", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("error closing rows", "error", err)
		}
	}()

	var cats []string
	for rows.Next() {
		var cat string
		if err := rows.Scan(&cat); err != nil {
			return nil, fmt.Errorf("failed to scan category: %w", err)
		}
		cats = append(cats, cat)
	}
	return cats, rows.Err()
}
