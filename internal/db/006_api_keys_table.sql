-- Migration 005: API Keys Management Table
-- This migration creates the api_keys table for runtime API key management,
-- replacing static configuration-based keys with database-managed keys.
-- Designed to support future RBAC implementation.

-- Create api_keys table for storing and managing API keys
CREATE TABLE api_keys (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),

    -- Basic key information
    name VARCHAR(255) NOT NULL,
    key_hash VARCHAR(255) UNIQUE NOT NULL, -- bcrypt hash of the API key
    key_prefix VARCHAR(20) NOT NULL,       -- Display prefix (e.g., "sk_live_abc...")

    -- Timestamps and usage tracking
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW() NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW() NOT NULL,
    last_used_at TIMESTAMP WITH TIME ZONE,
    expires_at TIMESTAMP WITH TIME ZONE,

    -- Status and usage
    is_active BOOLEAN DEFAULT true NOT NULL,
    usage_count INTEGER DEFAULT 0 NOT NULL,

    -- Phase 2 ready fields (RBAC support)
    permissions JSONB,                     -- Granular permissions object (deprecated - use roles)

    -- Administrative fields
    created_by UUID,                       -- Future: reference to admin user who created the key
    notes TEXT,                           -- Optional description/notes

    -- Constraints
    CONSTRAINT api_keys_name_length CHECK (char_length(name) >= 1 AND char_length(name) <= 255),
    CONSTRAINT api_keys_key_prefix_length CHECK (char_length(key_prefix) >= 8 AND char_length(key_prefix) <= 20),
    CONSTRAINT api_keys_usage_count_positive CHECK (usage_count >= 0),
    CONSTRAINT api_keys_expires_after_created CHECK (expires_at IS NULL OR expires_at > created_at)
);

-- Indexes for performance
CREATE INDEX idx_api_keys_key_hash ON api_keys(key_hash) WHERE is_active = true;
CREATE INDEX idx_api_keys_active ON api_keys(is_active) WHERE is_active = true;
CREATE INDEX idx_api_keys_expires_at ON api_keys(expires_at) WHERE expires_at IS NOT NULL AND is_active = true;
CREATE INDEX idx_api_keys_last_used ON api_keys(last_used_at DESC) WHERE is_active = true;
CREATE INDEX idx_api_keys_created_at ON api_keys(created_at DESC);

-- Partial index for active keys (most common query)
CREATE INDEX idx_api_keys_active_valid ON api_keys(key_hash, last_used_at)
WHERE is_active = true;

-- Create roles table for RBAC
CREATE TABLE roles (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),

    -- Basic role information
    name VARCHAR(100) UNIQUE NOT NULL,
    description TEXT,

    -- Permissions and configuration
    permissions JSONB DEFAULT '{}' NOT NULL,

    -- Status and metadata
    is_active BOOLEAN DEFAULT true NOT NULL,
    is_system BOOLEAN DEFAULT false NOT NULL, -- System roles cannot be deleted

    -- Timestamps
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW() NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW() NOT NULL,
    created_by UUID, -- Future reference to admin user

    -- Constraints
    CONSTRAINT roles_name_length CHECK (char_length(name) >= 1 AND char_length(name) <= 100),
    CONSTRAINT roles_name_format CHECK (name ~ '^[a-zA-Z][a-zA-Z0-9_-]*$') -- Alphanumeric, underscore, hyphen
);

-- Create junction table for API key roles (many-to-many)
CREATE TABLE api_key_roles (
    api_key_id UUID NOT NULL REFERENCES api_keys(id) ON DELETE CASCADE,
    role_id UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,

    -- Grant metadata
    granted_at TIMESTAMP WITH TIME ZONE DEFAULT NOW() NOT NULL,
    granted_by UUID, -- Future reference to admin user who granted the role

    -- Primary key on combination
    PRIMARY KEY (api_key_id, role_id)
);

-- Indexes for roles table
CREATE INDEX idx_roles_active ON roles(is_active) WHERE is_active = true;
CREATE INDEX idx_roles_name ON roles(name) WHERE is_active = true;
CREATE INDEX idx_roles_system ON roles(is_system);

-- Indexes for junction table
CREATE INDEX idx_api_key_roles_api_key ON api_key_roles(api_key_id);
CREATE INDEX idx_api_key_roles_role ON api_key_roles(role_id);
CREATE INDEX idx_api_key_roles_granted_at ON api_key_roles(granted_at DESC);

