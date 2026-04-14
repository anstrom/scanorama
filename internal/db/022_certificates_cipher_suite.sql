-- Migration 022: Add cipher_suite to certificates table.
-- Populated by the ZGrab2 TLS/HTTPS scanner during banner enrichment.

ALTER TABLE certificates
    ADD COLUMN IF NOT EXISTS cipher_suite VARCHAR(100);
