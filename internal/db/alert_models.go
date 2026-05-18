package db

import (
	"time"

	"github.com/google/uuid"
)

// AlertTrigger constants for alert rule trigger types.
const (
	AlertTriggerOnline  = "online"
	AlertTriggerOffline = "offline"
	AlertTriggerBoth    = "both"
)

// AlertChannelTypeWebhook is the only supported channel type for now.
const AlertChannelTypeWebhook = "webhook"

// AlertRule represents a configured alert rule for host status transitions.
type AlertRule struct {
	ID          uuid.UUID  `db:"id"           json:"id"`
	HostID      *uuid.UUID `db:"host_id"      json:"host_id"`
	GroupID     *uuid.UUID `db:"group_id"     json:"group_id"`
	Tag         *string    `db:"tag"          json:"tag"`
	Trigger     string     `db:"trigger"      json:"trigger"`
	ChannelType string     `db:"channel_type" json:"channel_type"`
	ChannelURL  string     `db:"channel_url"  json:"channel_url"`
	Enabled     bool       `db:"enabled"      json:"enabled"`
	CreatedAt   time.Time  `db:"created_at"   json:"created_at"`
	UpdatedAt   time.Time  `db:"updated_at"   json:"updated_at"`
}

// CreateAlertRuleInput is the input for creating a new alert rule.
type CreateAlertRuleInput struct {
	HostID     *uuid.UUID
	GroupID    *uuid.UUID
	Tag        *string
	Trigger    string // "online" | "offline" | "both"
	ChannelURL string
}

// UpdateAlertRuleInput is the input for partially updating an alert rule.
type UpdateAlertRuleInput struct {
	Trigger    *string
	ChannelURL *string
	Enabled    *bool
}

// StatusTransitionRow is a row returned by GetStatusTransitionsSince,
// enriched with the host's current tags for rule matching.
type StatusTransitionRow struct {
	HostID     uuid.UUID `db:"host_id"`
	FromStatus string    `db:"from_status"`
	ToStatus   string    `db:"to_status"`
	ChangedAt  time.Time `db:"changed_at"`
	Tags       []string  `db:"tags"`
}
