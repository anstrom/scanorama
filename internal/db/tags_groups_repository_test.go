// Package db provides integration tests for host-tag and host-group repository
// operations. These tests require a running Postgres instance; they are
// skipped automatically in short mode (or when no database is reachable).
package db

import (
	"context"
	"sort"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── Host tag tests ─────────────────────────────────────────────────────────────

// TestHostTags_CreateWithTags verifies that tags supplied at creation time are
// persisted and returned correctly by GetHost.
func TestHostTags_CreateWithTags(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()
	ctx := context.Background()

	const ip = "198.51.100.1"
	_, _ = db.ExecContext(ctx, "DELETE FROM hosts WHERE ip_address = $1::inet", ip)

	repo := NewHostRepository(db)
	host, err := repo.CreateHost(ctx, CreateHostInput{
		IPAddress: ip,
		Status:    HostStatusUp,
		Tags:      []string{"prod", "web"},
	})
	require.NoError(t, err)
	require.NotNil(t, host)

	got, err := repo.GetHost(ctx, host.ID)
	require.NoError(t, err)

	gotTags := make([]string, len(got.Tags))
	copy(gotTags, got.Tags)
	sort.Strings(gotTags)
	assert.Equal(t, []string{"prod", "web"}, gotTags)
}

// TestHostTags_DefaultsToEmpty verifies that a host created with nil Tags has
// a non-nil, empty tag list when read back.
func TestHostTags_DefaultsToEmpty(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()
	ctx := context.Background()

	const ip = "198.51.100.2"
	_, _ = db.ExecContext(ctx, "DELETE FROM hosts WHERE ip_address = $1::inet", ip)

	repo := NewHostRepository(db)
	host, err := repo.CreateHost(ctx, CreateHostInput{
		IPAddress: ip,
		Status:    HostStatusUp,
		Tags:      nil,
	})
	require.NoError(t, err)
	require.NotNil(t, host)

	got, err := repo.GetHost(ctx, host.ID)
	require.NoError(t, err)

	assert.NotNil(t, got.Tags)
	assert.Len(t, got.Tags, 0)
}

// TestHostTags_UpdateReplacesAll verifies that UpdateHostTags replaces the
// complete tag list rather than appending to it.
func TestHostTags_UpdateReplacesAll(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()
	ctx := context.Background()

	const ip = "198.51.100.3"
	_, _ = db.ExecContext(ctx, "DELETE FROM hosts WHERE ip_address = $1::inet", ip)

	repo := NewHostRepository(db)
	host, err := repo.CreateHost(ctx, CreateHostInput{
		IPAddress: ip,
		Status:    HostStatusUp,
		Tags:      []string{"prod", "web"},
	})
	require.NoError(t, err)

	require.NoError(t, repo.UpdateHostTags(ctx, host.ID, []string{"staging"}))

	got, err := repo.GetHost(ctx, host.ID)
	require.NoError(t, err)

	sort.Strings(got.Tags)
	assert.Equal(t, []string{"staging"}, []string(got.Tags))
}

// TestHostTags_AddAppendsDeduplicates verifies that AddHostTags appends new
// tags while deduplicating existing ones.
func TestHostTags_AddAppendsDeduplicates(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()
	ctx := context.Background()

	const ip = "198.51.100.4"
	_, _ = db.ExecContext(ctx, "DELETE FROM hosts WHERE ip_address = $1::inet", ip)

	repo := NewHostRepository(db)
	host, err := repo.CreateHost(ctx, CreateHostInput{
		IPAddress: ip,
		Status:    HostStatusUp,
		Tags:      []string{"prod"},
	})
	require.NoError(t, err)

	require.NoError(t, repo.AddHostTags(ctx, host.ID, []string{"web", "prod"}))

	got, err := repo.GetHost(ctx, host.ID)
	require.NoError(t, err)

	sort.Strings(got.Tags)
	assert.Equal(t, []string{"prod", "web"}, []string(got.Tags))
}

// TestHostTags_RemoveSpecific verifies that RemoveHostTags removes only the
// nominated tags and leaves the rest intact.
func TestHostTags_RemoveSpecific(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()
	ctx := context.Background()

	const ip = "198.51.100.5"
	_, _ = db.ExecContext(ctx, "DELETE FROM hosts WHERE ip_address = $1::inet", ip)

	repo := NewHostRepository(db)
	host, err := repo.CreateHost(ctx, CreateHostInput{
		IPAddress: ip,
		Status:    HostStatusUp,
		Tags:      []string{"prod", "web", "staging"},
	})
	require.NoError(t, err)

	require.NoError(t, repo.RemoveHostTags(ctx, host.ID, []string{"web"}))

	got, err := repo.GetHost(ctx, host.ID)
	require.NoError(t, err)

	sort.Strings(got.Tags)
	assert.Equal(t, []string{"prod", "staging"}, []string(got.Tags))
}

// TestHostTags_GetAllTags verifies that GetAllTags returns the distinct union
// of all tags across all hosts, sorted alphabetically with no duplicates.
func TestHostTags_GetAllTags(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()
	ctx := context.Background()

	// Remove all hosts in the test range to prevent tag contamination from
	// other test functions that may have run previously.
	_, _ = db.ExecContext(ctx, "DELETE FROM hosts WHERE ip_address::text LIKE '198.51.100.%'")

	repo := NewHostRepository(db)

	_, err := repo.CreateHost(ctx, CreateHostInput{
		IPAddress: "198.51.100.6",
		Status:    HostStatusUp,
		Tags:      []string{"prod", "web"},
	})
	require.NoError(t, err)

	_, err = repo.CreateHost(ctx, CreateHostInput{
		IPAddress: "198.51.100.7",
		Status:    HostStatusUp,
		Tags:      []string{"prod", "db"},
	})
	require.NoError(t, err)

	tags, err := repo.GetAllTags(ctx)
	require.NoError(t, err)

	sort.Strings(tags)
	assert.Equal(t, []string{"db", "prod", "web"}, tags)
}

// TestHostTags_BulkSet verifies that BulkUpdateTags with action "set" replaces
// all tags on every supplied host.
func TestHostTags_BulkSet(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()
	ctx := context.Background()

	for _, ip := range []string{"198.51.100.8", "198.51.100.9"} {
		_, _ = db.ExecContext(ctx, "DELETE FROM hosts WHERE ip_address = $1::inet", ip)
	}

	repo := NewHostRepository(db)
	host1, err := repo.CreateHost(ctx, CreateHostInput{
		IPAddress: "198.51.100.8",
		Status:    HostStatusUp,
		Tags:      []string{"old"},
	})
	require.NoError(t, err)

	host2, err := repo.CreateHost(ctx, CreateHostInput{
		IPAddress: "198.51.100.9",
		Status:    HostStatusUp,
		Tags:      []string{"old"},
	})
	require.NoError(t, err)

	ids := []uuid.UUID{host1.ID, host2.ID}
	require.NoError(t, repo.BulkUpdateTags(ctx, ids, []string{"new"}, "set"))

	got1, err := repo.GetHost(ctx, host1.ID)
	require.NoError(t, err)
	sort.Strings(got1.Tags)
	assert.Equal(t, []string{"new"}, []string(got1.Tags))

	got2, err := repo.GetHost(ctx, host2.ID)
	require.NoError(t, err)
	sort.Strings(got2.Tags)
	assert.Equal(t, []string{"new"}, []string(got2.Tags))
}

// TestHostTags_BulkAdd verifies that BulkUpdateTags with action "add" appends
// the given tags to every supplied host.
func TestHostTags_BulkAdd(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()
	ctx := context.Background()

	for _, ip := range []string{"198.51.100.10", "198.51.100.11"} {
		_, _ = db.ExecContext(ctx, "DELETE FROM hosts WHERE ip_address = $1::inet", ip)
	}

	repo := NewHostRepository(db)
	host1, err := repo.CreateHost(ctx, CreateHostInput{
		IPAddress: "198.51.100.10",
		Status:    HostStatusUp,
		Tags:      []string{"base"},
	})
	require.NoError(t, err)

	host2, err := repo.CreateHost(ctx, CreateHostInput{
		IPAddress: "198.51.100.11",
		Status:    HostStatusUp,
		Tags:      []string{"base"},
	})
	require.NoError(t, err)

	ids := []uuid.UUID{host1.ID, host2.ID}
	require.NoError(t, repo.BulkUpdateTags(ctx, ids, []string{"extra"}, "add"))

	got1, err := repo.GetHost(ctx, host1.ID)
	require.NoError(t, err)
	sort.Strings(got1.Tags)
	assert.Equal(t, []string{"base", "extra"}, []string(got1.Tags))

	got2, err := repo.GetHost(ctx, host2.ID)
	require.NoError(t, err)
	sort.Strings(got2.Tags)
	assert.Equal(t, []string{"base", "extra"}, []string(got2.Tags))
}

// TestHostTags_BulkRemove verifies that BulkUpdateTags with action "remove"
// strips only the nominated tags from every supplied host.
func TestHostTags_BulkRemove(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()
	ctx := context.Background()

	for _, ip := range []string{"198.51.100.12", "198.51.100.13"} {
		_, _ = db.ExecContext(ctx, "DELETE FROM hosts WHERE ip_address = $1::inet", ip)
	}

	repo := NewHostRepository(db)
	host1, err := repo.CreateHost(ctx, CreateHostInput{
		IPAddress: "198.51.100.12",
		Status:    HostStatusUp,
		Tags:      []string{"keep", "remove-me"},
	})
	require.NoError(t, err)

	host2, err := repo.CreateHost(ctx, CreateHostInput{
		IPAddress: "198.51.100.13",
		Status:    HostStatusUp,
		Tags:      []string{"keep", "remove-me"},
	})
	require.NoError(t, err)

	ids := []uuid.UUID{host1.ID, host2.ID}
	require.NoError(t, repo.BulkUpdateTags(ctx, ids, []string{"remove-me"}, "remove"))

	got1, err := repo.GetHost(ctx, host1.ID)
	require.NoError(t, err)
	sort.Strings(got1.Tags)
	assert.Equal(t, []string{"keep"}, []string(got1.Tags))

	got2, err := repo.GetHost(ctx, host2.ID)
	require.NoError(t, err)
	sort.Strings(got2.Tags)
	assert.Equal(t, []string{"keep"}, []string(got2.Tags))
}

// TestHostTags_UpdateNotFound verifies that UpdateHostTags returns a non-nil
// error when the host ID does not exist.
func TestHostTags_UpdateNotFound(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()
	ctx := context.Background()

	repo := NewHostRepository(db)
	err := repo.UpdateHostTags(ctx, uuid.New(), []string{"tag"})
	assert.Error(t, err)
}

// ── GetHostGroups tests ────────────────────────────────────────────────────────

// TestGetHostGroups_NoGroups verifies that a host with no group memberships
// returns an empty (not errored) result from GetHostGroups.
func TestGetHostGroups_NoGroups(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()
	ctx := context.Background()

	const ip = "198.51.100.14"
	_, _ = db.ExecContext(ctx, "DELETE FROM hosts WHERE ip_address = $1::inet", ip)

	hostRepo := NewHostRepository(db)
	host, err := hostRepo.CreateHost(ctx, CreateHostInput{
		IPAddress: ip,
		Status:    HostStatusUp,
	})
	require.NoError(t, err)

	groups, err := hostRepo.GetHostGroups(ctx, host.ID)
	require.NoError(t, err)
	assert.Empty(t, groups)
}

// TestGetHostGroups_MultipleGroups verifies that GetHostGroups returns all
// group summaries for a host that belongs to more than one group.
func TestGetHostGroups_MultipleGroups(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()
	ctx := context.Background()

	const ip = "198.51.100.15"
	_, _ = db.ExecContext(ctx, "DELETE FROM hosts WHERE ip_address = $1::inet", ip)
	for _, name := range []string{"host-groups-test-1", "host-groups-test-2"} {
		_, _ = db.ExecContext(ctx, "DELETE FROM host_groups WHERE name = $1", name)
	}

	hostRepo := NewHostRepository(db)
	groupRepo := NewGroupRepository(db)

	host, err := hostRepo.CreateHost(ctx, CreateHostInput{
		IPAddress: ip,
		Status:    HostStatusUp,
	})
	require.NoError(t, err)

	g1, err := groupRepo.CreateGroup(ctx, CreateGroupInput{Name: "host-groups-test-1"})
	require.NoError(t, err)

	g2, err := groupRepo.CreateGroup(ctx, CreateGroupInput{Name: "host-groups-test-2"})
	require.NoError(t, err)

	require.NoError(t, groupRepo.AddHostsToGroup(ctx, g1.ID, []uuid.UUID{host.ID}))
	require.NoError(t, groupRepo.AddHostsToGroup(ctx, g2.ID, []uuid.UUID{host.ID}))

	groups, err := hostRepo.GetHostGroups(ctx, host.ID)
	require.NoError(t, err)
	require.Len(t, groups, 2)

	seen := make(map[uuid.UUID]bool, 2)
	for _, g := range groups {
		seen[g.ID] = true
	}
	assert.True(t, seen[g1.ID], "expected group 1 in host groups")
	assert.True(t, seen[g2.ID], "expected group 2 in host groups")
}

// ── Group repository tests ─────────────────────────────────────────────────────

// TestGroupRepo_CreateAndGet verifies basic group creation and retrieval,
// asserting all supplied fields and a zero MemberCount.
func TestGroupRepo_CreateAndGet(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()
	ctx := context.Background()

	_, _ = db.ExecContext(ctx, "DELETE FROM host_groups WHERE name = $1", "engineering")

	repo := NewGroupRepository(db)
	g, err := repo.CreateGroup(ctx, CreateGroupInput{
		Name:        "engineering",
		Description: "eng team",
		Color:       "#3b82f6",
	})
	require.NoError(t, err)
	require.NotNil(t, g)

	got, err := repo.GetGroup(ctx, g.ID)
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, got.ID)
	assert.Equal(t, "engineering", got.Name)
	assert.Equal(t, "eng team", got.Description)
	assert.Equal(t, "#3b82f6", got.Color)
	assert.Equal(t, 0, got.MemberCount)
}

