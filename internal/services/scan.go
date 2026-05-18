// Package services provides business logic for Scanorama operations.
// This file implements scan management, including input validation, profile
// verification, and enriched state-transition methods.
package services

import (
	"context"
	"database/sql"
	stderrors "errors"
	"fmt"
	"log/slog"
	"net"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/errors"
)

// -- Validation constants --

// MaxScanNameLength is the maximum allowed length for a scan name.
const MaxScanNameLength = 100

// MaxTargetLength is the maximum allowed length for a single target string.
const MaxTargetLength = 200

// MaxTargetCount is the maximum number of targets allowed per scan.
const MaxTargetCount = 100

const (
	scanTypeConnect       = "connect"
	scanTypeSYN           = "syn"
	scanTypeACK           = "ack"
	scanTypeUDP           = "udp"
	scanTypeAggressive    = "aggressive"
	scanTypeComprehensive = "comprehensive"
)

// hostnameLabel matches a single DNS label: 1–63 chars, alphanumeric plus
// interior hyphens, not starting or ending with a hyphen (RFC 1123).
var hostnameLabel = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?$`)

// IsValidScanTarget reports whether s is an acceptable scan target:
// a plain IP address, a CIDR range, or an RFC 1123 hostname.
func IsValidScanTarget(s string) bool {
	if net.ParseIP(s) != nil {
		return true
	}
	if _, _, err := net.ParseCIDR(s); err == nil {
		return true
	}
	// Hostname: dot-separated labels, total length ≤ 253.
	s = strings.TrimRight(s, ".") // strip optional trailing dot
	if s == "" || len(s) > 253 {
		return false
	}
	for _, label := range strings.Split(s, ".") {
		if !hostnameLabel.MatchString(label) {
			return false
		}
	}
	return true
}

// validScanTypes lists the scan-type values accepted by the service layer.
var validScanTypes = map[string]bool{
	scanTypeConnect:       true,
	scanTypeSYN:           true,
	scanTypeACK:           true,
	scanTypeUDP:           true,
	scanTypeAggressive:    true,
	scanTypeComprehensive: true,
}

// -- Repository interface --

// scanRepository is the DB-facing interface consumed by ScanService.
type scanRepository interface {
	ListScans(ctx context.Context, filters db.ScanFilters, offset, limit int) ([]*db.Scan, int64, error)
	CreateScan(ctx context.Context, input db.CreateScanInput) (*db.Scan, error)
	GetScan(ctx context.Context, id uuid.UUID) (*db.Scan, error)
	UpdateScan(ctx context.Context, id uuid.UUID, input db.UpdateScanInput) (*db.Scan, error)
	DeleteScan(ctx context.Context, id uuid.UUID) error
	StartScan(ctx context.Context, id uuid.UUID) error
	StopScan(ctx context.Context, id uuid.UUID, errMsg ...string) error
	CompleteScan(ctx context.Context, id uuid.UUID) error
	GetScanResults(ctx context.Context, scanID uuid.UUID, offset, limit int) ([]*db.ScanResult, int64, error)
	GetAllScanResults(ctx context.Context, scanID uuid.UUID) ([]*db.ScanResult, error)
	GetHostForScan(ctx context.Context, scanID uuid.UUID) (uuid.UUID, error)
	GetScanSummary(ctx context.Context, scanID uuid.UUID) (*db.ScanSummary, error)
	GetProfile(ctx context.Context, id string) (*db.ScanProfile, error)
}

// -- Service --

// ScanService provides business logic for scan lifecycle operations.
type ScanService struct {
	repo   scanRepository
	logger *slog.Logger
}

// NewScanService creates a new ScanService backed by the given repository.
func NewScanService(repo scanRepository, logger *slog.Logger) *ScanService {
	return &ScanService{
		repo:   repo,
		logger: logger,
	}
}

// DB returns the underlying raw *db.DB connection, or nil when the repository
// is not a concrete *db.ScanRepository (e.g. a mock in tests).
// This is used exclusively by the scan execution pipeline which needs direct
// database access to persist discovered hosts and port-scan results.
func (s *ScanService) DB() *db.DB {
	if repo, ok := s.repo.(*db.ScanRepository); ok {
		return repo.DB()
	}
	return nil
}

// ListScans retrieves scans with optional filtering and pagination.
func (s *ScanService) ListScans(
	ctx context.Context,
	filters db.ScanFilters,
	offset, limit int,
) ([]*db.Scan, int64, error) {
	return s.repo.ListScans(ctx, filters, offset, limit)
}

// CreateScan validates the input, optionally verifies the referenced profile,
// and delegates creation to the repository.
func (s *ScanService) CreateScan(ctx context.Context, input db.CreateScanInput) (*db.Scan, error) {
	if err := validateScanInput(input); err != nil {
		return nil, err
	}

	if input.ProfileID != nil && *input.ProfileID != "" {
		_, err := s.repo.GetProfile(ctx, *input.ProfileID)
		if err != nil {
			if errors.IsNotFound(err) {
				return nil, errors.NewScanError(
					errors.CodeValidation,
					fmt.Sprintf("profile %q not found", *input.ProfileID),
				)
			}
			return nil, fmt.Errorf("failed to verify profile: %w", err)
		}
	}

	return s.repo.CreateScan(ctx, input)
}

// GetScan retrieves a single scan by its ID.
func (s *ScanService) GetScan(ctx context.Context, id uuid.UUID) (*db.Scan, error) {
	return s.repo.GetScan(ctx, id)
}

// UpdateScan applies the given mutations to an existing scan.
func (s *ScanService) UpdateScan(
	ctx context.Context,
	id uuid.UUID,
	input db.UpdateScanInput,
) (*db.Scan, error) {
	return s.repo.UpdateScan(ctx, id, input)
}

// DeleteScan removes a scan record by ID.
func (s *ScanService) DeleteScan(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteScan(ctx, id)
}

// StartScan transitions a scan to the running state.
// It fetches the current scan to guard against invalid state transitions,
// calls repo.StartScan, then returns the refreshed scan record.
func (s *ScanService) StartScan(ctx context.Context, id uuid.UUID) (*db.Scan, error) {
	// repo.StartScan uses a single conditional UPDATE (WHERE status = 'pending')
	// that is atomic at the database level — no separate pre-read is needed.
	if err := s.repo.StartScan(ctx, id); err != nil {
		return nil, err
	}
	return s.repo.GetScan(ctx, id)
}

// StopScan halts a running scan, optionally recording an error message.
func (s *ScanService) StopScan(ctx context.Context, id uuid.UUID, errMsg ...string) error {
	return s.repo.StopScan(ctx, id, errMsg...)
}

// CompleteScan marks a scan as successfully completed.
func (s *ScanService) CompleteScan(ctx context.Context, id uuid.UUID) error {
	return s.repo.CompleteScan(ctx, id)
}

// GetScanResults retrieves paginated port-scan results for a given scan.
func (s *ScanService) GetScanResults(
	ctx context.Context,
	scanID uuid.UUID,
	offset, limit int,
) ([]*db.ScanResult, int64, error) {
	return s.repo.GetScanResults(ctx, scanID, offset, limit)
}

// GetScanSummary retrieves aggregated statistics for a given scan.
func (s *ScanService) GetScanSummary(ctx context.Context, scanID uuid.UUID) (*db.ScanSummary, error) {
	return s.repo.GetScanSummary(ctx, scanID)
}

// GetProfile retrieves a scan profile by its string ID.
func (s *ScanService) GetProfile(ctx context.Context, id string) (*db.ScanProfile, error) {
	return s.repo.GetProfile(ctx, id)
}

// portKey is used to index port scan results by (port, protocol).
type portKey struct {
	port     int
	protocol string
}

// diffStatusOrder maps a diff status to a sort priority (lower = first).
var diffStatusOrder = map[string]int{
	db.DiffStatusNew:       0,
	db.DiffStatusClosed:    1,
	db.DiffStatusChanged:   2,
	db.DiffStatusUnchanged: 3,
}

// GetScanDiff computes the diff between two scans of the same host.
// It returns all ports classified as new, closed, changed, or unchanged.
func (s *ScanService) GetScanDiff(ctx context.Context, scanAID, scanBID uuid.UUID) (*db.ScanDiff, error) {
	// Verify both scans exist by looking up results (GetScan is also an option
	// but GetAllScanResults covers both existence and data in one call).
	resultsA, err := s.repo.GetAllScanResults(ctx, scanAID)
	if err != nil {
		return nil, fmt.Errorf("get scan A results: %w", err)
	}

	resultsB, err := s.repo.GetAllScanResults(ctx, scanBID)
	if err != nil {
		return nil, fmt.Errorf("get scan B results: %w", err)
	}

	// Resolve host IDs.
	hostA, hostB, err := resolveHostIDs(ctx, s.repo, scanAID, scanBID, resultsA, resultsB)
	if err != nil {
		return nil, err
	}

	// Build indexes.
	indexA := indexResults(resultsA)
	indexB := indexResults(resultsB)

	diff := &db.ScanDiff{
		ScanAID: scanAID,
		ScanBID: scanBID,
		HostID:  hostA,
		Ports:   make([]db.ScanDiffEntry, 0),
	}

	// Ports present in scan B.
	for key, b := range indexB {
		entry := buildDiffEntry(b, indexA[key])
		diff.Ports = append(diff.Ports, entry)
		incrementCount(diff, entry.Status)
	}

	// Ports present only in scan A → closed.
	for key, a := range indexA {
		if _, inB := indexB[key]; !inB {
			svcName := ptrString(a.Service)
			entry := db.ScanDiffEntry{
				Port:               a.Port,
				Protocol:           a.Protocol,
				State:              a.State,
				ServiceName:        nil,
				ServiceVersion:     nil,
				Status:             db.DiffStatusClosed,
				PrevState:          ptrString(a.State),
				PrevServiceName:    svcName,
				PrevServiceVersion: nil,
			}
			diff.Ports = append(diff.Ports, entry)
			diff.ClosedCount++
		}
	}

	// Sort: new → closed → changed → unchanged, then by port number.
	sortDiffEntries(diff.Ports)

	// OS diff: compare OS from first result in each scan.
	diff.OSChanged, diff.PrevOSName, diff.CurrOSName = computeOSDiff(resultsA, resultsB)

	// Use host B for HostID when host A is nil (scan A had no results).
	if diff.HostID == (uuid.UUID{}) {
		diff.HostID = hostB
	}

	return diff, nil
}

// resolveHostIDs resolves and validates host IDs from both scans.
func resolveHostIDs(
	ctx context.Context,
	repo scanRepository,
	scanAID, scanBID uuid.UUID,
	resultsA, resultsB []*db.ScanResult,
) (hostA, hostB uuid.UUID, err error) {
	// Prefer host from results (already loaded); fall back to DB query.
	if len(resultsA) > 0 {
		hostA = resultsA[0].HostID
	} else {
		var err error
		hostA, err = repo.GetHostForScan(ctx, scanAID)
		if err != nil && !isNoRows(err) {
			return uuid.Nil, uuid.Nil, fmt.Errorf("get host for scan A: %w", err)
		}
	}

	if len(resultsB) > 0 {
		hostB = resultsB[0].HostID
	} else {
		var err error
		hostB, err = repo.GetHostForScan(ctx, scanBID)
		if err != nil && !isNoRows(err) {
			return uuid.Nil, uuid.Nil, fmt.Errorf("get host for scan B: %w", err)
		}
	}

	if hostA != (uuid.UUID{}) && hostB != (uuid.UUID{}) && hostA != hostB {
		return uuid.Nil, uuid.Nil,
			errors.NewScanError(errors.CodeValidation, "scans belong to different hosts")
	}
	return hostA, hostB, nil
}

// isNoRows reports whether err wraps sql.ErrNoRows.
func isNoRows(err error) bool {
	return stderrors.Is(err, sql.ErrNoRows)
}

// indexResults builds a map from (port, protocol) → *ScanResult.
func indexResults(results []*db.ScanResult) map[portKey]*db.ScanResult {
	idx := make(map[portKey]*db.ScanResult, len(results))
	for _, r := range results {
		idx[portKey{r.Port, r.Protocol}] = r
	}
	return idx
}

// ptrString returns a non-nil pointer to s, or nil when s is empty.
func ptrString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// buildDiffEntry constructs a ScanDiffEntry comparing b (current) against a (previous).
// a may be nil when the port is new.
func buildDiffEntry(b, a *db.ScanResult) db.ScanDiffEntry {
	entry := db.ScanDiffEntry{
		Port:        b.Port,
		Protocol:    b.Protocol,
		State:       b.State,
		ServiceName: ptrString(b.Service),
	}

	if a == nil {
		entry.Status = db.DiffStatusNew
		return entry
	}

	// Compare state and service.
	if a.State != b.State || a.Service != b.Service {
		entry.Status = db.DiffStatusChanged
		entry.PrevState = ptrString(a.State)
		entry.PrevServiceName = ptrString(a.Service)
	} else {
		entry.Status = db.DiffStatusUnchanged
	}
	return entry
}

// incrementCount updates the appropriate counter on diff.
func incrementCount(diff *db.ScanDiff, status string) {
	switch status {
	case db.DiffStatusNew:
		diff.NewCount++
	case db.DiffStatusClosed:
		diff.ClosedCount++
	case db.DiffStatusChanged:
		diff.ChangedCount++
	case db.DiffStatusUnchanged:
		diff.UnchangedCount++
	}
}

// sortDiffEntries sorts entries by status priority then port number.
func sortDiffEntries(entries []db.ScanDiffEntry) {
	n := len(entries)
	for i := 1; i < n; i++ {
		for j := i; j > 0; j-- {
			a, b := entries[j-1], entries[j]
			aOrd := diffStatusOrder[a.Status]
			bOrd := diffStatusOrder[b.Status]
			if aOrd > bOrd || (aOrd == bOrd && a.Port > b.Port) {
				entries[j-1], entries[j] = entries[j], entries[j-1]
			} else {
				break
			}
		}
	}
}

// computeOSDiff compares the OS observed in two scan result sets.
func computeOSDiff(resultsA, resultsB []*db.ScanResult) (changed bool, prevOS, currOS *string) {
	prevName := ""
	currName := ""
	if len(resultsA) > 0 {
		prevName = resultsA[0].OSName
	}
	if len(resultsB) > 0 {
		currName = resultsB[0].OSName
	}
	if prevName == currName {
		return false, nil, nil
	}
	return true, ptrString(prevName), ptrString(currName)
}

// -- Input validation --

// validateScanInput checks all fields of a CreateScanInput before the record
// is persisted. Returns a *errors.ScanError with CodeValidation on failure.
func validateScanInput(input db.CreateScanInput) error {
	if input.Name == "" {
		return errors.NewScanError(errors.CodeValidation, "scan name is required")
	}
	if len(input.Name) > MaxScanNameLength {
		return errors.NewScanError(errors.CodeValidation,
			fmt.Sprintf("scan name too long (max %d characters)", MaxScanNameLength))
	}

	if len(input.Targets) == 0 {
		return errors.NewScanError(errors.CodeValidation, "at least one target is required")
	}
	if len(input.Targets) > MaxTargetCount {
		return errors.NewScanError(errors.CodeValidation,
			fmt.Sprintf("too many targets (max %d)", MaxTargetCount))
	}

	for i, target := range input.Targets {
		if target == "" {
			return errors.NewScanError(errors.CodeValidation,
				fmt.Sprintf("target %d is empty", i+1))
		}
		if len(target) > MaxTargetLength {
			return errors.NewScanError(errors.CodeValidation,
				fmt.Sprintf("target %d too long (max %d characters)", i+1, MaxTargetLength))
		}
		if !IsValidScanTarget(target) {
			return errors.NewScanError(errors.CodeValidation,
				fmt.Sprintf("target %d: %q is not a valid IP address, CIDR range, or hostname", i+1, target))
		}
	}

	if !validScanTypes[input.ScanType] {
		return errors.NewScanError(errors.CodeValidation,
			fmt.Sprintf("invalid scan type: %s", input.ScanType))
	}

	if input.Ports == "" {
		return errors.NewScanError(errors.CodeValidation, "ports is required")
	}
	if err := ParsePortSpec(input.Ports); err != nil {
		return errors.NewScanError(errors.CodeValidation, err.Error())
	}

	return nil
}

// -- Port-spec parsing (exported so handlers and tests can reuse) --

// ParsePortSpec validates a port specification string.
// The spec is comma-separated with optional T:/U: protocol prefixes and
// optional hyphenated ranges (e.g. "T:80,U:53,1024-9999").
// Every individual port value must be in the range 1-65535.
func ParsePortSpec(ports string) error {
	for _, token := range strings.Split(ports, ",") {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		if err := parsePortToken(token); err != nil {
			return err
		}
	}
	return nil
}

// parsePortToken validates a single port token (after comma-splitting).
// It strips an optional T:/U: prefix, rejects whitespace, then validates the
// port number or range.
func parsePortToken(token string) error {
	// Strip optional protocol prefix (T: or U:).
	if len(token) >= 2 && (token[0] == 'T' || token[0] == 'U') && token[1] == ':' {
		token = token[2:]
	}
	// Reject tokens containing whitespace (e.g. "80 - 443").
	if strings.ContainsAny(token, " 	") {
		return fmt.Errorf("invalid port spec %q: whitespace not allowed", token)
	}
	parts := strings.SplitN(token, "-", 2)
	if len(parts) == 2 {
		return parsePortRange(parts[0], parts[1])
	}
	return validatePortNumber(parts[0])
}

// parsePortRange validates that start and end are valid ports and start <= end.
func parsePortRange(startStr, endStr string) error {
	startNum, err := strconv.Atoi(startStr)
	if err != nil {
		return fmt.Errorf("invalid port %q: must be a number", startStr)
	}
	if startNum < 1 || startNum > 65535 {
		return fmt.Errorf("invalid port %d: must be between 1 and 65535", startNum)
	}
	endNum, err := strconv.Atoi(endStr)
	if err != nil {
		return fmt.Errorf("invalid port %q: must be a number", endStr)
	}
	if endNum < 1 || endNum > 65535 {
		return fmt.Errorf("invalid port %d: must be between 1 and 65535", endNum)
	}
	if startNum > endNum {
		return fmt.Errorf("invalid port range %d-%d: start must be <= end", startNum, endNum)
	}
	return nil
}

// validatePortNumber checks that s is a valid port-number string (1-65535).
func validatePortNumber(s string) error {
	portNum, err := strconv.Atoi(s)
	if err != nil {
		return fmt.Errorf("invalid port %q: must be a number", s)
	}
	if portNum < 1 || portNum > 65535 {
		return fmt.Errorf("invalid port %d: must be between 1 and 65535", portNum)
	}
	return nil
}
