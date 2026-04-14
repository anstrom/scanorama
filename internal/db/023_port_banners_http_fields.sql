-- Migration 023: HTTP-specific columns on port_banners.
-- Populated by the ZGrab2 HTTP scanner during banner enrichment.

ALTER TABLE port_banners
    ADD COLUMN IF NOT EXISTS http_status_code      SMALLINT,
    ADD COLUMN IF NOT EXISTS http_redirect         TEXT,
    ADD COLUMN IF NOT EXISTS http_response_headers JSONB;
