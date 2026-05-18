// Package services contains unit tests for the alert service business logic.
// These tests use hand-rolled mocks so no database or external dependencies
// are required.
package services

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anstrom/scanorama/internal/db"
)

// ─────────────────────────────────────────────────────────────────────────────
// Hand-rolled mock repository
// ─────────────────────────────────────────────────────────────────────────────

// updateAlertFn is the type for the UpdateAlertRule mock function field.
type updateAlertFn func(
	ctx context.Context, id uuid.UUID, input db.UpdateAlertRuleInput,
) (*db.AlertRule, error)

type mockAlertRepo struct {
	listAlertRulesFn            func(ctx context.Context) ([]*db.AlertRule, error)
	listAlertRulesForHostFn     func(ctx context.Context, hostID uuid.UUID) ([]*db.AlertRule, error)
	createAlertRuleFn           func(ctx context.Context, input db.CreateAlertRuleInput) (*db.AlertRule, error)
	getAlertRuleFn              func(ctx context.Context, id uuid.UUID) (*db.AlertRule, error)
	updateAlertRuleFn           updateAlertFn
	deleteAlertRuleFn           func(ctx context.Context, id uuid.UUID) error
	getStatusTransitionsSinceFn func(ctx context.Context, since time.Time) ([]db.StatusTransitionRow, error)
}

func (m *mockAlertRepo) ListAlertRules(ctx context.Context) ([]*db.AlertRule, error) {
	if m.listAlertRulesFn != nil {
		return m.listAlertRulesFn(ctx)
	}
	return nil, nil
}

func (m *mockAlertRepo) ListAlertRulesForHost(ctx context.Context, hostID uuid.UUID) ([]*db.AlertRule, error) {
	if m.listAlertRulesForHostFn != nil {
		return m.listAlertRulesForHostFn(ctx, hostID)
	}
	return nil, nil
}

func (m *mockAlertRepo) CreateAlertRule(
	ctx context.Context, input db.CreateAlertRuleInput,
) (*db.AlertRule, error) {
	if m.createAlertRuleFn != nil {
		return m.createAlertRuleFn(ctx, input)
	}
	return nil, nil
}

func (m *mockAlertRepo) GetAlertRule(ctx context.Context, id uuid.UUID) (*db.AlertRule, error) {
	if m.getAlertRuleFn != nil {
		return m.getAlertRuleFn(ctx, id)
	}
	return nil, nil
}

func (m *mockAlertRepo) UpdateAlertRule(
	ctx context.Context, id uuid.UUID, input db.UpdateAlertRuleInput,
) (*db.AlertRule, error) {
	if m.updateAlertRuleFn != nil {
		return m.updateAlertRuleFn(ctx, id, input)
	}
	return nil, nil
}

func (m *mockAlertRepo) DeleteAlertRule(ctx context.Context, id uuid.UUID) error {
	if m.deleteAlertRuleFn != nil {
		return m.deleteAlertRuleFn(ctx, id)
	}
	return nil
}

