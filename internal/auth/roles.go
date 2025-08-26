// Package auth provides role-based access control (RBAC) structures and utilities.
// This file implements the Role structure and permission management for the Scanorama API.
package auth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Permission actions
const (
	PermissionRead   = "read"
	PermissionWrite  = "write"
	PermissionDelete = "delete"
	PermissionAdmin  = "*" // Wildcard for all actions
)

// Resource types
const (
	ResourceAll       = "*" // Wildcard for all resources
	ResourceScans     = "scans"
	ResourceHosts     = "hosts"
	ResourceNetworks  = "networks"
	ResourceProfiles  = "profiles"
	ResourceDiscovery = "discovery"
	ResourceAPIKeys   = "apikeys"
	ResourceAdmin     = "admin"
)

// System role names
const (
	RoleAdmin    = "admin"
	RoleReadonly = "readonly"
	RoleOperator = "operator"
)

// Role validation constants
const (
	MinRoleNameLength = 1
	MaxRoleNameLength = 100
	MaxRoleDescLength = 1000
)

// Role represents a role in the RBAC system
type Role struct {
	ID          string                 `json:"id" db:"id"`
	Name        string                 `json:"name" db:"name" validate:"required,min=1,max=100"`
	Description string                 `json:"description,omitempty" db:"description"`
	Permissions map[string]interface{} `json:"permissions" db:"permissions"`
	IsActive    bool                   `json:"is_active" db:"is_active"`
	IsSystem    bool                   `json:"is_system" db:"is_system"`
	CreatedAt   time.Time              `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at" db:"updated_at"`
	CreatedBy   *string                `json:"created_by,omitempty" db:"created_by"`
}

// Permission represents a structured permission
type Permission struct {
	Resource string   `json:"resource"`
	Actions  []string `json:"actions"`
}

// RolePermissions represents the structured permissions for a role
type RolePermissions struct {
	Permissions []Permission `json:"permissions"`
}

// APIKeyWithRoles represents an API key with its associated roles
type APIKeyWithRoles struct {
	APIKeyInfo
	Roles []Role `json:"roles"`
}

// ValidateRoleName validates a role name format
func ValidateRoleName(name string) error {
	if name == "" {
		return fmt.Errorf("role name cannot be empty")
	}

	if len(name) < MinRoleNameLength {
		return fmt.Errorf("role name must be at least %d characters", MinRoleNameLength)
	}

	if len(name) > MaxRoleNameLength {
		return fmt.Errorf("role name must be at most %d characters", MaxRoleNameLength)
	}

	// Check format: alphanumeric, underscore, hyphen, must start with letter
	matched, err := regexp.MatchString(`^[a-zA-Z][a-zA-Z0-9_-]*$`, name)
	if err != nil {
		return fmt.Errorf("error validating role name format: %w", err)
	}
	if !matched {
		return fmt.Errorf("role name must start with a letter and contain only alphanumeric " +
			"characters, underscores, and hyphens")
	}

	return nil
}

// ValidateRoleDescription validates a role description
func ValidateRoleDescription(description string) error {
	if len(description) > MaxRoleDescLength {
		return fmt.Errorf("role description must be at most %d characters", MaxRoleDescLength)
	}
	return nil
}

// NewRole creates a new role with validation
func NewRole(name, description string, permissions map[string]interface{}) (*Role, error) {
	if err := ValidateRoleName(name); err != nil {
		return nil, fmt.Errorf("invalid role name: %w", err)
	}

	if err := ValidateRoleDescription(description); err != nil {
		return nil, fmt.Errorf("invalid role description: %w", err)
	}

	if permissions == nil {
		permissions = make(map[string]interface{})
	}

	return &Role{
		Name:        name,
		Description: description,
		Permissions: permissions,
		IsActive:    true,
		IsSystem:    false,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}, nil
}

// HasPermission checks if the role has a specific permission
func (r *Role) HasPermission(resource, action string) bool {
	if !r.IsActive {
		return false
	}

	// Check for wildcard permissions (admin role)
	if r.hasWildcardPermission() {
		return true
	}

	// Check specific resource permissions
	return r.hasResourcePermission(resource, action)
}

// hasWildcardPermission checks if the role has admin permissions for all resources
func (r *Role) hasWildcardPermission() bool {
	actions, exists := r.Permissions[ResourceAll]
	if !exists {
		return false
	}

	actionList, ok := actions.([]interface{})
	if !ok {
		return false
	}

	for _, a := range actionList {
		if a == PermissionAdmin {
			return true
		}
	}
	return false
}

