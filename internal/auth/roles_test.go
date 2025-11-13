// Package auth provides comprehensive unit tests for RBAC roles and permissions.
// This file tests role creation, validation, permission checking, and role management
// with various edge cases and security scenarios.
package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateRoleName(t *testing.T) {
	tests := []struct {
		name        string
		roleName    string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid_name",
			roleName:    "admin",
			expectError: false,
		},
		{
			name:        "valid_name_with_underscore",
			roleName:    "read_only",
			expectError: false,
		},
		{
			name:        "valid_name_with_hyphen",
			roleName:    "read-only",
			expectError: false,
		},
		{
			name:        "valid_name_with_numbers",
			roleName:    "operator123",
			expectError: false,
		},
		{
			name:        "valid_mixed_case",
			roleName:    "ReadOnly",
			expectError: false,
		},
		{
			name:        "valid_complex_name",
			roleName:    "My_Role-123",
			expectError: false,
		},
		{
			name:        "empty_name",
			roleName:    "",
			expectError: true,
			errorMsg:    "role name cannot be empty",
		},
		{
			name:        "too_long_name",
			roleName:    strings.Repeat("a", 101),
			expectError: true,
			errorMsg:    "role name must be at most 100 characters",
		},
		{
			name:        "starts_with_number",
			roleName:    "123admin",
			expectError: true,
			errorMsg:    "must start with a letter",
		},
		{
			name:        "starts_with_underscore",
			roleName:    "_admin",
			expectError: true,
			errorMsg:    "must start with a letter",
		},
		{
			name:        "starts_with_hyphen",
			roleName:    "-admin",
			expectError: true,
			errorMsg:    "must start with a letter",
		},
		{
			name:        "contains_spaces",
			roleName:    "read only",
			expectError: true,
			errorMsg:    "must start with a letter",
		},
		{
			name:        "contains_special_chars",
			roleName:    "admin@role",
			expectError: true,
			errorMsg:    "must start with a letter",
		},
		{
			name:        "contains_dots",
			roleName:    "admin.role",
			expectError: true,
			errorMsg:    "must start with a letter",
		},
		{
			name:        "single_character",
			roleName:    "a",
			expectError: false,
		},
		{
			name:        "max_length",
			roleName:    "a" + strings.Repeat("b", 99),
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRoleName(tt.roleName)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateRoleDescription(t *testing.T) {
	tests := []struct {
		name        string
		description string
		expectError bool
	}{
		{
			name:        "valid_description",
			description: "This is a valid role description",
			expectError: false,
		},
		{
			name:        "empty_description",
			description: "",
			expectError: false,
		},
		{
			name:        "max_length_description",
			description: strings.Repeat("a", 1000),
			expectError: false,
		},
		{
			name:        "too_long_description",
			description: strings.Repeat("a", 1001),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRoleDescription(tt.description)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestNewRole(t *testing.T) {
	tests := []struct {
		name        string
		roleName    string
		description string
		permissions map[string]interface{}
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid_role",
			roleName:    "admin",
			description: "Administrator role",
			permissions: map[string]interface{}{
				ResourceAll: []interface{}{PermissionAdmin},
			},
			expectError: false,
		},
		{
			name:        "valid_role_nil_permissions",
			roleName:    "readonly",
			description: "Read-only role",
			permissions: nil,
			expectError: false,
		},
		{
			name:        "invalid_role_name",
			roleName:    "123admin",
			description: "Invalid role",
			permissions: nil,
			expectError: true,
			errorMsg:    "invalid role name",
		},
		{
			name:        "invalid_description",
			roleName:    "admin",
			description: strings.Repeat("a", 1001),
			permissions: nil,
			expectError: true,
			errorMsg:    "invalid role description",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			role, err := NewRole(tt.roleName, tt.description, tt.permissions)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, role)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, role)
				assert.Equal(t, tt.roleName, role.Name)
				assert.Equal(t, tt.description, role.Description)
				assert.True(t, role.IsActive)
				assert.False(t, role.IsSystem)
				assert.NotNil(t, role.Permissions)

				if tt.permissions != nil {
					assert.Equal(t, tt.permissions, role.Permissions)
				} else {
					assert.Empty(t, role.Permissions)
				}
			}
		})
	}
}

