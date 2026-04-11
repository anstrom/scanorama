-- Migration 016: Add knowledge_score column to hosts.
-- Stores a 0-100 integer representing how much is known about a host.
-- Recomputed by the knowledge service after each enrichment pass.

ALTER TABLE hosts ADD COLUMN IF NOT EXISTS knowledge_score SMALLINT NOT NULL DEFAULT 0;

-- Backfill: recalculate knowledge_score for all existing hosts using the same
-- five-factor formula as RecalculateKnowledgeScore in repository_host.go.
UPDATE hosts h
SET knowledge_score = (
    CASE WHEN h.os_family IS NOT NULL AND h.os_family != '' THEN 20 ELSE 0 END +
    CASE WHEN EXISTS(
        SELECT 1 FROM port_scans ps WHERE ps.host_id = h.id AND ps.state = 'open'
    ) THEN 20 ELSE 0 END +
    CASE WHEN EXISTS(
        SELECT 1 FROM port_banners pb WHERE pb.host_id = h.id AND pb.service IS NOT NULL
    ) THEN 20 ELSE 0 END +
    CASE WHEN h.last_seen > NOW() - INTERVAL '7 days' THEN 20 ELSE 0 END +
    CASE WHEN (
        EXISTS(SELECT 1 FROM port_banners pb2 WHERE pb2.host_id = h.id) OR
        EXISTS(SELECT 1 FROM host_snmp_data sd WHERE sd.host_id = h.id)
    ) THEN 20 ELSE 0 END
);
