// Package services provides business logic services for Scanorama.
// This file implements network management functionality including
// config-based network seeding, exclusion management, and integration
// with the discovery engine.
package services

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net"
	"strings"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/anstrom/scanorama/internal/config"
	"github.com/anstrom/scanorama/internal/db"
)

// NetworkService manages network configuration and exclusions.
type NetworkService struct {
	database *db.DB
}

// NewNetworkService creates a new network service.
func NewNetworkService(database *db.DB) *NetworkService {
	return &NetworkService{
		database: database,
	}
}

// NetworkWithExclusions represents a network with its exclusions.
type NetworkWithExclusions struct {
	Network    *db.Network
	Exclusions []*db.NetworkExclusion
}

// SeedNetworksFromConfig creates or updates networks based on config.
func (s *NetworkService) SeedNetworksFromConfig(ctx context.Context, cfg *config.Config) error {
	if !cfg.Discovery.AutoSeed {
		log.Printf("Network auto-seeding disabled in config")
		return nil
	}

	log.Printf("Seeding %d networks from config", len(cfg.Discovery.Networks))

	// Start transaction for atomic operation
	tx, err := s.database.Beginx()
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
			log.Printf("Warning: failed to rollback transaction: %v", err)
		}
	}()

	// First, create/update networks
	for i := range cfg.Discovery.Networks {
		netConfig := &cfg.Discovery.Networks[i]
		if err := s.validateNetworkConfig(netConfig); err != nil {
			return fmt.Errorf("invalid network config %s: %w", netConfig.Name, err)
		}

		if err := s.upsertNetworkFromConfig(ctx, tx, netConfig, cfg.Discovery.Defaults); err != nil {
			return fmt.Errorf("failed to upsert network %s: %w", netConfig.Name, err)
		}
	}

	// Then, create global exclusions
	if err := s.upsertGlobalExclusions(ctx, tx, cfg.Discovery.GlobalExclusions); err != nil {
		return fmt.Errorf("failed to upsert global exclusions: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit network seeding: %w", err)
	}

	log.Printf("Successfully seeded %d networks from config", len(cfg.Discovery.Networks))
	return nil
}

// upsertNetworkFromConfig creates or updates a network from config.
func (s *NetworkService) upsertNetworkFromConfig(
	ctx context.Context,
	tx *sqlx.Tx,
	netConfig *config.NetworkConfig,
	defaults config.DiscoveryDefaults,
) error {
	// Use defaults if not specified
	method := netConfig.Method
	if method == "" {
		method = defaults.Method
	}

	// Upsert network
	query := `
		INSERT INTO networks (
			name,
			cidr,
			description,
			discovery_method,
			is_active,
			scan_enabled
		) VALUES (
			$1, $2, $3, $4, $5, true
		)
		ON CONFLICT (name) DO UPDATE SET
			cidr = EXCLUDED.cidr,
			description = EXCLUDED.description,
			discovery_method = EXCLUDED.discovery_method,
			is_active = EXCLUDED.is_active,
			updated_at = NOW()
		RETURNING id`

	var networkID uuid.UUID
	err := tx.QueryRowContext(ctx, query,
		netConfig.Name,
		netConfig.CIDR,
		netConfig.Description,
		method,
		netConfig.Enabled,
	).Scan(&networkID)

	if err != nil {
		return fmt.Errorf("failed to upsert network: %w", err)
	}

	// Create network-specific exclusions
	if len(netConfig.Exclusions) > 0 {
		if err := s.upsertNetworkExclusions(ctx, tx, networkID, netConfig.Exclusions); err != nil {
			return fmt.Errorf("failed to upsert exclusions: %w", err)
		}
	}

	log.Printf("Upserted network '%s' (%s) with %d exclusions",
		netConfig.Name, netConfig.CIDR, len(netConfig.Exclusions))
	return nil
}

