package db

import (
	"context"
	"fmt"
	"regexp"

	"github.com/lib/pq"
)

// maxIdentifierLen is PostgreSQL's default identifier length limit (NAMEDATALEN-1).
const maxIdentifierLen = 63

// identifierPattern matches a safe, unquoted SQL identifier: a leading letter or
// underscore followed by letters, digits, or underscores. CREATE ROLE/DATABASE
// cannot bind the object name as a parameter, so names are validated here and
// quoted at use; this keeps `scanorama setup` from passing anything injectable.
var identifierPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// BootstrapResult reports what EnsureRoleAndDatabase changed, so callers can tell
// a fresh bootstrap from a no-op re-run.
type BootstrapResult struct {
	RoleCreated     bool
	DatabaseCreated bool
}

// validateIdentifier ensures name is a plain SQL identifier of acceptable length.
func validateIdentifier(name string) error {
	if len(name) > maxIdentifierLen {
		return fmt.Errorf("identifier %q exceeds %d characters", name, maxIdentifierLen)
	}
	if !identifierPattern.MatchString(name) {
		return fmt.Errorf("identifier %q is not a valid SQL identifier", name)
	}
	return nil
}

// EnsureRoleAndDatabase idempotently creates a LOGIN role and a database it owns.
// It backs `scanorama setup`: bootstrapping a fresh local PostgreSQL so the
// unprivileged daemon can connect over the Unix socket via peer authentication.
//
// conn must be authenticated as a superuser (the postgres role). The role is
// created with LOGIN and no password — authentication is peer-based, keyed on the
// connecting OS user, so there is no secret to store. Re-running is a no-op.
func EnsureRoleAndDatabase(ctx context.Context, conn *DB, role, database string) (BootstrapResult, error) {
	var res BootstrapResult

	if err := validateIdentifier(role); err != nil {
		return res, fmt.Errorf("invalid role name: %w", err)
	}
	if err := validateIdentifier(database); err != nil {
		return res, fmt.Errorf("invalid database name: %w", err)
	}

	roleExists, err := existsQuery(ctx, conn,
		"SELECT EXISTS(SELECT 1 FROM pg_roles WHERE rolname = $1)", role)
	if err != nil {
		return res, fmt.Errorf("checking role %q: %w", role, err)
	}
	if !roleExists {
		if _, err := conn.ExecContext(ctx, "CREATE ROLE "+pq.QuoteIdentifier(role)+" LOGIN"); err != nil {
			return res, fmt.Errorf("creating role %q: %w", role, err)
		}
		res.RoleCreated = true
	}

	dbExists, err := existsQuery(ctx, conn,
		"SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)", database)
	if err != nil {
		return res, fmt.Errorf("checking database %q: %w", database, err)
	}
	if !dbExists {
		stmt := fmt.Sprintf("CREATE DATABASE %s OWNER %s",
			pq.QuoteIdentifier(database), pq.QuoteIdentifier(role))
		if _, err := conn.ExecContext(ctx, stmt); err != nil {
			return res, fmt.Errorf("creating database %q: %w", database, err)
		}
		res.DatabaseCreated = true
	}

	return res, nil
}

// existsQuery runs an EXISTS probe and returns its boolean result.
func existsQuery(ctx context.Context, conn *DB, query string, args ...any) (bool, error) {
	var exists bool
	if err := conn.GetContext(ctx, &exists, query, args...); err != nil {
		return false, err
	}
	return exists, nil
}
