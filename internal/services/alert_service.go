package services

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/anstrom/scanorama/internal/db"
)

const alertDeliveryTimeout = 30 * time.Second

// alertRepo is the data-access interface used by AlertService.
type alertRepo interface {
	ListAlertRules(ctx context.Context) ([]*db.AlertRule, error)
	ListAlertRulesForHost(ctx context.Context, hostID uuid.UUID) ([]*db.AlertRule, error)
	CreateAlertRule(ctx context.Context, input db.CreateAlertRuleInput) (*db.AlertRule, error)
	GetAlertRule(ctx context.Context, id uuid.UUID) (*db.AlertRule, error)
	UpdateAlertRule(ctx context.Context, id uuid.UUID, input db.UpdateAlertRuleInput) (*db.AlertRule, error)
	DeleteAlertRule(ctx context.Context, id uuid.UUID) error
	GetStatusTransitionsSince(ctx context.Context, since time.Time) ([]db.StatusTransitionRow, error)
}

// alertWebhookDeliverer is the subset of WebhookService used by AlertService.
// It posts directly to a caller-supplied URL rather than fanning out to all
// registered webhook subscribers.
type alertWebhookDeliverer interface {
	DeliverToURL(ctx context.Context, url string, event WebhookEvent) error
}

// AlertService handles alert rule management and event-driven alert evaluation.
type AlertService struct {
	repo    alertRepo
	webhook alertWebhookDeliverer
	logger  *slog.Logger
}

// NewAlertService creates a new AlertService.
func NewAlertService(
	repo alertRepo, webhook alertWebhookDeliverer, logger *slog.Logger,
) *AlertService {
	return &AlertService{
		repo:    repo,
		webhook: webhook,
		logger:  logger.With("service", "alert"),
	}
}

// ListAlertRules returns all configured alert rules.
func (s *AlertService) ListAlertRules(ctx context.Context) ([]*db.AlertRule, error) {
	return s.repo.ListAlertRules(ctx)
}

// ListAlertRulesForHost returns alert rules targeting a specific host.
func (s *AlertService) ListAlertRulesForHost(
	ctx context.Context, hostID uuid.UUID,
) ([]*db.AlertRule, error) {
	return s.repo.ListAlertRulesForHost(ctx, hostID)
}

// CreateAlertRule validates inputs and creates a new alert rule.
func (s *AlertService) CreateAlertRule(
	ctx context.Context, input db.CreateAlertRuleInput,
) (*db.AlertRule, error) {
	if err := validateAlertTrigger(input.Trigger); err != nil {
		return nil, err
	}
	if err := validateAlertChannelURL(input.ChannelURL); err != nil {
		return nil, err
	}
	if err := validateAlertTarget(input.HostID, input.GroupID, input.Tag); err != nil {
		return nil, err
	}
	return s.repo.CreateAlertRule(ctx, input)
}

// GetAlertRule retrieves a single alert rule by ID.
func (s *AlertService) GetAlertRule(ctx context.Context, id uuid.UUID) (*db.AlertRule, error) {
	return s.repo.GetAlertRule(ctx, id)
}

// UpdateAlertRule applies partial updates to an alert rule.
func (s *AlertService) UpdateAlertRule(
	ctx context.Context, id uuid.UUID, input db.UpdateAlertRuleInput,
) (*db.AlertRule, error) {
	if input.Trigger != nil {
		if err := validateAlertTrigger(*input.Trigger); err != nil {
			return nil, err
		}
	}
	if input.ChannelURL != nil {
		if err := validateAlertChannelURL(*input.ChannelURL); err != nil {
			return nil, err
		}
	}
	return s.repo.UpdateAlertRule(ctx, id, input)
}

// DeleteAlertRule removes an alert rule by ID.
func (s *AlertService) DeleteAlertRule(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteAlertRule(ctx, id)
}

// alertPayload is the webhook payload fired for each matched alert.
type alertPayload struct {
	HostID     uuid.UUID `json:"host_id"`
	Trigger    string    `json:"trigger"`
	FromStatus string    `json:"from_status"`
	ToStatus   string    `json:"to_status"`
	ChangedAt  time.Time `json:"changed_at"`
	RuleID     uuid.UUID `json:"rule_id"`
}