// upsertGlobalExclusions creates or updates global exclusions.
func (s *NetworkService) upsertGlobalExclusions(ctx context.Context, tx *sqlx.Tx, exclusions []string) error {
	if len(exclusions) == 0 {
		return nil
	}

	// Remove existing global exclusions that are not in the config
	deleteQuery := `
		DELETE FROM network_exclusions
		WHERE network_id IS NULL`
	if _, err := tx.ExecContext(ctx, deleteQuery); err != nil {
		return fmt.Errorf("failed to clean global exclusions: %w", err)
	}

	// Insert new global exclusions
	for _, exclusion := range exclusions {
		normalizedCIDR, err := s.normalizeCIDR(exclusion)
		if err != nil {
			log.Printf("Warning: invalid exclusion CIDR '%s', skipping: %v", exclusion, err)
			continue
		}

		insertQuery := `
			INSERT INTO network_exclusions (
				network_id,
				excluded_cidr,
				reason,
				enabled
			) VALUES (
				NULL, $1, $2, true
			)
			ON CONFLICT (network_id, excluded_cidr)
			WHERE enabled = true
			DO NOTHING`

		if _, err := tx.ExecContext(ctx, insertQuery, normalizedCIDR, "Global exclusion from config"); err != nil {
			return fmt.Errorf("failed to insert global exclusion %s: %w", normalizedCIDR, err)
		}
	}

	log.Printf("Upserted %d global exclusions", len(exclusions))
	return nil
}

// upsertNetworkExclusions creates or updates network-specific exclusions.
func (s *NetworkService) upsertNetworkExclusions(
	ctx context.Context,
	tx *sqlx.Tx,
	networkID uuid.UUID,
	exclusions []string,
) error {
	if len(exclusions) == 0 {
		return nil
	}

	// Remove existing exclusions for this network that are not in the config
	deleteQuery := `
		DELETE FROM network_exclusions
		WHERE network_id = $1`
	if _, err := tx.ExecContext(ctx, deleteQuery, networkID); err != nil {
		return fmt.Errorf("failed to clean network exclusions: %w", err)
	}

	// Insert new exclusions
	for _, exclusion := range exclusions {
		normalizedCIDR, err := s.normalizeCIDR(exclusion)
		if err != nil {
			log.Printf("Warning: invalid exclusion CIDR '%s', skipping: %v", exclusion, err)
			continue
		}

		insertQuery := `
			INSERT INTO network_exclusions (
				network_id,
				excluded_cidr,
				reason,
				enabled
			) VALUES (
				$1, $2, $3, true
			)`

		if _, err := tx.ExecContext(ctx, insertQuery, networkID, normalizedCIDR,
			"Network exclusion from config"); err != nil {
			return fmt.Errorf("failed to insert network exclusion %s: %w", normalizedCIDR, err)
		}
	}

	return nil
}

// GetNetworkByName retrieves a network by name with its exclusions.
func (s *NetworkService) GetNetworkByName(ctx context.Context, name string) (*NetworkWithExclusions, error) {
	// Get network
	query := `
		SELECT
			id, name, cidr, description, discovery_method,
			is_active, scan_enabled, last_discovery, last_scan,
			host_count, active_host_count, created_at, updated_at, created_by
		FROM networks
		WHERE name = $1`

	network := &db.Network{}
	err := s.database.GetContext(ctx, network, query, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get network %s: %w", name, err)
	}

	// Get exclusions
	exclusions, err := s.getNetworkExclusions(ctx, &network.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get exclusions for network %s: %w", name, err)
	}

	return &NetworkWithExclusions{
		Network:    network,
		Exclusions: exclusions,
	}, nil
}

// GetNetworkByID retrieves a network by ID with its exclusions.
func (s *NetworkService) GetNetworkByID(ctx context.Context, id uuid.UUID) (*NetworkWithExclusions, error) {
	// Get network
	query := `
		SELECT
			id, name, cidr, description, discovery_method,
			is_active, scan_enabled, last_discovery, last_scan,
			host_count, active_host_count, created_at, updated_at, created_by
		FROM networks
		WHERE id = $1`

	network := &db.Network{}
	err := s.database.GetContext(ctx, network, query, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get network %s: %w", id, err)
	}

	// Get exclusions
	exclusions, err := s.getNetworkExclusions(ctx, &id)
	if err != nil {
		return nil, fmt.Errorf("failed to get exclusions for network %s: %w", id, err)
	}

	return &NetworkWithExclusions{
		Network:    network,
		Exclusions: exclusions,
	}, nil
}

// GetActiveNetworks returns all active networks with their exclusions.
func (s *NetworkService) GetActiveNetworks(ctx context.Context) ([]*NetworkWithExclusions, error) {
	// Get active networks
	query := `
		SELECT
			id, name, cidr, description, discovery_method,
			is_active, scan_enabled, last_discovery, last_scan,
			host_count, active_host_count, created_at, updated_at, created_by
		FROM networks
		WHERE is_active = true
		ORDER BY name`

	var networks []*db.Network
	err := s.database.SelectContext(ctx, &networks, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list active networks: %w", err)
	}

	// Get exclusions for each network
	result := make([]*NetworkWithExclusions, 0, len(networks))
	for _, network := range networks {
		exclusions, err := s.getNetworkExclusions(ctx, &network.ID)
		if err != nil {
			log.Printf("Warning: failed to get exclusions for network %s: %v", network.Name, err)
			exclusions = []*db.NetworkExclusion{} // Continue with empty exclusions
		}

		result = append(result, &NetworkWithExclusions{
			Network:    network,
			Exclusions: exclusions,
		})
	}

	return result, nil
}

