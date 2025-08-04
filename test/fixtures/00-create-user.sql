-- Create test user and grant privileges
-- This script creates the test user and ensures proper permissions

-- Check if the role already exists
DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = 'test_user') THEN
        CREATE USER test_user WITH PASSWORD 'test_password';
    END IF;
END
$$;

-- Make sure test_user can create databases and tables
ALTER USER test_user WITH CREATEDB SUPERUSER;

-- Grant privileges on the database
GRANT ALL PRIVILEGES ON DATABASE scanorama_test TO test_user;

-- Grant privileges for extensions
GRANT CREATE ON DATABASE scanorama_test TO test_user;
ALTER USER test_user WITH SUPERUSER;

-- Connect to the database to set schema permissions
\c scanorama_test;

-- Set ownership and permissions
ALTER SCHEMA public OWNER TO test_user;
GRANT ALL ON SCHEMA public TO test_user;

-- Ensure test_user can create and use extensions
GRANT CREATE ON DATABASE scanorama_test TO test_user;
ALTER DATABASE scanorama_test SET search_path TO "$user", public, extensions;

-- Enable extensions for the database
\c scanorama_test;
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "btree_gist";
