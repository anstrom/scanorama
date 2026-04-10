// Package db — unit tests for GroupRepository using sqlmock.
// These run without a live database and complement the integration tests in
// tags_groups_repository_test.go.
package db

import (
	"context"
	"database/sql/driver"
	"fmt"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/lib/pq/pqerror"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anstrom/scanorama/internal/errors"
)

// Compile-time check that driver.Value is used correctly.
var _ driver.Value = driver.Value(nil)

// groupCols lists the eight columns returned by every group SELECT.
var groupCols = []string{
	"id", "name", "description", "filter_rule",
	"color", "member_count", "created_at", "updated_at",
}

// makeGroupRow returns a sqlmock row for a single HostGroup.
func makeGroupRow(id uuid.UUID, name string) *sqlmock.Rows {
	now := time.Now().UTC()
	return sqlmock.NewRows(groupCols).
		AddRow(id, name, "test description", "null", "#3b82f6", 0, now, now)
}

// ── NewGroupRepository ─────────────────────────────────────────────────────

func TestGroupRepository_New(t *testing.T) {
	db, _ := newMockDB(t)
	repo := NewGroupRepository(db)
	require.NotNil(t, repo)
}

// ── CreateGroup ────────────────────────────────────────────────────────────

func TestGroupRepository_CreateGroup_DuplicateName(t *testing.T) {
	db, mock := newMockDB(t)

	pqErr := &pq.Error{
		Code:       pqerror.UniqueViolation,
		Constraint: "uq_host_groups_name",
		Message:    "duplicate key value",
	}
	mock.ExpectExec("INSERT INTO host_groups").WillReturnError(pqErr)

	_, err := NewGroupRepository(db).CreateGroup(context.Background(), CreateGroupInput{
		Name: "engineering",
	})

	require.Error(t, err)
	assert.True(t, errors.IsCode(err, errors.CodeConflict),
		"expected conflict error, got: %v", err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGroupRepository_CreateGroup_DBError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectExec("INSERT INTO host_groups").WillReturnError(fmt.Errorf("connection reset"))

	_, err := NewGroupRepository(db).CreateGroup(context.Background(), CreateGroupInput{Name: "x"})

	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGroupRepository_CreateGroup_Success(t *testing.T) {
	db, mock := newMockDB(t)
	id := uuid.New()

	mock.ExpectExec("INSERT INTO host_groups").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery("SELECT").WillReturnRows(makeGroupRow(id, "engineering"))

	g, err := NewGroupRepository(db).CreateGroup(context.Background(), CreateGroupInput{
		Name:        "engineering",
		Description: "eng team",
		Color:       "#3b82f6",
	})

	require.NoError(t, err)
	assert.Equal(t, "engineering", g.Name)
	assert.Equal(t, id, g.ID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGroupRepository_CreateGroup_WithFilterRule(t *testing.T) {
	db, mock := newMockDB(t)
	id := uuid.New()
	rule := JSONB(`{"field":"status","cmp":"is","value":"up"}`)

	mock.ExpectExec("INSERT INTO host_groups").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery("SELECT").WillReturnRows(makeGroupRow(id, "auto-group"))

	g, err := NewGroupRepository(db).CreateGroup(context.Background(), CreateGroupInput{
		Name:       "auto-group",
		FilterRule: &rule,
	})

	require.NoError(t, err)
	assert.Equal(t, "auto-group", g.Name)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── GetGroup ───────────────────────────────────────────────────────────────

func TestGroupRepository_GetGroup_NotFound(t *testing.T) {
	db, mock := newMockDB(t)
	id := uuid.New()

	mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows(groupCols))

	_, err := NewGroupRepository(db).GetGroup(context.Background(), id)

	require.Error(t, err)
	assert.True(t, errors.IsNotFound(err), "expected not-found, got: %v", err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGroupRepository_GetGroup_DBError(t *testing.T) {
	db, mock := newMockDB(t)

	mock.ExpectQuery("SELECT").WillReturnError(fmt.Errorf("timeout"))

	_, err := NewGroupRepository(db).GetGroup(context.Background(), uuid.New())

	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGroupRepository_GetGroup_Success(t *testing.T) {
	db, mock := newMockDB(t)
	id := uuid.New()
	now := time.Now().UTC()

	mock.ExpectQuery("SELECT").
		WillReturnRows(sqlmock.NewRows(groupCols).
			AddRow(id, "infra", "infra team", "null", "#10b981", 3, now, now))

	g, err := NewGroupRepository(db).GetGroup(context.Background(), id)

	require.NoError(t, err)
	assert.Equal(t, id, g.ID)
	assert.Equal(t, "infra", g.Name)
	assert.Equal(t, "infra team", g.Description)
	assert.Equal(t, "#10b981", g.Color)
	assert.Equal(t, 3, g.MemberCount)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGroupRepository_GetGroup_WithFilterRule(t *testing.T) {
	db, mock := newMockDB(t)
	id := uuid.New()
	now := time.Now().UTC()
	filterJSON := `{"field":"status","cmp":"is","value":"up"}`

	mock.ExpectQuery("SELECT").
		WillReturnRows(sqlmock.NewRows(groupCols).
			AddRow(id, "auto", "", filterJSON, "", 0, now, now))

	g, err := NewGroupRepository(db).GetGroup(context.Background(), id)

	require.NoError(t, err)
	assert.Equal(t, JSONB(filterJSON), g.FilterRule)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── ListGroups ─────────────────────────────────────────────────────────────

func TestGroupRepository_ListGroups_Empty(t *testing.T) {
	db, mock := newMockDB(t)

	mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows(groupCols))

	groups, err := NewGroupRepository(db).ListGroups(context.Background())

	require.NoError(t, err)
	assert.Empty(t, groups)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGroupRepository_ListGroups_DBError(t *testing.T) {
	db, mock := newMockDB(t)

	mock.ExpectQuery("SELECT").WillReturnError(fmt.Errorf("timeout"))

	_, err := NewGroupRepository(db).ListGroups(context.Background())

	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGroupRepository_ListGroups_Multiple(t *testing.T) {
	db, mock := newMockDB(t)
	now := time.Now().UTC()
	id1, id2 := uuid.New(), uuid.New()

	mock.ExpectQuery("SELECT").
		WillReturnRows(sqlmock.NewRows(groupCols).
			AddRow(id1, "alpha", "", "null", "", 2, now, now).
			AddRow(id2, "beta", "", "null", "#fff", 5, now, now))

	groups, err := NewGroupRepository(db).ListGroups(context.Background())

	require.NoError(t, err)
	require.Len(t, groups, 2)
	assert.Equal(t, "alpha", groups[0].Name)
	assert.Equal(t, 2, groups[0].MemberCount)
	assert.Equal(t, "beta", groups[1].Name)
	assert.Equal(t, 5, groups[1].MemberCount)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── UpdateGroup ────────────────────────────────────────────────────────────

func TestGroupRepository_UpdateGroup_BeginTxFails(t *testing.T) {
	db, mock := newMockDB(t)

	mock.ExpectBegin().WillReturnError(fmt.Errorf("connection lost"))

	name := "new"
	_, err := NewGroupRepository(db).UpdateGroup(context.Background(), uuid.New(),
		UpdateGroupInput{Name: &name})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "begin transaction")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGroupRepository_UpdateGroup_NotFound(t *testing.T) {
	db, mock := newMockDB(t)
	id := uuid.New()
	name := "new"

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT EXISTS").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectRollback()

	_, err := NewGroupRepository(db).UpdateGroup(context.Background(), id,
		UpdateGroupInput{Name: &name})

	require.Error(t, err)
	assert.True(t, errors.IsNotFound(err), "expected not-found, got: %v", err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGroupRepository_UpdateGroup_NoFields(t *testing.T) {
	db, mock := newMockDB(t)

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT EXISTS").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectRollback()

	_, err := NewGroupRepository(db).UpdateGroup(context.Background(), uuid.New(),
		UpdateGroupInput{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "no fields")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGroupRepository_UpdateGroup_DuplicateName(t *testing.T) {
	db, mock := newMockDB(t)
	id := uuid.New()
	name := "taken"

	pqErr := &pq.Error{Code: pqerror.UniqueViolation, Constraint: "uq_host_groups_name"}

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT EXISTS").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec("UPDATE host_groups SET").WillReturnError(pqErr)
	mock.ExpectRollback()

	_, err := NewGroupRepository(db).UpdateGroup(context.Background(), id,
		UpdateGroupInput{Name: &name})

	require.Error(t, err)
	assert.True(t, errors.IsCode(err, errors.CodeConflict))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGroupRepository_UpdateGroup_Success(t *testing.T) {
	db, mock := newMockDB(t)
	id := uuid.New()
	name := "updated"

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT EXISTS").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec("UPDATE host_groups SET").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	mock.ExpectQuery("SELECT").WillReturnRows(makeGroupRow(id, name))

	g, err := NewGroupRepository(db).UpdateGroup(context.Background(), id,
		UpdateGroupInput{Name: &name})

	require.NoError(t, err)
	assert.Equal(t, name, g.Name)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGroupRepository_UpdateGroup_ClearFilter(t *testing.T) {
	db, mock := newMockDB(t)
	id := uuid.New()
	name := "filtered"

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT EXISTS").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec("UPDATE host_groups SET").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	mock.ExpectQuery("SELECT").WillReturnRows(makeGroupRow(id, name))

	g, err := NewGroupRepository(db).UpdateGroup(context.Background(), id,
		UpdateGroupInput{Name: &name, ClearFilter: true})

	require.NoError(t, err)
	assert.Equal(t, name, g.Name)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── DeleteGroup ────────────────────────────────────────────────────────────

func TestGroupRepository_DeleteGroup_NotFound(t *testing.T) {
	db, mock := newMockDB(t)

	mock.ExpectExec("DELETE FROM host_groups").
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := NewGroupRepository(db).DeleteGroup(context.Background(), uuid.New())

	require.Error(t, err)
	assert.True(t, errors.IsNotFound(err))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGroupRepository_DeleteGroup_DBError(t *testing.T) {
	db, mock := newMockDB(t)

	mock.ExpectExec("DELETE FROM host_groups").WillReturnError(fmt.Errorf("timeout"))

	err := NewGroupRepository(db).DeleteGroup(context.Background(), uuid.New())

	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGroupRepository_DeleteGroup_Success(t *testing.T) {
	db, mock := newMockDB(t)

	mock.ExpectExec("DELETE FROM host_groups").
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := NewGroupRepository(db).DeleteGroup(context.Background(), uuid.New())

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── AddHostsToGroup ────────────────────────────────────────────────────────

func TestGroupRepository_AddHostsToGroup_EmptyIDs_NoOp(t *testing.T) {
	db, mock := newMockDB(t)

	// No DB calls expected for an empty slice.
	err := NewGroupRepository(db).AddHostsToGroup(context.Background(), uuid.New(), nil)

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGroupRepository_AddHostsToGroup_DBError(t *testing.T) {
	db, mock := newMockDB(t)

	mock.ExpectExec("INSERT INTO host_group_members").
		WillReturnError(fmt.Errorf("foreign key violation"))

	err := NewGroupRepository(db).AddHostsToGroup(context.Background(),
		uuid.New(), []uuid.UUID{uuid.New()})

	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGroupRepository_AddHostsToGroup_Success(t *testing.T) {
	db, mock := newMockDB(t)

	mock.ExpectExec("INSERT INTO host_group_members").
		WillReturnResult(sqlmock.NewResult(2, 2))

	err := NewGroupRepository(db).AddHostsToGroup(context.Background(),
		uuid.New(), []uuid.UUID{uuid.New(), uuid.New()})

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── RemoveHostsFromGroup ───────────────────────────────────────────────────

func TestGroupRepository_RemoveHostsFromGroup_EmptyIDs_NoOp(t *testing.T) {
	db, mock := newMockDB(t)

	err := NewGroupRepository(db).RemoveHostsFromGroup(context.Background(), uuid.New(), nil)

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGroupRepository_RemoveHostsFromGroup_DBError(t *testing.T) {
	db, mock := newMockDB(t)

	mock.ExpectExec("DELETE FROM host_group_members").
		WillReturnError(fmt.Errorf("timeout"))

	err := NewGroupRepository(db).RemoveHostsFromGroup(context.Background(),
		uuid.New(), []uuid.UUID{uuid.New()})

	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGroupRepository_RemoveHostsFromGroup_Success(t *testing.T) {
	db, mock := newMockDB(t)

	mock.ExpectExec("DELETE FROM host_group_members").
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := NewGroupRepository(db).RemoveHostsFromGroup(context.Background(),
		uuid.New(), []uuid.UUID{uuid.New()})

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── GetGroupMembers ────────────────────────────────────────────────────────

func TestGroupRepository_GetGroupMembers_CountError(t *testing.T) {
	db, mock := newMockDB(t)

	mock.ExpectQuery("SELECT COUNT").WillReturnError(fmt.Errorf("timeout"))

	_, _, err := NewGroupRepository(db).GetGroupMembers(context.Background(), uuid.New(), 0, 10)

	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGroupRepository_GetGroupMembers_Empty(t *testing.T) {
	db, mock := newMockDB(t)

	mock.ExpectQuery("SELECT COUNT").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT").
		WillReturnRows(sqlmock.NewRows(groupMemberCols()))

	hosts, total, err := NewGroupRepository(db).GetGroupMembers(context.Background(), uuid.New(), 0, 10)

	require.NoError(t, err)
	assert.Equal(t, int64(0), total)
	assert.Empty(t, hosts)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGroupRepository_GetGroupMembers_WithHosts(t *testing.T) {
	db, mock := newMockDB(t)
	now := time.Now().UTC()
	hostID := uuid.New()

	mock.ExpectQuery("SELECT COUNT").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT").
		WillReturnRows(sqlmock.NewRows(groupMemberCols()).
			AddRow(groupMemberRow(hostID, "10.0.0.1", "up", now)...))

	hosts, total, err := NewGroupRepository(db).GetGroupMembers(context.Background(), uuid.New(), 0, 10)

	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, hosts, 1)
	assert.Equal(t, hostID, hosts[0].ID)
	assert.Equal(t, "10.0.0.1", hosts[0].IPAddress.String())
	assert.Equal(t, "up", hosts[0].Status)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGroupRepository_GetGroupMembers_QueryError(t *testing.T) {
	db, mock := newMockDB(t)

	mock.ExpectQuery("SELECT COUNT").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	mock.ExpectQuery("SELECT").WillReturnError(fmt.Errorf("timeout"))

	_, _, err := NewGroupRepository(db).GetGroupMembers(context.Background(), uuid.New(), 0, 10)

	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── helpers ────────────────────────────────────────────────────────────────

// groupMemberCols returns the 25 column names matching the GetGroupMembers SELECT.
func groupMemberCols() []string {
	return []string{
		"id", "ip_address", "hostname", "mac_address", "vendor",
		"os_family", "os_name", "os_version", "os_confidence", "os_detected_at",
		"os_method", "os_details", "discovery_method",
		"response_time_ms", "response_time_min_ms", "response_time_max_ms", "response_time_avg_ms",
		"ignore_scanning", "first_seen", "last_seen", "status",
		"status_changed_at", "previous_status", "timeout_count",
		"tags",
	}
}

// groupMemberRow returns a minimal 25-value slice for a group member row.
// Nullable columns that are not under test are set to nil.
// Returns []driver.Value to match sqlmock v1's AddRow signature.
func groupMemberRow(id uuid.UUID, ip, status string, ts time.Time) []driver.Value {
	return []driver.Value{
		id,               // id
		ip,               // ip_address
		nil,              // hostname
		nil,              // mac_address
		nil,              // vendor
		nil,              // os_family
		nil,              // os_name
		nil,              // os_version
		nil,              // os_confidence
		nil,              // os_detected_at
		nil,              // os_method
		nil,              // os_details
		nil,              // discovery_method
		nil,              // response_time_ms
		nil,              // response_time_min_ms
		nil,              // response_time_max_ms
		nil,              // response_time_avg_ms
		false,            // ignore_scanning
		ts,               // first_seen
		ts,               // last_seen
		status,           // status
		nil,              // status_changed_at
		nil,              // previous_status
		0,                // timeout_count
		pq.StringArray{}, // tags
	}
}