// TestGroupRepo_DuplicateNameFails verifies that creating two groups with the
// same name returns an error on the second attempt.
func TestGroupRepo_DuplicateNameFails(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()
	ctx := context.Background()

	_, _ = db.ExecContext(ctx, "DELETE FROM host_groups WHERE name = $1", "dup-group-xyz")

	repo := NewGroupRepository(db)
	_, err := repo.CreateGroup(ctx, CreateGroupInput{Name: "dup-group-xyz"})
	require.NoError(t, err)

	_, err = repo.CreateGroup(ctx, CreateGroupInput{Name: "dup-group-xyz"})
	require.Error(t, err)
}

// TestGroupRepo_ListOrderedByName verifies that ListGroups returns groups in
// alphabetical order by name.
func TestGroupRepo_ListOrderedByName(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()
	ctx := context.Background()

	for _, name := range []string{"zebra", "alpha", "mango"} {
		_, _ = db.ExecContext(ctx, "DELETE FROM host_groups WHERE name = $1", name)
	}

	repo := NewGroupRepository(db)
	for _, name := range []string{"zebra", "alpha", "mango"} {
		_, err := repo.CreateGroup(ctx, CreateGroupInput{Name: name})
		require.NoError(t, err)
	}

	groups, err := repo.ListGroups(ctx)
	require.NoError(t, err)

	// Filter the global list to only the names created in this test so that
	// leftover rows from other tests do not interfere with the ordering check.
	wantSet := map[string]bool{"zebra": true, "alpha": true, "mango": true}
	var foundNames []string
	for _, g := range groups {
		if wantSet[g.Name] {
			foundNames = append(foundNames, g.Name)
		}
	}
	assert.Equal(t, []string{"alpha", "mango", "zebra"}, foundNames)
}

