package services

import (
	"context"
	"log/slog"
	"net"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anstrom/scanorama/internal/db"
)

// parseMACAddr parses a MAC string for use in Host fixtures.
func parseMACAddr(t *testing.T, mac string) *db.MACAddr {
	t.Helper()
	hw, err := net.ParseMAC(mac)
	require.NoError(t, err)
	m := &db.MACAddr{HardwareAddr: hw}
	return m
}

// mockMatcherRepo is a minimal in-memory implementation of matcherRepository.
type mockMatcherRepo struct {
	devices     []db.DeviceSignals
	attached    map[uuid.UUID]uuid.UUID // hostID → deviceID
	suggestions []suggestionRecord
	learnedMACs []string
}

type suggestionRecord struct {
	hostID   uuid.UUID
	deviceID uuid.UUID
	score    int
}

func (m *mockMatcherRepo) AllDevicesWithSignals(_ context.Context) ([]db.DeviceSignals, error) {
	return m.devices, nil
}
func (m *mockMatcherRepo) AttachHost(_ context.Context, deviceID, hostID uuid.UUID) error {
	if m.attached == nil {
		m.attached = map[uuid.UUID]uuid.UUID{}
	}
	m.attached[hostID] = deviceID
	return nil
}
func (m *mockMatcherRepo) UpsertKnownMAC(_ context.Context, _ uuid.UUID, mac string) error {
	m.learnedMACs = append(m.learnedMACs, mac)
	return nil
}
func (m *mockMatcherRepo) UpsertKnownName(_ context.Context, _ uuid.UUID, _, _ string) error {
	return nil
}
func (m *mockMatcherRepo) UpsertSuggestion(_ context.Context, hostID, deviceID uuid.UUID, score int, _ string) error {
	m.suggestions = append(m.suggestions, suggestionRecord{hostID, deviceID, score})
	return nil
}

// ── Auto-attach on globally-administered MAC ──────────────────────────────

func TestDeviceMatcher_AutoAttach_GlobalMAC(t *testing.T) {
	deviceID := uuid.New()
	hostID := uuid.New()
	// aa:bb:cc:.. — first octet 0xAA = 10101010, bit 1 (U/L) = 1 → locally-administered.
	// Use 00:.. instead: 0x00 = bit 1 = 0 → globally-administered.
	mac := "00:11:22:33:44:55"

	repo := &mockMatcherRepo{
		devices: []db.DeviceSignals{
			{Device: db.Device{ID: deviceID, Name: "Router"}, KnownMACs: []string{mac}},
		},
	}
	svc := NewDeviceMatcher(repo, slog.Default())

	macAddr := parseMACAddr(t, mac)
	host := &db.Host{ID: hostID, MACAddress: macAddr}

	err := svc.MatchHost(context.Background(), host)
	require.NoError(t, err)
	assert.Equal(t, deviceID, repo.attached[hostID], "host should be auto-attached")
	assert.Empty(t, repo.suggestions, "should not create suggestion when auto-attaching")
}

// ── Suggestion on locally-administered (randomized) MAC ──────────────────

func TestDeviceMatcher_Suggestion_RandomMAC(t *testing.T) {
	deviceID := uuid.New()
	hostID := uuid.New()
	// 02:.. — first octet 0x02 = bit 1 = 1 → locally-administered.
	mac := "02:ab:cd:ef:01:23"

	repo := &mockMatcherRepo{
		devices: []db.DeviceSignals{
			{Device: db.Device{ID: deviceID, Name: "Phone"}, KnownMACs: []string{mac}},
		},
	}
	svc := NewDeviceMatcher(repo, slog.Default())

	macAddr := parseMACAddr(t, mac)
	host := &db.Host{ID: hostID, MACAddress: macAddr}

	err := svc.MatchHost(context.Background(), host)
	require.NoError(t, err)
	assert.Empty(t, repo.attached, "should NOT auto-attach on randomized MAC")
	require.Len(t, repo.suggestions, 1, "should create one suggestion")
	assert.Equal(t, 1, repo.suggestions[0].score)
}

// ── Auto-attach on mDNS name ──────────────────────────────────────────────