// EvaluateAlerts loads recent status transitions and fires webhook deliveries
// for each matching enabled alert rule. It returns nil even when individual
// deliveries fail — they are non-blocking.
func (s *AlertService) EvaluateAlerts(ctx context.Context, since time.Time) error {
	transitions, err := s.repo.GetStatusTransitionsSince(ctx, since)
	if err != nil {
		return fmt.Errorf("get status transitions: %w", err)
	}
	if len(transitions) == 0 {
		return nil
	}

	rules, err := s.repo.ListAlertRules(ctx)
	if err != nil {
		return fmt.Errorf("list alert rules: %w", err)
	}

	for i := range transitions {
		t := &transitions[i]
		// Skip no-op transitions (guard against identical from/to status).
		if t.FromStatus == t.ToStatus {
			continue
		}
		for _, rule := range rules {
			if !rule.Enabled {
				continue
			}
			if !ruleMatchesTransition(rule, t) {
				continue
			}
			if !triggerMatchesTransition(rule.Trigger, t.ToStatus) {
				continue
			}
			s.fireAlert(rule, t)
		}
	}
	return nil
}

// ruleMatchesTransition returns true when the rule's target (host or tag)
// matches the given transition. Group matching is not yet implemented.
func ruleMatchesTransition(rule *db.AlertRule, t *db.StatusTransitionRow) bool {
	if rule.HostID != nil && *rule.HostID == t.HostID {
		return true
	}
	if rule.Tag != nil {
		for _, tag := range t.Tags {
			if tag == *rule.Tag {
				return true
			}
		}
	}
	return false
}

// triggerMatchesTransition returns true when the rule trigger matches the
// direction of the transition.
func triggerMatchesTransition(trigger, toStatus string) bool {
	switch trigger {
	case db.AlertTriggerOnline:
		return toStatus == db.HostStatusUp
	case db.AlertTriggerOffline:
		return toStatus == db.HostStatusDown
	case db.AlertTriggerBoth:
		return toStatus == db.HostStatusUp || toStatus == db.HostStatusDown
	default:
		return false
	}
}

// fireAlert delivers a single alert webhook event for a matched rule.
// It posts directly to rule.ChannelURL without going through the subscriber
// fanout — each alert rule targets exactly one endpoint.
func (s *AlertService) fireAlert(rule *db.AlertRule, t *db.StatusTransitionRow) {
	eventType := EventHostOnline
	if t.ToStatus == db.HostStatusDown {
		eventType = EventHostOffline
	}
	event := WebhookEvent{
		Type: eventType,
		Payload: alertPayload{
			HostID:     t.HostID,
			Trigger:    rule.Trigger,
			FromStatus: t.FromStatus,
			ToStatus:   t.ToStatus,
			ChangedAt:  t.ChangedAt,
			RuleID:     rule.ID,
		},
		FiredAt: time.Now().UTC(),
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), alertDeliveryTimeout)
		defer cancel()
		if err := s.webhook.DeliverToURL(ctx, rule.ChannelURL, event); err != nil {
			s.logger.Warn("alert webhook delivery failed",
				"rule_id", rule.ID,
				"host_id", t.HostID,
				"error", err,
			)
		}
	}()
}

// validateAlertTrigger checks that trigger is one of the allowed values.
func validateAlertTrigger(trigger string) error {
	switch trigger {
	case db.AlertTriggerOnline, db.AlertTriggerOffline, db.AlertTriggerBoth:
		return nil
	default:
		return fmt.Errorf("trigger must be one of: online, offline, both")
	}
}

// validateAlertChannelURL checks that the URL is non-empty and http/https.
func validateAlertChannelURL(url string) error {
	if url == "" {
		return fmt.Errorf("channel_url is required")
	}
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return fmt.Errorf("channel_url must start with http:// or https://")
	}
	return nil
}

// validateAlertTarget ensures exactly one of hostID or tag is set.
// group_id is accepted in the schema for future use but not yet evaluated —
// reject it at the API layer until group matching is implemented.
func validateAlertTarget(hostID, groupID *uuid.UUID, tag *string) error {
	if groupID != nil {
		return fmt.Errorf("group-based alert rules are not yet supported")
	}
	count := 0
	if hostID != nil {
		count++
	}
	if tag != nil && *tag != "" {
		count++
	}
	if count != 1 {
		return fmt.Errorf("exactly one of host_id or tag must be set")
	}
	return nil
}