// TestGroupRepo_UpdateFields verifies that UpdateGroup persists the new name
// and description and that GetGroup returns the updated values.
func TestGroupRepo_UpdateFields(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()
	ctx := context.Background()

	for _, name := range []string{"update-test-group", "update-test-group-new"} {
		_, _ = db.ExecContext(ctx, "DELETE FROM host_groups WHERE name = $1", name)
	}

	repo := NewGroupRepository(db)
	g, err := repo.CreateGroup(ctx, CreateGroupInput{
		Name:        "update-test-group",
		Description: "original desc",
	})
	require.NoError(t, err)

	newName := "update-test-group-new"
	newDesc := "updated desc"
	updated, err := repo.UpdateGroup(ctx, g.ID, UpdateGroupInput{
		Name:        &newName,
		Description: &newDesc,
	})
	require.NoError(t, err)
	assert.Equal(t, "update-test-group-new", updated.Name)
	assert.Equal(t, "updated desc", updated.Description)

	got, err := repo.GetGroup(ctx, g.ID)
	require.NoError(t, err)
	assert.Equal(t, "update-test-group-new", got.Name)
	assert.Equal(t, "updated desc", got.Description)
}

// TestGroupRepo_DeleteRemovesGroup verifies that a deleted group can no longer
// be retrieved via GetGroup.
func TestGroupRepo_DeleteRemovesGroup(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()
	ctx := context.Background()

	_, _ = db.ExecContext(ctx, "DELETE FROM host_groups WHERE name = $1", "delete-test-group")

	repo := NewGroupRepository(db)
	g, err := repo.CreateGroup(ctx, CreateGroupInput{Name: "delete-test-group"})
	require.NoError(t, err)

	require.NoError(t, repo.DeleteGroup(ctx, g.ID))

	_, err = repo.GetGroup(ctx, g.ID)
	require.Error(t, err)
}