func TestDeviceMatcher_AutoAttach_MDNSName(t *testing.T) {
	deviceID := uuid.New()
	hostID := uuid.New()
	mdnsName := "myphone.local"

	repo := &mockMatcherRepo{
		devices: []db.DeviceSignals{
			{
				Device: db.Device{ID: deviceID, Name: "Phone"},
				KnownNames: []db.DeviceKnownNameSignal{
					{Name: mdnsName, Source: "mdns"},
				},
			},
		},
	}
	svc := NewDeviceMatcher(repo, slog.Default())

	host := &db.Host{ID: hostID, MDNSName: &mdnsName}

	err := svc.MatchHost(context.Background(), host)
	require.NoError(t, err)
	assert.Equal(t, deviceID, repo.attached[hostID], "host should be auto-attached via mDNS name")
}

// ── No match → no action ──────────────────────────────────────────────────

func TestDeviceMatcher_NoMatch_NoAction(t *testing.T) {
	deviceID := uuid.New()
	hostID := uuid.New()

	repo := &mockMatcherRepo{
		devices: []db.DeviceSignals{
			{Device: db.Device{ID: deviceID, Name: "Router"}, KnownMACs: []string{"00:aa:bb:cc:dd:ee"}},
		},
	}
	svc := NewDeviceMatcher(repo, slog.Default())

	// Host has a different MAC — no match.
	differentMAC := parseMACAddr(t, "00:ff:ff:ff:ff:ff")
	host := &db.Host{ID: hostID, MACAddress: differentMAC}

	err := svc.MatchHost(context.Background(), host)
	require.NoError(t, err)
	assert.Empty(t, repo.attached)
	assert.Empty(t, repo.suggestions)
}

// ── Tie → both suggested, neither attached ────────────────────────────────

func TestDeviceMatcher_Tie_SuggestBoth(t *testing.T) {
	deviceA := uuid.New()
	deviceB := uuid.New()
	hostID := uuid.New()
	mac := "00:11:22:33:44:55" // globally-administered = weight 3

	repo := &mockMatcherRepo{
		devices: []db.DeviceSignals{
			{Device: db.Device{ID: deviceA, Name: "A"}, KnownMACs: []string{mac}},
			{Device: db.Device{ID: deviceB, Name: "B"}, KnownMACs: []string{mac}},
		},
	}
	svc := NewDeviceMatcher(repo, slog.Default())

	macAddr := parseMACAddr(t, mac)
	host := &db.Host{ID: hostID, MACAddress: macAddr}

	err := svc.MatchHost(context.Background(), host)
	require.NoError(t, err)
	assert.Empty(t, repo.attached, "tie should not auto-attach")
	assert.Len(t, repo.suggestions, 2, "both candidates should be suggested")
}

// ── Signal learning on auto-attach ───────────────────────────────────────

func TestDeviceMatcher_AutoAttach_LearnsMDNSName(t *testing.T) {
	deviceID := uuid.New()
	hostID := uuid.New()
	mac := "00:11:22:33:44:55"
	mdnsName := "newname.local"

	repo := &mockMatcherRepo{
		devices: []db.DeviceSignals{
			{Device: db.Device{ID: deviceID, Name: "Router"}, KnownMACs: []string{mac}},
		},
	}
	svc := NewDeviceMatcher(repo, slog.Default())

	macAddr := parseMACAddr(t, mac)
	host := &db.Host{ID: hostID, MACAddress: macAddr, MDNSName: &mdnsName}

	err := svc.MatchHost(context.Background(), host)
	require.NoError(t, err)
	assert.Equal(t, deviceID, repo.attached[hostID])
	assert.Contains(t, repo.learnedMACs, mac, "MAC should be re-learned on auto-attach")
}

// ── No devices → no action ────────────────────────────────────────────────

func TestDeviceMatcher_NoDevices_NoAction(t *testing.T) {
	repo := &mockMatcherRepo{devices: []db.DeviceSignals{}}
	svc := NewDeviceMatcher(repo, slog.Default())

	host := &db.Host{ID: uuid.New()}
	err := svc.MatchHost(context.Background(), host)
	require.NoError(t, err)
	assert.Empty(t, repo.attached)
	assert.Empty(t, repo.suggestions)
}