func (m *mockAlertRepo) GetStatusTransitionsSince(
	ctx context.Context, since time.Time,
) ([]db.StatusTransitionRow, error) {
	if m.getStatusTransitionsSinceFn != nil {
		return m.getStatusTransitionsSinceFn(ctx, since)
	}
	return nil, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Hand-rolled mock webhook deliverer
// ─────────────────────────────────────────────────────────────────────────────

type mockWebhookDeliverer struct {
	deliverToURLFn func(ctx context.Context, url string, event WebhookEvent) error
}

func (m *mockWebhookDeliverer) DeliverToURL(ctx context.Context, url string, event WebhookEvent) error {
	if m.deliverToURLFn != nil {
		return m.deliverToURLFn(ctx, url, event)
	}
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

func newTestAlertService(repo alertRepo, webhook alertWebhookDeliverer) *AlertService {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewAlertService(repo, webhook, logger)
}

func ptr[T any](v T) *T { return &v }

// ─────────────────────────────────────────────────────────────────────────────
// CreateAlertRule — validation tests
// ─────────────────────────────────────────────────────────────────────────────

func TestAlertService_CreateAlertRule_InvalidTrigger(t *testing.T) {
	svc := newTestAlertService(&mockAlertRepo{}, &mockWebhookDeliverer{})
	hostID := uuid.New()
	_, err := svc.CreateAlertRule(context.Background(), db.CreateAlertRuleInput{
		Trigger:    "bad",
		ChannelURL: "https://example.com/hook",
		HostID:     &hostID,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "trigger")
}

func TestAlertService_CreateAlertRule_InvalidChannelURL(t *testing.T) {
	svc := newTestAlertService(&mockAlertRepo{}, &mockWebhookDeliverer{})
	hostID := uuid.New()
	_, err := svc.CreateAlertRule(context.Background(), db.CreateAlertRuleInput{
		Trigger:    db.AlertTriggerOnline,
		ChannelURL: "",
		HostID:     &hostID,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "channel_url")
}

func TestAlertService_CreateAlertRule_URLNotHTTP(t *testing.T) {
	svc := newTestAlertService(&mockAlertRepo{}, &mockWebhookDeliverer{})
	hostID := uuid.New()
	_, err := svc.CreateAlertRule(context.Background(), db.CreateAlertRuleInput{
		Trigger:    db.AlertTriggerOnline,
		ChannelURL: "ftp://example.com/hook",
		HostID:     &hostID,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "channel_url")
}

func TestAlertService_CreateAlertRule_NoTarget(t *testing.T) {
	svc := newTestAlertService(&mockAlertRepo{}, &mockWebhookDeliverer{})
	_, err := svc.CreateAlertRule(context.Background(), db.CreateAlertRuleInput{
		Trigger:    db.AlertTriggerOnline,
		ChannelURL: "https://example.com/hook",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exactly one")
}

func TestAlertService_CreateAlertRule_MultipleTargets(t *testing.T) {
	svc := newTestAlertService(&mockAlertRepo{}, &mockWebhookDeliverer{})
	hostID := uuid.New()
	tag := "server"
	_, err := svc.CreateAlertRule(context.Background(), db.CreateAlertRuleInput{
		Trigger:    db.AlertTriggerOnline,
		ChannelURL: "https://example.com/hook",
		HostID:     &hostID,
		Tag:        &tag,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exactly one")
}

func TestAlertService_CreateAlertRule_GroupIDRejected(t *testing.T) {
	svc := newTestAlertService(&mockAlertRepo{}, &mockWebhookDeliverer{})
	groupID := uuid.New()
	_, err := svc.CreateAlertRule(context.Background(), db.CreateAlertRuleInput{
		Trigger:    db.AlertTriggerOnline,
		ChannelURL: "https://example.com/hook",
		GroupID:    &groupID,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "group-based alert rules are not yet supported")
}

func TestAlertService_CreateAlertRule_ValidHost(t *testing.T) {
	hostID := uuid.New()
	ruleID := uuid.New()
	expected := &db.AlertRule{
		ID:         ruleID,
		HostID:     &hostID,
		Trigger:    db.AlertTriggerOnline,
		ChannelURL: "https://example.com/hook",
		Enabled:    true,
	}

	called := false
	repo := &mockAlertRepo{
		createAlertRuleFn: func(_ context.Context, input db.CreateAlertRuleInput) (*db.AlertRule, error) {
			called = true
			assert.Equal(t, hostID, *input.HostID)
			assert.Equal(t, db.AlertTriggerOnline, input.Trigger)
			return expected, nil
		},
	}
	svc := newTestAlertService(repo, &mockWebhookDeliverer{})

	got, err := svc.CreateAlertRule(context.Background(), db.CreateAlertRuleInput{
		Trigger:    db.AlertTriggerOnline,
		ChannelURL: "https://example.com/hook",
		HostID:     &hostID,
	})
	require.NoError(t, err)
	require.True(t, called, "repo.CreateAlertRule was not called")
	assert.Equal(t, expected, got)
}

// ─────────────────────────────────────────────────────────────────────────────
// UpdateAlertRule — validation tests
// ─────────────────────────────────────────────────────────────────────────────

func TestAlertService_UpdateAlertRule_InvalidTrigger(t *testing.T) {
	svc := newTestAlertService(&mockAlertRepo{}, &mockWebhookDeliverer{})
	_, err := svc.UpdateAlertRule(context.Background(), uuid.New(), db.UpdateAlertRuleInput{
		Trigger: ptr("bad"),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "trigger")
}

func TestAlertService_UpdateAlertRule_InvalidChannelURL(t *testing.T) {
	svc := newTestAlertService(&mockAlertRepo{}, &mockWebhookDeliverer{})
	_, err := svc.UpdateAlertRule(context.Background(), uuid.New(), db.UpdateAlertRuleInput{
		ChannelURL: ptr(""),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "channel_url")
}

func TestAlertService_UpdateAlertRule_NilTrigger(t *testing.T) {
	ruleID := uuid.New()
	expected := &db.AlertRule{ID: ruleID, Enabled: false}

	called := false
	repo := &mockAlertRepo{
		updateAlertRuleFn: func(_ context.Context, id uuid.UUID, _ db.UpdateAlertRuleInput) (*db.AlertRule, error) {
			called = true
			assert.Equal(t, ruleID, id)
			return expected, nil
		},
	}
	svc := newTestAlertService(repo, &mockWebhookDeliverer{})

	got, err := svc.UpdateAlertRule(context.Background(), ruleID, db.UpdateAlertRuleInput{
		Enabled: ptr(false),
	})
	require.NoError(t, err)
	require.True(t, called, "repo.UpdateAlertRule was not called")
	assert.Equal(t, expected, got)
}

// ─────────────────────────────────────────────────────────────────────────────
// EvaluateAlerts tests
// ─────────────────────────────────────────────────────────────────────────────

func TestAlertService_EvaluateAlerts_NoTransitions(t *testing.T) {
	listCalled := false
	repo := &mockAlertRepo{
		getStatusTransitionsSinceFn: func(_ context.Context, _ time.Time) ([]db.StatusTransitionRow, error) {
			return []db.StatusTransitionRow{}, nil
		},
		listAlertRulesFn: func(_ context.Context) ([]*db.AlertRule, error) {
			listCalled = true
			return nil, nil
		},
	}
	svc := newTestAlertService(repo, &mockWebhookDeliverer{})

	err := svc.EvaluateAlerts(context.Background(), time.Now().Add(-time.Minute))
	require.NoError(t, err)
	assert.False(t, listCalled, "ListAlertRules should not be called when there are no transitions")
}

func TestAlertService_EvaluateAlerts_SkipsIdenticalTransitions(t *testing.T) {
	hostID := uuid.New()
	ruleID := uuid.New()

	repo := &mockAlertRepo{
		getStatusTransitionsSinceFn: func(_ context.Context, _ time.Time) ([]db.StatusTransitionRow, error) {
			return []db.StatusTransitionRow{
				{HostID: hostID, FromStatus: db.HostStatusUp, ToStatus: db.HostStatusUp, ChangedAt: time.Now()},
			}, nil
		},
		listAlertRulesFn: func(_ context.Context) ([]*db.AlertRule, error) {
			return []*db.AlertRule{
				{
					ID: ruleID, HostID: &hostID,
					Trigger: db.AlertTriggerOnline, ChannelURL: "https://x.com", Enabled: true,
				},
			}, nil
		},
	}
	deliverCalled := false
	webhook := &mockWebhookDeliverer{
		deliverToURLFn: func(_ context.Context, _ string, _ WebhookEvent) error {
			deliverCalled = true
			return nil
		},
	}
	svc := newTestAlertService(repo, webhook)

	err := svc.EvaluateAlerts(context.Background(), time.Now().Add(-time.Minute))
	require.NoError(t, err)
	// Allow a brief moment for any goroutines that might have been spawned.
	time.Sleep(50 * time.Millisecond)
	assert.False(t, deliverCalled, "Deliver should not be called for identical from/to status")
}

func TestAlertService_EvaluateAlerts_FiresForMatchingHostRule(t *testing.T) {
	hostID := uuid.New()
	ruleID := uuid.New()

	repo := &mockAlertRepo{
		getStatusTransitionsSinceFn: func(_ context.Context, _ time.Time) ([]db.StatusTransitionRow, error) {
			return []db.StatusTransitionRow{
				{HostID: hostID, FromStatus: db.HostStatusDown, ToStatus: db.HostStatusUp, ChangedAt: time.Now()},
			}, nil
		},
		listAlertRulesFn: func(_ context.Context) ([]*db.AlertRule, error) {
			return []*db.AlertRule{
				{
					ID: ruleID, HostID: &hostID,
					Trigger: db.AlertTriggerOnline, ChannelURL: "https://x.com", Enabled: true,
				},
			}, nil
		},
	}

	delivered := make(chan WebhookEvent, 1)
	webhook := &mockWebhookDeliverer{
		deliverToURLFn: func(_ context.Context, _ string, event WebhookEvent) error {
			delivered <- event
			return nil
		},
	}
	svc := newTestAlertService(repo, webhook)

	err := svc.EvaluateAlerts(context.Background(), time.Now().Add(-time.Minute))
	require.NoError(t, err)

	select {
	case event := <-delivered:
		assert.Equal(t, EventHostOnline, event.Type)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for webhook delivery")
	}
}

func TestAlertService_EvaluateAlerts_SkipsDisabledRule(t *testing.T) {
	hostID := uuid.New()
	ruleID := uuid.New()

	repo := &mockAlertRepo{
		getStatusTransitionsSinceFn: func(_ context.Context, _ time.Time) ([]db.StatusTransitionRow, error) {
			return []db.StatusTransitionRow{
				{HostID: hostID, FromStatus: db.HostStatusDown, ToStatus: db.HostStatusUp, ChangedAt: time.Now()},
			}, nil
		},
		listAlertRulesFn: func(_ context.Context) ([]*db.AlertRule, error) {
			return []*db.AlertRule{
				{
					ID: ruleID, HostID: &hostID, Trigger: db.AlertTriggerOnline,
					ChannelURL: "https://x.com", Enabled: false,
				},
			}, nil
		},
	}
	deliverCalled := false
	webhook := &mockWebhookDeliverer{
		deliverToURLFn: func(_ context.Context, _ string, _ WebhookEvent) error {
			deliverCalled = true
			return nil
		},
	}
	svc := newTestAlertService(repo, webhook)

	err := svc.EvaluateAlerts(context.Background(), time.Now().Add(-time.Minute))
	require.NoError(t, err)
	time.Sleep(50 * time.Millisecond)
	assert.False(t, deliverCalled, "Deliver should not be called for a disabled rule")
}

func TestAlertService_EvaluateAlerts_TagMatching(t *testing.T) {
	hostID := uuid.New()
	ruleID := uuid.New()
	tag := "server"

	repo := &mockAlertRepo{
		getStatusTransitionsSinceFn: func(_ context.Context, _ time.Time) ([]db.StatusTransitionRow, error) {
			return []db.StatusTransitionRow{
				{
					HostID:     hostID,
					FromStatus: db.HostStatusDown,
					ToStatus:   db.HostStatusUp,
					ChangedAt:  time.Now(),
					Tags:       []string{"server", "prod"},
				},
			}, nil
		},
		listAlertRulesFn: func(_ context.Context) ([]*db.AlertRule, error) {
			return []*db.AlertRule{
				{ID: ruleID, Tag: &tag, Trigger: db.AlertTriggerOnline, ChannelURL: "https://x.com", Enabled: true},
			}, nil
		},
	}

	var wg sync.WaitGroup
	wg.Add(1)
	delivered := make(chan WebhookEvent, 1)
	webhook := &mockWebhookDeliverer{
		deliverToURLFn: func(_ context.Context, _ string, event WebhookEvent) error {
			delivered <- event
			wg.Done()
			return nil
		},
	}
	svc := newTestAlertService(repo, webhook)

	err := svc.EvaluateAlerts(context.Background(), time.Now().Add(-time.Minute))
	require.NoError(t, err)

	select {
	case event := <-delivered:
		assert.Equal(t, EventHostOnline, event.Type)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for webhook delivery via tag matching")
	}
}

func TestAlertService_EvaluateAlerts_TargetedDeliveryToChannelURL(t *testing.T) {
	hostID := uuid.New()
	ruleID := uuid.New()
	wantURL := "https://alerts.example.com/hook"

	repo := &mockAlertRepo{
		getStatusTransitionsSinceFn: func(_ context.Context, _ time.Time) ([]db.StatusTransitionRow, error) {
			return []db.StatusTransitionRow{
				{HostID: hostID, FromStatus: db.HostStatusDown, ToStatus: db.HostStatusUp, ChangedAt: time.Now()},
			}, nil
		},
		listAlertRulesFn: func(_ context.Context) ([]*db.AlertRule, error) {
			return []*db.AlertRule{
				{ID: ruleID, HostID: &hostID, Trigger: db.AlertTriggerOnline, ChannelURL: wantURL, Enabled: true},
			}, nil
		},
	}

	gotURL := make(chan string, 1)
	webhook := &mockWebhookDeliverer{
		deliverToURLFn: func(_ context.Context, url string, _ WebhookEvent) error {
			gotURL <- url
			return nil
		},
	}
	svc := newTestAlertService(repo, webhook)

	err := svc.EvaluateAlerts(context.Background(), time.Now().Add(-time.Minute))
	require.NoError(t, err)

	select {
	case url := <-gotURL:
		assert.Equal(t, wantURL, url, "DeliverToURL should target the rule's channel_url, not a fan-out")
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for targeted webhook delivery")
	}
}

func TestAlertService_EvaluateAlerts_TriggerMismatch(t *testing.T) {
	hostID := uuid.New()
	ruleID := uuid.New()

	repo := &mockAlertRepo{
		getStatusTransitionsSinceFn: func(_ context.Context, _ time.Time) ([]db.StatusTransitionRow, error) {
			return []db.StatusTransitionRow{
				// Host went down, but rule only fires on online.
				{HostID: hostID, FromStatus: db.HostStatusUp, ToStatus: db.HostStatusDown, ChangedAt: time.Now()},
			}, nil
		},
		listAlertRulesFn: func(_ context.Context) ([]*db.AlertRule, error) {
			return []*db.AlertRule{
				{
					ID: ruleID, HostID: &hostID,
					Trigger: db.AlertTriggerOnline, ChannelURL: "https://x.com", Enabled: true,
				},
			}, nil
		},
	}
	deliverCalled := false
	webhook := &mockWebhookDeliverer{
		deliverToURLFn: func(_ context.Context, _ string, _ WebhookEvent) error {
			deliverCalled = true
			return nil
		},
	}
	svc := newTestAlertService(repo, webhook)

	err := svc.EvaluateAlerts(context.Background(), time.Now().Add(-time.Minute))
	require.NoError(t, err)
	time.Sleep(50 * time.Millisecond)
	assert.False(t, deliverCalled, "Deliver should not be called when trigger direction does not match")
}

// ─────────────────────────────────────────────────────────────────────────────
// Helper function unit tests (same package → unexported functions accessible)
// ─────────────────────────────────────────────────────────────────────────────

func TestTriggerMatchesTransition(t *testing.T) {
	tests := []struct {
		trigger  string
		toStatus string
		want     bool
	}{
		{db.AlertTriggerOnline, db.HostStatusUp, true},
		{db.AlertTriggerOnline, db.HostStatusDown, false},
		{db.AlertTriggerOffline, db.HostStatusDown, true},
		{db.AlertTriggerOffline, db.HostStatusUp, false},
		{db.AlertTriggerBoth, db.HostStatusUp, true},
		{db.AlertTriggerBoth, db.HostStatusDown, true},
		{"unknown", db.HostStatusUp, false},
		{"", db.HostStatusDown, false},
	}
	for _, tc := range tests {
		got := triggerMatchesTransition(tc.trigger, tc.toStatus)
		assert.Equal(t, tc.want, got, "trigger=%q toStatus=%q", tc.trigger, tc.toStatus)
	}
}

func TestRuleMatchesTransition_HostID(t *testing.T) {
	hostID := uuid.New()
	otherID := uuid.New()

	rule := &db.AlertRule{HostID: &hostID}
	matching := &db.StatusTransitionRow{HostID: hostID}
	notMatching := &db.StatusTransitionRow{HostID: otherID}

	assert.True(t, ruleMatchesTransition(rule, matching))
	assert.False(t, ruleMatchesTransition(rule, notMatching))
}

func TestRuleMatchesTransition_Tag(t *testing.T) {
	tag := "web"
	rule := &db.AlertRule{Tag: &tag}

	withTag := &db.StatusTransitionRow{HostID: uuid.New(), Tags: []string{"web", "prod"}}
	withoutTag := &db.StatusTransitionRow{HostID: uuid.New(), Tags: []string{"db", "prod"}}
	noTags := &db.StatusTransitionRow{HostID: uuid.New(), Tags: []string{}}

	assert.True(t, ruleMatchesTransition(rule, withTag))
	assert.False(t, ruleMatchesTransition(rule, withoutTag))
	assert.False(t, ruleMatchesTransition(rule, noTags))
}

// ─────────────────────────────────────────────────────────────────────────────
// fireAlert — log on delivery failure (no panic, no return value)
// ─────────────────────────────────────────────────────────────────────────────

func TestAlertService_FireAlert_LogsOnDeliveryFailure(t *testing.T) {
	hostID := uuid.New()
	ruleID := uuid.New()
	rule := &db.AlertRule{ID: ruleID, Trigger: db.AlertTriggerOnline, ChannelURL: "https://x.com"}
	transition := &db.StatusTransitionRow{HostID: hostID, ToStatus: db.HostStatusUp}

	done := make(chan struct{})
	webhook := &mockWebhookDeliverer{
		deliverToURLFn: func(_ context.Context, _ string, _ WebhookEvent) error {
			defer close(done)
			return errors.New("delivery failed")
		},
	}
	svc := newTestAlertService(&mockAlertRepo{}, webhook)

	// Must not panic even when Deliver returns an error.
	svc.fireAlert(rule, transition)

	select {
	case <-done:
		// goroutine ran and returned error — logger swallowed it
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for fireAlert goroutine")
	}
}