// TestGroupRepo_DeleteNonExistentFails verifies that attempting to delete a
// group with an unknown UUID returns an error.
func TestGroupRepo_DeleteNonExistentFails(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()
	ctx := context.Background()

	repo := NewGroupRepository(db)
	err := repo.DeleteGroup(ctx, uuid.New())
	require.Error(t, err)
}

// TestGroupRepo_AddMembersAndCount verifies that after adding three hosts to a
// group GetGroupMembers reports a total of three.
func TestGroupRepo_AddMembersAndCount(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()
	ctx := context.Background()

	for _, ip := range []string{"198.51.100.16", "198.51.100.17", "198.51.100.18"} {
		_, _ = db.ExecContext(ctx, "DELETE FROM hosts WHERE ip_address = $1::inet", ip)
	}
	_, _ = db.ExecContext(ctx, "DELETE FROM host_groups WHERE name = $1", "count-test-group")

	hostRepo := NewHostRepository(db)
	groupRepo := NewGroupRepository(db)

	hosts := make([]*Host, 3)
	for i, ip := range []string{"198.51.100.16", "198.51.100.17", "198.51.100.18"} {
		h, err := hostRepo.CreateHost(ctx, CreateHostInput{
			IPAddress: ip,
			Status:    HostStatusUp,
		})
		require.NoError(t, err)
		hosts[i] = h
	}

	g, err := groupRepo.CreateGroup(ctx, CreateGroupInput{Name: "count-test-group"})
	require.NoError(t, err)

	ids := []uuid.UUID{hosts[0].ID, hosts[1].ID, hosts[2].ID}
	require.NoError(t, groupRepo.AddHostsToGroup(ctx, g.ID, ids))

	_, total, err := groupRepo.GetGroupMembers(ctx, g.ID, 0, 100)
	require.NoError(t, err)
	assert.Equal(t, int64(3), total)
}

