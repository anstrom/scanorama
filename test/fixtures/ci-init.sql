-- CI Database Initialization Script
-- This script ensures proper database setup in GitHub Actions CI environment

-- Create extensions if they don't exist
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "btree_gist";

-- Ensure test_user has proper permissions
GRANT ALL PRIVILEGES ON DATABASE scanorama_test TO test_user;
GRANT CREATE ON DATABASE scanorama_test TO test_user;
ALTER USER test_user WITH CREATEDB SUPERUSER;

-- Set schema permissions
GRANT ALL ON SCHEMA public TO test_user;
GRANT CREATE ON SCHEMA public TO test_user;

-- Verify extensions are available
SELECT 'uuid-ossp extension check: ' || CASE WHEN COUNT(*) > 0 THEN 'OK' ELSE 'MISSING' END
FROM pg_extension WHERE extname = 'uuid-ossp';

SELECT 'btree_gist extension check: ' || CASE WHEN COUNT(*) > 0 THEN 'OK' ELSE 'MISSING' END
FROM pg_extension WHERE extname = 'btree_gist';

-- Test basic functionality
SELECT 'Database ready: ' || current_database() || ' as user ' || current_user;