func TestRole_HasPermission(t *testing.T) {
	tests := []struct {
		name        string
		role        *Role
		resource    string
		action      string
		expected    bool
		description string
	}{
		{
			name: "admin_has_all_permissions",
			role: &Role{
				Name:     RoleAdmin,
				IsActive: true,
				Permissions: map[string]interface{}{
					ResourceAll: []interface{}{PermissionAdmin},
				},
			},
			resource:    ResourceScans,
			action:      PermissionWrite,
			expected:    true,
			description: "Admin role should have all permissions",
		},
		{
			name: "readonly_has_read_permission",
			role: &Role{
				Name:     RoleReadonly,
				IsActive: true,
				Permissions: map[string]interface{}{
					ResourceScans: []interface{}{PermissionRead},
				},
			},
			resource:    ResourceScans,
			action:      PermissionRead,
			expected:    true,
			description: "Readonly role should have read permission",
		},
		{
			name: "readonly_no_write_permission",
			role: &Role{
				Name:     RoleReadonly,
				IsActive: true,
				Permissions: map[string]interface{}{
					ResourceScans: []interface{}{PermissionRead},
				},
			},
			resource:    ResourceScans,
			action:      PermissionWrite,
			expected:    false,
			description: "Readonly role should not have write permission",
		},
		{
			name: "inactive_role_no_permissions",
			role: &Role{
				Name:     RoleAdmin,
				IsActive: false,
				Permissions: map[string]interface{}{
					ResourceAll: []interface{}{PermissionAdmin},
				},
			},
			resource:    ResourceScans,
			action:      PermissionRead,
			expected:    false,
			description: "Inactive role should not have any permissions",
		},
		{
			name: "specific_resource_permission",
			role: &Role{
				Name:     RoleOperator,
				IsActive: true,
				Permissions: map[string]interface{}{
					ResourceScans: []interface{}{PermissionRead, PermissionWrite},
					ResourceHosts: []interface{}{PermissionRead},
				},
			},
			resource:    ResourceScans,
			action:      PermissionWrite,
			expected:    true,
			description: "Role should have write permission on scans",
		},
		{
			name: "no_permission_on_different_resource",
			role: &Role{
				Name:     RoleOperator,
				IsActive: true,
				Permissions: map[string]interface{}{
					ResourceScans: []interface{}{PermissionRead, PermissionWrite},
				},
			},
			resource:    ResourceHosts,
			action:      PermissionRead,
			expected:    false,
			description: "Role should not have permission on different resource",
		},
		{
			name: "wildcard_action_permission",
			role: &Role{
				Name:     "custom",
				IsActive: true,
				Permissions: map[string]interface{}{
					ResourceScans: []interface{}{PermissionAdmin},
				},
			},
			resource:    ResourceScans,
			action:      PermissionDelete,
			expected:    true,
			description: "Wildcard action should grant any permission",
		},
		{
			name: "empty_permissions",
			role: &Role{
				Name:        "empty",
				IsActive:    true,
				Permissions: map[string]interface{}{},
			},
			resource:    ResourceScans,
			action:      PermissionRead,
			expected:    false,
			description: "Empty permissions should not grant access",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.role.HasPermission(tt.resource, tt.action)
			assert.Equal(t, tt.expected, result, tt.description)
		})
	}
}