// TestGroupRepo_AddMembers_Idempotent verifies that adding the same host to a
// group twice results in exactly one membership row.
func TestGroupRepo_AddMembers_Idempotent(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()
	ctx := context.Background()

	const ip = "198.51.100.19"
	_, _ = db.ExecContext(ctx, "DELETE FROM hosts WHERE ip_address = $1::inet", ip)
	_, _ = db.ExecContext(ctx, "DELETE FROM host_groups WHERE name = $1", "idempotent-test-group")

	hostRepo := NewHostRepository(db)
	groupRepo := NewGroupRepository(db)

	host, err := hostRepo.CreateHost(ctx, CreateHostInput{
		IPAddress: ip,
		Status:    HostStatusUp,
	})
	require.NoError(t, err)

	g, err := groupRepo.CreateGroup(ctx, CreateGroupInput{Name: "idempotent-test-group"})
	require.NoError(t, err)

	// Add the same host twice; the second call must not error.
	require.NoError(t, groupRepo.AddHostsToGroup(ctx, g.ID, []uuid.UUID{host.ID}))
	require.NoError(t, groupRepo.AddHostsToGroup(ctx, g.ID, []uuid.UUID{host.ID}))

	_, total, err := groupRepo.GetGroupMembers(ctx, g.ID, 0, 100)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
}

// TestGroupRepo_RemoveMember verifies that removing one host from a group
// leaves the remaining two members intact.
func TestGroupRepo_RemoveMember(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()
	ctx := context.Background()

	for _, ip := range []string{"198.51.100.20", "198.51.100.21", "198.51.100.22"} {
		_, _ = db.ExecContext(ctx, "DELETE FROM hosts WHERE ip_address = $1::inet", ip)
	}
	_, _ = db.ExecContext(ctx, "DELETE FROM host_groups WHERE name = $1", "remove-member-group")

	hostRepo := NewHostRepository(db)
	groupRepo := NewGroupRepository(db)

	hosts := make([]*Host, 3)
	for i, ip := range []string{"198.51.100.20", "198.51.100.21", "198.51.100.22"} {
		h, err := hostRepo.CreateHost(ctx, CreateHostInput{
			IPAddress: ip,
			Status:    HostStatusUp,
		})
		require.NoError(t, err)
		hosts[i] = h
	}

	g, err := groupRepo.CreateGroup(ctx, CreateGroupInput{Name: "remove-member-group"})
	require.NoError(t, err)

	ids := []uuid.UUID{hosts[0].ID, hosts[1].ID, hosts[2].ID}
	require.NoError(t, groupRepo.AddHostsToGroup(ctx, g.ID, ids))

	require.NoError(t, groupRepo.RemoveHostsFromGroup(ctx, g.ID, []uuid.UUID{hosts[0].ID}))

	_, total, err := groupRepo.GetGroupMembers(ctx, g.ID, 0, 100)
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
}