// GetGlobalExclusions returns all global exclusions.
func (s *NetworkService) GetGlobalExclusions(ctx context.Context) ([]*db.NetworkExclusion, error) {
	return s.getNetworkExclusions(ctx, nil) // nil = global exclusions
}

// getNetworkExclusions retrieves exclusions for a network (or global if networkID is nil).
func (s *NetworkService) getNetworkExclusions(
	ctx context.Context,
	networkID *uuid.UUID,
) ([]*db.NetworkExclusion, error) {
	var query string
	var args []interface{}

	if networkID == nil {
		// Global exclusions
		query = `
			SELECT
				id, network_id, excluded_cidr::text, reason, enabled,
				created_at, updated_at, created_by
			FROM network_exclusions
			WHERE network_id IS NULL AND enabled = true
			ORDER BY excluded_cidr`
		args = []interface{}{}
	} else {
		// Network-specific exclusions
		query = `
			SELECT
				id, network_id, excluded_cidr::text, reason, enabled,
				created_at, updated_at, created_by
			FROM network_exclusions
			WHERE network_id = $1 AND enabled = true
			ORDER BY excluded_cidr`
		args = []interface{}{*networkID}
	}

	var exclusions []*db.NetworkExclusion
	err := s.database.SelectContext(ctx, &exclusions, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query exclusions: %w", err)
	}

	return exclusions, nil
}

// GenerateTargetsForNetwork generates valid scan targets for a network, applying all exclusions.
func (s *NetworkService) GenerateTargetsForNetwork(
	ctx context.Context,
	networkID uuid.UUID,
	maxHosts int,
) ([]string, error) {
	// Get the network first to get its CIDR
	network, err := s.GetNetworkByID(ctx, networkID)
	if err != nil {
		return nil, fmt.Errorf("failed to get network: %w", err)
	}

	// Use the enhanced database function that handles exclusions
	query := `
		SELECT ip_address::text
		FROM generate_host_ips_with_exclusions($1::CIDR, $2, $3)
		ORDER BY ip_address`

	rows, err := s.database.QueryContext(ctx, query, network.Network.CIDR.String(), networkID, maxHosts)
	if err != nil {
		return nil, fmt.Errorf("failed to generate targets: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("Warning: failed to close rows: %v", err)
		}
	}()

	var targets []string
	for rows.Next() {
		var ip string
		if err := rows.Scan(&ip); err != nil {
			log.Printf("Error scanning IP address: %v", err)
			continue
		}
		// Remove the /xx suffix from PostgreSQL inet output
		if idx := strings.Index(ip, "/"); idx != -1 {
			ip = ip[:idx]
		}
		targets = append(targets, ip)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating target IPs: %w", err)
	}

	log.Printf("Generated %d targets for network %s (max: %d)", len(targets), network.Network.Name, maxHosts)
	return targets, nil
}

// AddExclusion adds a new exclusion rule.
func (s *NetworkService) AddExclusion(
	ctx context.Context,
	networkID *uuid.UUID,
	cidr, reason string,
) (*db.NetworkExclusion, error) {
	normalizedCIDR, err := s.normalizeCIDR(cidr)
	if err != nil {
		return nil, fmt.Errorf("invalid CIDR or IP address: %s: %w", cidr, err)
	}

	query := `
		INSERT INTO network_exclusions (
			network_id,
			excluded_cidr,
			reason,
			enabled
		) VALUES (
			$1, $2, $3, true
		)
		RETURNING
			id, network_id, excluded_cidr::text, reason, enabled,
			created_at, updated_at, created_by`

	exclusion := &db.NetworkExclusion{}
	err = s.database.QueryRowContext(ctx, query, networkID, normalizedCIDR, reason).Scan(
		&exclusion.ID,
		&exclusion.NetworkID,
		&exclusion.ExcludedCIDR,
		&exclusion.Reason,
		&exclusion.Enabled,
		&exclusion.CreatedAt,
		&exclusion.UpdatedAt,
		&exclusion.CreatedBy,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to insert exclusion: %w", err)
	}

	return exclusion, nil
}