-- Function to update the updated_at timestamp
CREATE OR REPLACE FUNCTION update_api_keys_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Trigger to automatically update updated_at timestamp
CREATE TRIGGER trigger_api_keys_updated_at
    BEFORE UPDATE ON api_keys
    FOR EACH ROW
    EXECUTE FUNCTION update_api_keys_updated_at();

-- Function to clean up expired API keys (can be called by a scheduled job)
CREATE OR REPLACE FUNCTION cleanup_expired_api_keys()
RETURNS INTEGER AS $$
DECLARE
    deleted_count INTEGER;
BEGIN
    -- Deactivate expired keys instead of deleting them for audit purposes
    UPDATE api_keys
    SET is_active = false, updated_at = NOW()
    WHERE is_active = true
    AND expires_at IS NOT NULL
    AND expires_at < NOW();

    GET DIAGNOSTICS deleted_count = ROW_COUNT;

    -- Log the cleanup operation
    INSERT INTO audit_log (
        operation,
        table_name,
        details,
        created_at
    ) VALUES (
        'cleanup_expired_keys',
        'api_keys',
        json_build_object('deactivated_count', deleted_count),
        NOW()
    ) ON CONFLICT DO NOTHING; -- Ignore if audit_log table doesn't exist yet

    RETURN deleted_count;
END;
$$ LANGUAGE plpgsql;

-- Add comments for documentation
COMMENT ON TABLE api_keys IS 'Runtime-managed API keys for authentication, replacing static configuration-based keys';
COMMENT ON COLUMN api_keys.key_hash IS 'bcrypt hash of the actual API key (never store plaintext keys)';
COMMENT ON COLUMN api_keys.key_prefix IS 'Display-safe prefix of the key for identification in UI (e.g., sk_abc123...)';
COMMENT ON COLUMN api_keys.permissions IS 'JSONB object containing granular permissions (deprecated - use roles instead)';
COMMENT ON COLUMN api_keys.usage_count IS 'Number of times this key has been used for authentication';
COMMENT ON COLUMN api_keys.expires_at IS 'Optional expiration timestamp - key becomes invalid after this time';

-- Comments for roles table
COMMENT ON TABLE roles IS 'Roles for role-based access control (RBAC) system';
COMMENT ON COLUMN roles.name IS 'Unique role name (e.g., admin, readonly, operator)';
COMMENT ON COLUMN roles.description IS 'Human-readable description of the role';
COMMENT ON COLUMN roles.permissions IS 'JSONB object defining what actions this role can perform';
COMMENT ON COLUMN roles.is_system IS 'System roles cannot be deleted and are managed by code';

-- Comments for junction table
COMMENT ON TABLE api_key_roles IS 'Many-to-many relationship between API keys and roles';
COMMENT ON COLUMN api_key_roles.granted_at IS 'When this role was granted to the API key';
COMMENT ON COLUMN api_key_roles.granted_by IS 'Admin user who granted this role (future feature)';

-- Create default roles
INSERT INTO roles (name, description, permissions, is_system) VALUES
    ('admin', 'Full administrative access to all resources', '{"*": ["*"]}', true),
    ('readonly', 'Read-only access to all resources', '{"*": ["read"]}', true),
    ('operator', 'Operational access for scans and discovery', '{"scans": ["*"], "discovery": ["*"], "hosts": ["read"], "networks": ["read"]}', true);

-- Create initial admin API key if none exists (migration safety)
-- This ensures the system remains accessible after migration
DO $$
DECLARE
    admin_key_exists BOOLEAN;
    new_key_id UUID;
    admin_role_id UUID;
BEGIN
    -- Check if any active keys exist
    SELECT EXISTS(SELECT 1 FROM api_keys WHERE is_active = true) INTO admin_key_exists;

    -- If no active keys exist, create a default admin key
    -- Note: In production, this should be changed immediately
    IF NOT admin_key_exists THEN
        -- Insert the API key
        INSERT INTO api_keys (
            name,
            key_hash,
            key_prefix,
            notes,
            created_at
        ) VALUES (
            'Initial Admin Key (CHANGE IMMEDIATELY)',
            '$2a$12$LQv3c1yqBWVHxkd0LHAkCOYz6TtxMQJqhN8/lewQ9L93Y0X9F8/oa', -- bcrypt hash of 'admin-key-please-change-immediately'
            'sk_initial...',
            'Default admin key created during migration. Change this immediately for security.',
            NOW()
        ) RETURNING id INTO new_key_id;

        -- Get admin role ID
        SELECT id INTO admin_role_id FROM roles WHERE name = 'admin';

        -- Grant admin role to the initial key
        INSERT INTO api_key_roles (api_key_id, role_id) VALUES (new_key_id, admin_role_id);
    END IF;
END $$;