// TestGroupRepo_MemberCountInList verifies that ListGroups returns accurate
// MemberCount values for each group.
func TestGroupRepo_MemberCountInList(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()
	ctx := context.Background()

	for _, ip := range []string{"198.51.100.23", "198.51.100.24", "198.51.100.25", "198.51.100.26"} {
		_, _ = db.ExecContext(ctx, "DELETE FROM hosts WHERE ip_address = $1::inet", ip)
	}
	for _, name := range []string{"count-list-group-1", "count-list-group-2"} {
		_, _ = db.ExecContext(ctx, "DELETE FROM host_groups WHERE name = $1", name)
	}

	hostRepo := NewHostRepository(db)
	groupRepo := NewGroupRepository(db)

	hosts := make([]*Host, 4)
	for i, ip := range []string{"198.51.100.23", "198.51.100.24", "198.51.100.25", "198.51.100.26"} {
		h, err := hostRepo.CreateHost(ctx, CreateHostInput{
			IPAddress: ip,
			Status:    HostStatusUp,
		})
		require.NoError(t, err)
		hosts[i] = h
	}

	g1, err := groupRepo.CreateGroup(ctx, CreateGroupInput{Name: "count-list-group-1"})
	require.NoError(t, err)

	g2, err := groupRepo.CreateGroup(ctx, CreateGroupInput{Name: "count-list-group-2"})
	require.NoError(t, err)

	// 3 hosts into g1, 1 host into g2.
	require.NoError(t, groupRepo.AddHostsToGroup(ctx, g1.ID,
		[]uuid.UUID{hosts[0].ID, hosts[1].ID, hosts[2].ID}))
	require.NoError(t, groupRepo.AddHostsToGroup(ctx, g2.ID,
		[]uuid.UUID{hosts[3].ID}))

	groups, err := groupRepo.ListGroups(ctx)
	require.NoError(t, err)

	counts := make(map[uuid.UUID]int)
	for _, g := range groups {
		counts[g.ID] = g.MemberCount
	}
	assert.Equal(t, 3, counts[g1.ID])
	assert.Equal(t, 1, counts[g2.ID])
}

// TestGroupRepo_DeleteGroupDoesNotDeleteHosts verifies that deleting a group
// removes only the membership rows (via CASCADE) and leaves the host records
// themselves untouched.
func TestGroupRepo_DeleteGroupDoesNotDeleteHosts(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()
	ctx := context.Background()

	for _, ip := range []string{"198.51.100.27", "198.51.100.28"} {
		_, _ = db.ExecContext(ctx, "DELETE FROM hosts WHERE ip_address = $1::inet", ip)
	}
	_, _ = db.ExecContext(ctx, "DELETE FROM host_groups WHERE name = $1", "delete-group-no-host-delete")

	hostRepo := NewHostRepository(db)
	groupRepo := NewGroupRepository(db)

	host1, err := hostRepo.CreateHost(ctx, CreateHostInput{
		IPAddress: "198.51.100.27",
		Status:    HostStatusUp,
	})
	require.NoError(t, err)

	host2, err := hostRepo.CreateHost(ctx, CreateHostInput{
		IPAddress: "198.51.100.28",
		Status:    HostStatusUp,
	})
	require.NoError(t, err)

	g, err := groupRepo.CreateGroup(ctx, CreateGroupInput{Name: "delete-group-no-host-delete"})
	require.NoError(t, err)

	require.NoError(t, groupRepo.AddHostsToGroup(ctx, g.ID, []uuid.UUID{host1.ID, host2.ID}))
	require.NoError(t, groupRepo.DeleteGroup(ctx, g.ID))

	// Both host records must still be accessible after the group is gone.
	got1, err := hostRepo.GetHost(ctx, host1.ID)
	require.NoError(t, err)
	assert.Equal(t, host1.ID, got1.ID)

	got2, err := hostRepo.GetHost(ctx, host2.ID)
	require.NoError(t, err)
	assert.Equal(t, host2.ID, got2.ID)
}
