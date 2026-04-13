-- Migration 021: NSE script metadata columns on port_banners
-- http_title and ssh_key_fingerprint are populated by the nse_storage goroutine
-- after scans that include NSE script execution.

ALTER TABLE port_banners
    ADD COLUMN IF NOT EXISTS http_title          TEXT,
    ADD COLUMN IF NOT EXISTS ssh_key_fingerprint TEXT;