// RemoveExclusion removes an exclusion rule.
func (s *NetworkService) RemoveExclusion(ctx context.Context, exclusionID uuid.UUID) error {
	query := `
		DELETE FROM network_exclusions
		WHERE id = $1`

	result, err := s.database.ExecContext(ctx, query, exclusionID)
	if err != nil {
		return fmt.Errorf("failed to delete exclusion: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("exclusion not found")
	}

	return nil
}

// UpdateNetworkDiscoveryTime updates the last discovery timestamp for a network.
func (s *NetworkService) UpdateNetworkDiscoveryTime(
	ctx context.Context,
	networkID uuid.UUID,
	discoveredHosts, activeHosts int,
) error {
	query := `
		UPDATE networks
		SET
			last_discovery = NOW(),
			host_count = $2,
			active_host_count = $3,
			updated_at = NOW()
		WHERE id = $1`

	_, err := s.database.ExecContext(ctx, query, networkID, discoveredHosts, activeHosts)
	if err != nil {
		return fmt.Errorf("failed to update network discovery time: %w", err)
	}

	return nil
}

// GetNetworkStats returns statistics about networks and exclusions.
func (s *NetworkService) GetNetworkStats(ctx context.Context) (map[string]interface{}, error) {
	statsQuery := `
		SELECT
			COUNT(*) as total_networks,
			COUNT(*) FILTER (WHERE is_active = true) as active_networks,
			COUNT(*) FILTER (WHERE scan_enabled = true) as scan_enabled_networks,
			COALESCE(SUM(host_count), 0) as total_hosts,
			COALESCE(SUM(active_host_count), 0) as total_active_hosts
		FROM networks`

	var stats struct {
		TotalNetworks       int `db:"total_networks"`
		ActiveNetworks      int `db:"active_networks"`
		ScanEnabledNetworks int `db:"scan_enabled_networks"`
		TotalHosts          int `db:"total_hosts"`
		TotalActiveHosts    int `db:"total_active_hosts"`
	}

	err := s.database.GetContext(ctx, &stats, statsQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to get network stats: %w", err)
	}

	// Get exclusion stats
	exclusionQuery := `
		SELECT
			COUNT(*) as total_exclusions,
			COUNT(*) FILTER (WHERE network_id IS NULL) as global_exclusions,
			COUNT(*) FILTER (WHERE network_id IS NOT NULL) as network_exclusions
		FROM network_exclusions
		WHERE enabled = true`

	var exclusionStats struct {
		TotalExclusions   int `db:"total_exclusions"`
		GlobalExclusions  int `db:"global_exclusions"`
		NetworkExclusions int `db:"network_exclusions"`
	}

	err = s.database.GetContext(ctx, &exclusionStats, exclusionQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to get exclusion stats: %w", err)
	}

	return map[string]interface{}{
		"networks": map[string]interface{}{
			"total":        stats.TotalNetworks,
			"active":       stats.ActiveNetworks,
			"scan_enabled": stats.ScanEnabledNetworks,
		},
		"hosts": map[string]interface{}{
			"total":  stats.TotalHosts,
			"active": stats.TotalActiveHosts,
		},
		"exclusions": map[string]interface{}{
			"total":   exclusionStats.TotalExclusions,
			"global":  exclusionStats.GlobalExclusions,
			"network": exclusionStats.NetworkExclusions,
		},
	}, nil
}

// validateNetworkConfig validates a network configuration.
func (s *NetworkService) validateNetworkConfig(netConfig *config.NetworkConfig) error {
	if netConfig.Name == "" {
		return fmt.Errorf("network name is required")
	}

	// Validate CIDR
	if _, _, err := net.ParseCIDR(netConfig.CIDR); err != nil {
		return fmt.Errorf("invalid CIDR %s: %w", netConfig.CIDR, err)
	}

	// Validate method
	validMethods := map[string]bool{
		"ping": true,
		"tcp":  true,
		"arp":  true,
		"icmp": true,
	}
	if netConfig.Method != "" && !validMethods[netConfig.Method] {
		return fmt.Errorf("invalid discovery method: %s", netConfig.Method)
	}

	// Validate exclusions
	for _, exclusion := range netConfig.Exclusions {
		if _, err := s.normalizeCIDR(exclusion); err != nil {
			return fmt.Errorf("invalid exclusion CIDR or IP: %s: %w", exclusion, err)
		}
	}

	return nil
}

// normalizeCIDR converts IP addresses to CIDR notation and validates CIDR ranges.
func (s *NetworkService) normalizeCIDR(cidr string) (string, error) {
	// Try to parse as CIDR first
	if _, _, err := net.ParseCIDR(cidr); err == nil {
		return cidr, nil
	}

	// Try to parse as single IP
	if ip := net.ParseIP(cidr); ip != nil {
		// Convert single IP to /32 CIDR
		if ip.To4() != nil {
			return cidr + "/32", nil
		}
		return cidr + "/128", nil // IPv6
	}

	return "", fmt.Errorf("invalid CIDR or IP address: %s", cidr)
}

// CreateNetwork creates a new network.
func (s *NetworkService) CreateNetwork(
	ctx context.Context,
	name, cidr, description, method string,
	enabled bool,
) (*db.Network, error) {
	// Validate CIDR
	if _, _, err := net.ParseCIDR(cidr); err != nil {
		return nil, fmt.Errorf("invalid CIDR %s: %w", cidr, err)
	}

	// Validate method
	validMethods := map[string]bool{
		"ping": true,
		"tcp":  true,
		"arp":  true,
		"icmp": true,
	}
	if !validMethods[method] {
		return nil, fmt.Errorf("invalid discovery method: %s", method)
	}

	query := `
		INSERT INTO networks (
			name,
			cidr,
			description,
			discovery_method,
			is_active,
			scan_enabled
		) VALUES (
			$1, $2, $3, $4, $5, true
		)
		RETURNING
			id, name, cidr, description, discovery_method,
			is_active, scan_enabled, last_discovery, last_scan,
			host_count, active_host_count, created_at, updated_at, created_by`

	network := &db.Network{}
	err := s.database.QueryRowContext(ctx, query, name, cidr, description, method, enabled).Scan(
		&network.ID,
		&network.Name,
		&network.CIDR,
		&network.Description,
		&network.DiscoveryMethod,
		&network.IsActive,
		&network.ScanEnabled,
		&network.LastDiscovery,
		&network.LastScan,
		&network.HostCount,
		&network.ActiveHostCount,
		&network.CreatedAt,
		&network.UpdatedAt,
		&network.CreatedBy,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to insert network: %w", err)
	}

	return network, nil
}

// UpdateNetwork updates an existing network.
func (s *NetworkService) UpdateNetwork(
	ctx context.Context,
	id uuid.UUID,
	name, cidr, description, method string,
	enabled bool,
) (*db.Network, error) {
	// Validate CIDR
	if _, _, err := net.ParseCIDR(cidr); err != nil {
		return nil, fmt.Errorf("invalid CIDR %s: %w", cidr, err)
	}

	// Validate method
	validMethods := map[string]bool{
		"ping": true,
		"tcp":  true,
		"arp":  true,
		"icmp": true,
	}
	if !validMethods[method] {
		return nil, fmt.Errorf("invalid discovery method: %s", method)
	}

	query := `
		UPDATE networks
		SET
			name = $2,
			cidr = $3,
			description = $4,
			discovery_method = $5,
			is_active = $6,
			updated_at = NOW()
		WHERE id = $1
		RETURNING
			id, name, cidr, description, discovery_method,
			is_active, scan_enabled, last_discovery, last_scan,
			host_count, active_host_count, created_at, updated_at, created_by`

	network := &db.Network{}
	err := s.database.QueryRowContext(ctx, query, id, name, cidr, description, method, enabled).Scan(
		&network.ID,
		&network.Name,
		&network.CIDR,
		&network.Description,
		&network.DiscoveryMethod,
		&network.IsActive,
		&network.ScanEnabled,
		&network.LastDiscovery,
		&network.LastScan,
		&network.HostCount,
		&network.ActiveHostCount,
		&network.CreatedAt,
		&network.UpdatedAt,
		&network.CreatedBy,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to update network: %w", err)
	}

	return network, nil
}

// DeleteNetwork deletes a network and all its exclusions.
func (s *NetworkService) DeleteNetwork(ctx context.Context, id uuid.UUID) error {
	query := `
		DELETE FROM networks
		WHERE id = $1`

	result, err := s.database.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete network: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("network not found")
	}

	return nil
}

// ListNetworks returns all networks with optional filtering.
func (s *NetworkService) ListNetworks(ctx context.Context, activeOnly bool) ([]*db.Network, error) {
	query := `
		SELECT
			id, name, cidr, description, discovery_method,
			is_active, scan_enabled, last_discovery, last_scan,
			host_count, active_host_count, created_at, updated_at, created_by
		FROM networks`

	if activeOnly {
		query += ` WHERE is_active = true`
	}

	query += ` ORDER BY name`

	var networks []*db.Network
	err := s.database.SelectContext(ctx, &networks, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list networks: %w", err)
	}

	return networks, nil
}