// hasResourcePermission checks if the role has specific permission for a resource
func (r *Role) hasResourcePermission(resource, action string) bool {
	actions, exists := r.Permissions[resource]
	if !exists {
		return false
	}

	actionList, ok := actions.([]interface{})
	if !ok {
		return false
	}

	for _, a := range actionList {
		actionStr, ok := a.(string)
		if !ok {
			continue
		}
		if actionStr == action || actionStr == PermissionAdmin {
			return true
		}
	}
	return false
}

// GetPermissionsSummary returns a human-readable summary of permissions
func (r *Role) GetPermissionsSummary() []string {
	var summary []string

	for resource, actions := range r.Permissions {
		if actionList, ok := actions.([]interface{}); ok {
			var actionStrings []string
			for _, action := range actionList {
				if actionStr, ok := action.(string); ok {
					actionStrings = append(actionStrings, actionStr)
				}
			}
			if len(actionStrings) > 0 {
				summary = append(summary, fmt.Sprintf("%s: %s", resource, strings.Join(actionStrings, ", ")))
			}
		}
	}

	return summary
}

// IsSystemRole checks if this is a system role
func (r *Role) IsSystemRole() bool {
	return r.IsSystem
}

// CanBeDeleted checks if the role can be deleted
func (r *Role) CanBeDeleted() bool {
	return !r.IsSystem && r.IsActive
}

// DefaultRoles returns the default system roles
func DefaultRoles() []Role {
	adminPerms := map[string]interface{}{
		ResourceAll: []interface{}{PermissionAdmin},
	}

	readonlyPerms := map[string]interface{}{
		ResourceAll: []interface{}{PermissionRead},
	}

	operatorPerms := map[string]interface{}{
		ResourceScans:     []interface{}{PermissionRead, PermissionWrite, PermissionDelete},
		ResourceDiscovery: []interface{}{PermissionRead, PermissionWrite, PermissionDelete},
		ResourceHosts:     []interface{}{PermissionRead},
		ResourceNetworks:  []interface{}{PermissionRead},
		ResourceProfiles:  []interface{}{PermissionRead, PermissionWrite},
	}

	return []Role{
		{
			Name:        RoleAdmin,
			Description: "Full administrative access to all resources",
			Permissions: adminPerms,
			IsActive:    true,
			IsSystem:    true,
			CreatedAt:   time.Now().UTC(),
			UpdatedAt:   time.Now().UTC(),
		},
		{
			Name:        RoleReadonly,
			Description: "Read-only access to all resources",
			Permissions: readonlyPerms,
			IsActive:    true,
			IsSystem:    true,
			CreatedAt:   time.Now().UTC(),
			UpdatedAt:   time.Now().UTC(),
		},
		{
			Name:        RoleOperator,
			Description: "Operational access for scans and discovery",
			Permissions: operatorPerms,
			IsActive:    true,
			IsSystem:    true,
			CreatedAt:   time.Now().UTC(),
			UpdatedAt:   time.Now().UTC(),
		},
	}
}

// CheckPermissions checks if a set of roles has a specific permission
func CheckPermissions(roles []Role, resource, action string) bool {
	for _, role := range roles {
		if role.HasPermission(resource, action) {
			return true
		}
	}
	return false
}

// MarshalPermissions converts permissions to JSON for database storage
func MarshalPermissions(permissions map[string]interface{}) ([]byte, error) {
	return json.Marshal(permissions)
}

// UnmarshalPermissions converts JSON permissions from database
func UnmarshalPermissions(data []byte) (map[string]interface{}, error) {
	var permissions map[string]interface{}
	if len(data) == 0 {
		return make(map[string]interface{}), nil
	}
	err := json.Unmarshal(data, &permissions)
	return permissions, err
}

// ValidatePermissions validates the structure of permissions
func ValidatePermissions(permissions map[string]interface{}) error {
	for resource, actions := range permissions {
		if resource == "" {
			return fmt.Errorf("resource name cannot be empty")
		}

		actionList, ok := actions.([]interface{})
		if !ok {
			return fmt.Errorf("permissions for resource '%s' must be an array", resource)
		}

		if len(actionList) == 0 {
			return fmt.Errorf("at least one action required for resource '%s'", resource)
		}

		for _, action := range actionList {
			actionStr, ok := action.(string)
			if !ok {
				return fmt.Errorf("action must be a string for resource '%s'", resource)
			}
			if actionStr == "" {
				return fmt.Errorf("action cannot be empty for resource '%s'", resource)
			}
		}
	}

	return nil
}

// PermissionsEqual checks if two permission sets are equal
func PermissionsEqual(a, b map[string]interface{}) bool {
	if len(a) != len(b) {
		return false
	}

	aJSON, err := json.Marshal(a)
	if err != nil {
		return false
	}

	bJSON, err := json.Marshal(b)
	if err != nil {
		return false
	}

	return bytes.Equal(aJSON, bJSON)
}
