-- 008_tags_and_groups.sql
-- Adds host tags (array column) and host groups (separate table + membership join table).

-- Tags -------------------------------------------------------------------------

ALTER TABLE hosts
    ADD COLUMN tags TEXT[] NOT NULL DEFAULT '{}';

-- GIN index allows fast array-containment queries (h.tags @> ARRAY['prod']).
CREATE INDEX idx_hosts_tags ON hosts USING GIN (tags);

-- Host groups -----------------------------------------------------------------

CREATE TABLE host_groups (
    id          UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(100) NOT NULL,
    description TEXT         NOT NULL DEFAULT '',
    filter_rule JSONB,
    color       VARCHAR(7),
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_host_groups_name UNIQUE (name)
);

-- Membership join table -------------------------------------------------------

CREATE TABLE host_group_members (
    host_id     UUID NOT NULL REFERENCES hosts(id)       ON DELETE CASCADE,
    group_id    UUID NOT NULL REFERENCES host_groups(id) ON DELETE CASCADE,
    added_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (host_id, group_id)
);

CREATE INDEX idx_host_group_members_group_id ON host_group_members (group_id);