func TestRole_GetPermissionsSummary(t *testing.T) {
	tests := []struct {
		name        string
		role        *Role
		expected    int
		contains    []string
		description string
	}{
		{
			name: "admin_permissions",
			role: &Role{
				Name: RoleAdmin,
				Permissions: map[string]interface{}{
					ResourceAll: []interface{}{PermissionAdmin},
				},
			},
			expected:    1,
			contains:    []string{"*: *"},
			description: "Admin should show wildcard permissions",
		},
		{
			name: "multiple_resources",
			role: &Role{
				Name: RoleOperator,
				Permissions: map[string]interface{}{
					ResourceScans: []interface{}{PermissionRead, PermissionWrite},
					ResourceHosts: []interface{}{PermissionRead},
				},
			},
			expected:    2,
			contains:    []string{"scans:", "hosts:"},
			description: "Should list all resource permissions",
		},
		{
			name: "empty_permissions",
			role: &Role{
				Name:        "empty",
				Permissions: map[string]interface{}{},
			},
			expected:    0,
			description: "Empty permissions should return empty summary",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary := tt.role.GetPermissionsSummary()
			assert.Len(t, summary, tt.expected, tt.description)

			for _, expectedStr := range tt.contains {
				found := false
				for _, s := range summary {
					if strings.Contains(s, expectedStr) {
						found = true
						break
					}
				}
				assert.True(t, found, "Expected summary to contain: %s", expectedStr)
			}
		})
	}
}

