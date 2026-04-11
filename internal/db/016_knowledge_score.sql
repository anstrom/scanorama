-- Migration 016: Add knowledge_score column to hosts.
-- Stores a 0-100 integer representing how much is known about a host.
-- Recomputed by the knowledge service after each enrichment pass.

ALTER TABLE hosts ADD COLUMN IF NOT EXISTS knowledge_score SMALLINT NOT NULL DEFAULT 0;
