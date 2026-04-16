package services

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strings"

	"github.com/google/uuid"

	"github.com/anstrom/scanorama/internal/db"
)

// Signal weights for device matching.
// Weights are defined for all planned signals even when not yet wired,
// so adding a new signal source only requires calling sc.add() in the scorer.
const (
	weightStableMAC = 3
	weightMDNSName  = 3
	weightDNSName   = 2
	weightRandomMAC = 1

	autoAttachThreshold = 3
)

// matcherRepository is the DB interface required by DeviceMatcher.
type matcherRepository interface {
	AllDevicesWithSignals(ctx context.Context) ([]db.DeviceSignals, error)
	AttachHost(ctx context.Context, deviceID, hostID uuid.UUID) error
	UpsertKnownMAC(ctx context.Context, deviceID uuid.UUID, mac string) error
	UpsertKnownName(ctx context.Context, deviceID uuid.UUID, name, source string) error
	UpsertSuggestion(ctx context.Context, hostID, deviceID uuid.UUID, score int, reason string) error
}

// DeviceMatcher scores existing devices against a host's signals and either
// auto-attaches the host or creates a suggestion, based on confidence thresholds.
//
// It is designed to run post-discovery — not during live scanning — after the
// host record has been enriched with mDNS, SNMP, and DNS names.
type DeviceMatcher struct {
	repo   matcherRepository
	logger *slog.Logger
}

// NewDeviceMatcher creates a DeviceMatcher.
func NewDeviceMatcher(repo matcherRepository, logger *slog.Logger) *DeviceMatcher {
	return &DeviceMatcher{repo: repo, logger: logger.With("service", "device_matcher")}
}

// deviceScore accumulates the confidence score and reason string for one candidate.
type deviceScore struct {
	deviceID uuid.UUID
	score    int
	reasons  []string
}

func (s *deviceScore) add(weight int, reason string) {
	s.score += weight
	s.reasons = append(s.reasons, reason)
}

func (s *deviceScore) reason() string {
	return strings.Join(s.reasons, " · ")
}

// MatchHost scores all known devices against the host and auto-attaches or suggests.
//
// Thresholds:
//   - Score ≥ 3 with no tie → auto-attach and learn new signals
//   - Score 1–2, or tie at ≥ 3 → UpsertSuggestion for each candidate
//   - Score 0 → no action
func (m *DeviceMatcher) MatchHost(ctx context.Context, host *db.Host) error {
	devices, err := m.repo.AllDevicesWithSignals(ctx)
	if err != nil {
		return fmt.Errorf("device matcher: load devices: %w", err)
	}
	if len(devices) == 0 {
		return nil
	}

	scores := make([]deviceScore, 0, len(devices))
	for i := range devices {
		sc := deviceScore{deviceID: devices[i].Device.ID}
		m.scoreMAC(host, devices[i], &sc)
		m.scoreNames(host, devices[i], &sc)
		if sc.score > 0 {
			scores = append(scores, sc)
		}
	}
	if len(scores) == 0 {
		return nil
	}

	// Find maximum score across all candidates.
	maxScore := 0
	for _, sc := range scores {
		if sc.score > maxScore {
			maxScore = sc.score
		}
	}

	// Collect candidates at the maximum score.
	top := make([]deviceScore, 0)
	for _, sc := range scores {
		if sc.score == maxScore {
			top = append(top, sc)
		}
	}

	// Single winner above threshold → auto-attach.
	if maxScore >= autoAttachThreshold && len(top) == 1 {
		return m.autoAttach(ctx, host, top[0])
	}

	// Tie or below threshold → suggest all candidates with score ≥ 1.
	for _, sc := range scores {
		if err := m.repo.UpsertSuggestion(ctx, host.ID, sc.deviceID, sc.score, sc.reason()); err != nil {
			m.logger.Warn("device matcher: upsert suggestion failed",
				"host_id", host.ID, "device_id", sc.deviceID, "error", err)
		}
	}
	return nil
}

func (m *DeviceMatcher) autoAttach(ctx context.Context, host *db.Host, sc deviceScore) error {
	if err := m.repo.AttachHost(ctx, sc.deviceID, host.ID); err != nil {
		return fmt.Errorf("device matcher: attach host: %w", err)
	}
	m.logger.Info("device matcher: auto-attached",
		"host_id", host.ID, "device_id", sc.deviceID,
		"score", sc.score, "reason", sc.reason())

	// Learn new signals so future hosts with the same signals also auto-attach.
	if host.MACAddress != nil {
		mac := host.MACAddress.String()
		if mac != "" {
			if err := m.repo.UpsertKnownMAC(ctx, sc.deviceID, mac); err != nil {
				m.logger.Warn("device matcher: upsert known mac failed", "error", err)
			}
		}
	}
	if host.MDNSName != nil && *host.MDNSName != "" {
		if err := m.repo.UpsertKnownName(ctx, sc.deviceID, *host.MDNSName, "mdns"); err != nil {
			m.logger.Warn("device matcher: upsert known mdns name failed", "error", err)
		}
	}
	if host.Hostname != nil && *host.Hostname != "" {
		if err := m.repo.UpsertKnownName(ctx, sc.deviceID, *host.Hostname, "dns"); err != nil {
			m.logger.Warn("device matcher: upsert known dns name failed", "error", err)
		}
	}
	return nil
}

// scoreMAC adds MAC-based confidence. Globally-administered (hardware-burned)
// MACs score 3; locally-administered (potentially randomized) MACs score 1.
func (m *DeviceMatcher) scoreMAC(host *db.Host, sig db.DeviceSignals, sc *deviceScore) {
	if host.MACAddress == nil {
		return
	}
	hostMAC := strings.ToLower(host.MACAddress.String())
	if hostMAC == "" {
		return
	}
	for _, known := range sig.KnownMACs {
		if strings.EqualFold(known, hostMAC) {
			if isGloballyAdministeredMAC(hostMAC) {
				sc.add(weightStableMAC, "MAC:stable")
			} else {
				sc.add(weightRandomMAC, "MAC:random")
			}
			return
		}
	}
}

// isGloballyAdministeredMAC returns true when the MAC's U/L bit is 0
// (IEEE globally-administered, i.e. hardware-burned). A locally-administered
// MAC — which includes randomized MACs — has the U/L bit set to 1.
func isGloballyAdministeredMAC(mac string) bool {
	hw, err := net.ParseMAC(mac)
	if err != nil || len(hw) == 0 {
		return false
	}
	return hw[0]&0x02 == 0
}

// scoreNames adds confidence from mDNS and DNS name matches.
func (m *DeviceMatcher) scoreNames(host *db.Host, sig db.DeviceSignals, sc *deviceScore) {
	for _, known := range sig.KnownNames {
		switch known.Source {
		case "mdns":
			if host.MDNSName != nil && strings.EqualFold(*host.MDNSName, known.Name) {
				sc.add(weightMDNSName, "mDNS:"+known.Name)
			}
		case "dns":
			if host.Hostname != nil && strings.EqualFold(*host.Hostname, known.Name) {
				sc.add(weightDNSName, "DNS:"+known.Name)
			}
		}
	}
}

// MatchHosts runs MatchHost for each host in the slice.
// Individual host errors are logged but do not abort the remaining matches.
func (m *DeviceMatcher) MatchHosts(ctx context.Context, hosts []*db.Host) {
	for _, h := range hosts {
		if ctx.Err() != nil {
			return
		}
		if err := m.MatchHost(ctx, h); err != nil {
			m.logger.Warn("device matcher: match failed", "host_id", h.ID, "error", err)
		}
	}
}