func TestRole_IsSystemRole(t *testing.T) {
	tests := []struct {
		name     string
		role     *Role
		expected bool
	}{
		{
			name: "system_role",
			role: &Role{
				Name:     RoleAdmin,
				IsSystem: true,
			},
			expected: true,
		},
		{
			name: "custom_role",
			role: &Role{
				Name:     "custom",
				IsSystem: false,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.role.IsSystemRole()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRole_CanBeDeleted(t *testing.T) {
	tests := []struct {
		name     string
		role     *Role
		expected bool
	}{
		{
			name: "system_role_cannot_be_deleted",
			role: &Role{
				Name:     RoleAdmin,
				IsSystem: true,
				IsActive: true,
			},
			expected: false,
		},
		{
			name: "inactive_role_cannot_be_deleted",
			role: &Role{
				Name:     "custom",
				IsSystem: false,
				IsActive: false,
			},
			expected: false,
		},
		{
			name: "custom_active_role_can_be_deleted",
			role: &Role{
				Name:     "custom",
				IsSystem: false,
				IsActive: true,
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.role.CanBeDeleted()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDefaultRoles(t *testing.T) {
	roles := DefaultRoles()

	// Should have exactly 3 default roles
	assert.Len(t, roles, 3)

	// Check that all default roles exist
	roleNames := make(map[string]bool)
	for _, role := range roles {
		roleNames[role.Name] = true
		assert.True(t, role.IsActive)
		assert.True(t, role.IsSystem)
		assert.NotEmpty(t, role.Description)
		assert.NotNil(t, role.Permissions)
	}

	assert.True(t, roleNames[RoleAdmin])
	assert.True(t, roleNames[RoleReadonly])
	assert.True(t, roleNames[RoleOperator])

	// Test admin role
	var adminRole *Role
	for i := range roles {
		if roles[i].Name == RoleAdmin {
			adminRole = &roles[i]
			break
		}
	}
	require.NotNil(t, adminRole)
	assert.True(t, adminRole.HasPermission(ResourceScans, PermissionDelete))
	assert.True(t, adminRole.HasPermission(ResourceHosts, PermissionWrite))
	assert.True(t, adminRole.HasPermission(ResourceNetworks, PermissionAdmin))

	// Test readonly role
	var readonlyRole *Role
	for i := range roles {
		if roles[i].Name == RoleReadonly {
			readonlyRole = &roles[i]
			break
		}
	}
	require.NotNil(t, readonlyRole)
	assert.True(t, readonlyRole.HasPermission(ResourceScans, PermissionRead))
	assert.True(t, readonlyRole.HasPermission(ResourceHosts, PermissionRead))
	assert.True(t, readonlyRole.HasPermission(ResourceNetworks, PermissionRead))
	assert.False(t, readonlyRole.HasPermission(ResourceScans, PermissionWrite))
	assert.False(t, readonlyRole.HasPermission(ResourceHosts, PermissionDelete))

	// Test operator role
	var operatorRole *Role
	for i := range roles {
		if roles[i].Name == RoleOperator {
			operatorRole = &roles[i]
			break
		}
	}
	require.NotNil(t, operatorRole)
	assert.True(t, operatorRole.HasPermission(ResourceScans, PermissionWrite))
	assert.True(t, operatorRole.HasPermission(ResourceDiscovery, PermissionDelete))
	assert.True(t, operatorRole.HasPermission(ResourceHosts, PermissionRead))
	assert.False(t, operatorRole.HasPermission(ResourceHosts, PermissionWrite))
	assert.False(t, operatorRole.HasPermission(ResourceAPIKeys, PermissionRead))
}

func TestCheckPermissions(t *testing.T) {
	adminRole := Role{
		Name:     RoleAdmin,
		IsActive: true,
		Permissions: map[string]interface{}{
			ResourceAll: []interface{}{PermissionAdmin},
		},
	}

	readonlyRole := Role{
		Name:     RoleReadonly,
		IsActive: true,
		Permissions: map[string]interface{}{
			ResourceScans: []interface{}{PermissionRead},
		},
	}

	inactiveRole := Role{
		Name:     "inactive",
		IsActive: false,
		Permissions: map[string]interface{}{
			ResourceAll: []interface{}{PermissionAdmin},
		},
	}

	tests := []struct {
		name     string
		roles    []Role
		resource string
		action   string
		expected bool
	}{
		{
			name:     "admin_has_permission",
			roles:    []Role{adminRole},
			resource: ResourceScans,
			action:   PermissionWrite,
			expected: true,
		},
		{
			name:     "readonly_has_read",
			roles:    []Role{readonlyRole},
			resource: ResourceScans,
			action:   PermissionRead,
			expected: true,
		},
		{
			name:     "readonly_no_write",
			roles:    []Role{readonlyRole},
			resource: ResourceScans,
			action:   PermissionWrite,
			expected: false,
		},
		{
			name:     "multiple_roles_any_grants",
			roles:    []Role{readonlyRole, adminRole},
			resource: ResourceHosts,
			action:   PermissionDelete,
			expected: true,
		},
		{
			name:     "inactive_role_no_permission",
			roles:    []Role{inactiveRole},
			resource: ResourceScans,
			action:   PermissionRead,
			expected: false,
		},
		{
			name:     "empty_roles_no_permission",
			roles:    []Role{},
			resource: ResourceScans,
			action:   PermissionRead,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CheckPermissions(tt.roles, tt.resource, tt.action)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMarshalUnmarshalPermissions(t *testing.T) {
	tests := []struct {
		name        string
		permissions map[string]interface{}
	}{
		{
			name: "simple_permissions",
			permissions: map[string]interface{}{
				ResourceScans: []interface{}{PermissionRead},
			},
		},
		{
			name: "complex_permissions",
			permissions: map[string]interface{}{
				ResourceScans:     []interface{}{PermissionRead, PermissionWrite, PermissionDelete},
				ResourceHosts:     []interface{}{PermissionRead},
				ResourceDiscovery: []interface{}{PermissionAdmin},
			},
		},
		{
			name:        "empty_permissions",
			permissions: map[string]interface{}{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Marshal
			data, err := MarshalPermissions(tt.permissions)
			require.NoError(t, err)
			assert.NotNil(t, data)

			// Unmarshal
			unmarshaled, err := UnmarshalPermissions(data)
			require.NoError(t, err)
			assert.NotNil(t, unmarshaled)

			// Compare using PermissionsEqual
			assert.True(t, PermissionsEqual(tt.permissions, unmarshaled))
		})
	}
}

func TestUnmarshalPermissions_EmptyData(t *testing.T) {
	permissions, err := UnmarshalPermissions([]byte{})
	require.NoError(t, err)
	assert.NotNil(t, permissions)
	assert.Empty(t, permissions)
}

func TestValidatePermissions(t *testing.T) {
	tests := []struct {
		name        string
		permissions map[string]interface{}
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid_permissions",
			permissions: map[string]interface{}{
				ResourceScans: []interface{}{PermissionRead, PermissionWrite},
			},
			expectError: false,
		},
		{
			name: "valid_wildcard",
			permissions: map[string]interface{}{
				ResourceAll: []interface{}{PermissionAdmin},
			},
			expectError: false,
		},
		{
			name: "empty_resource_name",
			permissions: map[string]interface{}{
				"": []interface{}{PermissionRead},
			},
			expectError: true,
			errorMsg:    "resource name cannot be empty",
		},
		{
			name: "non_array_actions",
			permissions: map[string]interface{}{
				ResourceScans: "read",
			},
			expectError: true,
			errorMsg:    "must be an array",
		},
		{
			name: "empty_actions_array",
			permissions: map[string]interface{}{
				ResourceScans: []interface{}{},
			},
			expectError: true,
			errorMsg:    "at least one action required",
		},
		{
			name: "non_string_action",
			permissions: map[string]interface{}{
				ResourceScans: []interface{}{123},
			},
			expectError: true,
			errorMsg:    "action must be a string",
		},
		{
			name: "empty_action_string",
			permissions: map[string]interface{}{
				ResourceScans: []interface{}{""},
			},
			expectError: true,
			errorMsg:    "action cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePermissions(tt.permissions)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestPermissionsEqual(t *testing.T) {
	tests := []struct {
		name     string
		a        map[string]interface{}
		b        map[string]interface{}
		expected bool
	}{
		{
			name: "equal_simple",
			a: map[string]interface{}{
				ResourceScans: []interface{}{PermissionRead},
			},
			b: map[string]interface{}{
				ResourceScans: []interface{}{PermissionRead},
			},
			expected: true,
		},
		{
			name: "equal_complex",
			a: map[string]interface{}{
				ResourceScans: []interface{}{PermissionRead, PermissionWrite},
				ResourceHosts: []interface{}{PermissionRead},
			},
			b: map[string]interface{}{
				ResourceScans: []interface{}{PermissionRead, PermissionWrite},
				ResourceHosts: []interface{}{PermissionRead},
			},
			expected: true,
		},
		{
			name: "different_actions",
			a: map[string]interface{}{
				ResourceScans: []interface{}{PermissionRead},
			},
			b: map[string]interface{}{
				ResourceScans: []interface{}{PermissionWrite},
			},
			expected: false,
		},
		{
			name: "different_resources",
			a: map[string]interface{}{
				ResourceScans: []interface{}{PermissionRead},
			},
			b: map[string]interface{}{
				ResourceHosts: []interface{}{PermissionRead},
			},
			expected: false,
		},
		{
			name: "different_length",
			a: map[string]interface{}{
				ResourceScans: []interface{}{PermissionRead},
			},
			b: map[string]interface{}{
				ResourceScans: []interface{}{PermissionRead},
				ResourceHosts: []interface{}{PermissionRead},
			},
			expected: false,
		},
		{
			name:     "both_empty",
			a:        map[string]interface{}{},
			b:        map[string]interface{}{},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := PermissionsEqual(tt.a, tt.b)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRoleConstants(t *testing.T) {
	// Test permission actions
	assert.Equal(t, "read", PermissionRead)
	assert.Equal(t, "write", PermissionWrite)
	assert.Equal(t, "delete", PermissionDelete)
	assert.Equal(t, "*", PermissionAdmin)

	// Test resources
	assert.Equal(t, "*", ResourceAll)
	assert.Equal(t, "scans", ResourceScans)
	assert.Equal(t, "hosts", ResourceHosts)
	assert.Equal(t, "networks", ResourceNetworks)
	assert.Equal(t, "profiles", ResourceProfiles)
	assert.Equal(t, "discovery", ResourceDiscovery)
	assert.Equal(t, "apikeys", ResourceAPIKeys)
	assert.Equal(t, "admin", ResourceAdmin)

	// Test role names
	assert.Equal(t, "admin", RoleAdmin)
	assert.Equal(t, "readonly", RoleReadonly)
	assert.Equal(t, "operator", RoleOperator)

	// Test validation constants
	assert.Equal(t, 1, MinRoleNameLength)
	assert.Equal(t, 100, MaxRoleNameLength)
	assert.Equal(t, 1000, MaxRoleDescLength)
}

func TestRole_Integration(t *testing.T) {
	// Create a custom role
	permissions := map[string]interface{}{
		ResourceScans:     []interface{}{PermissionRead, PermissionWrite},
		ResourceDiscovery: []interface{}{PermissionRead},
	}

	role, err := NewRole("scanner", "Scanner role with limited permissions", permissions)
	require.NoError(t, err)
	require.NotNil(t, role)

	// Test permissions
	assert.True(t, role.HasPermission(ResourceScans, PermissionRead))
	assert.True(t, role.HasPermission(ResourceScans, PermissionWrite))
	assert.False(t, role.HasPermission(ResourceScans, PermissionDelete))
	assert.True(t, role.HasPermission(ResourceDiscovery, PermissionRead))
	assert.False(t, role.HasPermission(ResourceDiscovery, PermissionWrite))
	assert.False(t, role.HasPermission(ResourceHosts, PermissionRead))

	// Test summary
	summary := role.GetPermissionsSummary()
	assert.NotEmpty(t, summary)

	// Test role properties
	assert.True(t, role.IsActive)
	assert.False(t, role.IsSystem)
	assert.True(t, role.CanBeDeleted())

	// Test marshaling
	data, err := MarshalPermissions(role.Permissions)
	require.NoError(t, err)

	unmarshaled, err := UnmarshalPermissions(data)
	require.NoError(t, err)
	assert.True(t, PermissionsEqual(role.Permissions, unmarshaled))

	// Test validation
	assert.NoError(t, ValidatePermissions(role.Permissions))

	// Deactivate role
	role.IsActive = false
	assert.False(t, role.HasPermission(ResourceScans, PermissionRead))
	assert.False(t, role.CanBeDeleted())
}

func TestAPIKeyWithRoles(t *testing.T) {
	// Test struct creation
	keyInfo := APIKeyInfo{
		ID:        "test-key-1",
		Name:      "Test Key",
		IsActive:  true,
		CreatedAt: time.Now().UTC(),
	}

	roles := DefaultRoles()

	keyWithRoles := APIKeyWithRoles{
		APIKeyInfo: keyInfo,
		Roles:      roles,
	}

	assert.Equal(t, keyInfo.ID, keyWithRoles.ID)
	assert.Equal(t, keyInfo.Name, keyWithRoles.Name)
	assert.Len(t, keyWithRoles.Roles, 3)
}

// Benchmark tests
func BenchmarkRole_HasPermission(b *testing.B) {
	role := &Role{
		Name:     RoleAdmin,
		IsActive: true,
		Permissions: map[string]interface{}{
			ResourceAll: []interface{}{PermissionAdmin},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		role.HasPermission(ResourceScans, PermissionRead)
	}
}

func BenchmarkCheckPermissions(b *testing.B) {
	roles := DefaultRoles()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CheckPermissions(roles, ResourceScans, PermissionWrite)
	}
}

func BenchmarkValidatePermissions(b *testing.B) {
	permissions := map[string]interface{}{
		ResourceScans:     []interface{}{PermissionRead, PermissionWrite, PermissionDelete},
		ResourceHosts:     []interface{}{PermissionRead},
		ResourceNetworks:  []interface{}{PermissionRead, PermissionWrite},
		ResourceDiscovery: []interface{}{PermissionRead},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ValidatePermissions(permissions)
	}
}
