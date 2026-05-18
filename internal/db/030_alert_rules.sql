CREATE TYPE alert_trigger AS ENUM ('online', 'offline', 'both');

CREATE TABLE alert_rules (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    -- target: exactly one of these must be set
    host_id      UUID REFERENCES hosts(id) ON DELETE CASCADE,
    group_id     UUID REFERENCES host_groups(id) ON DELETE CASCADE,
    tag          TEXT,
    -- rule config
    trigger      alert_trigger NOT NULL,
    channel_type TEXT NOT NULL DEFAULT 'webhook',
    channel_url  TEXT NOT NULL,
    enabled      BOOLEAN NOT NULL DEFAULT TRUE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- ensure exactly one target is set
    CONSTRAINT alert_rules_single_target CHECK (
        (host_id IS NOT NULL)::int +
        (group_id IS NOT NULL)::int +
        (tag IS NOT NULL)::int = 1
    )
);

CREATE INDEX alert_rules_host_id_idx ON alert_rules(host_id) WHERE host_id IS NOT NULL;
CREATE INDEX alert_rules_group_id_idx ON alert_rules(group_id) WHERE group_id IS NOT NULL;
CREATE INDEX alert_rules_tag_idx ON alert_rules(tag) WHERE tag IS NOT NULL;
