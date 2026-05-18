// Package handlers provides HTTP request handlers for the Scanorama API.
// This file implements the host export endpoint that streams hosts as CSV or JSON.
package handlers

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/anstrom/scanorama/internal/db"
)

const (
	exportPageSize      = 1000
	exportFormatCSV     = "csv"
	exportFormatJSON    = "json"
	exportDefaultFormat = exportFormatCSV

	// exportColHostname is the CSV column name for the hostname field.
	exportColHostname = "hostname"
)

// ExportHosts handles GET /api/v1/hosts/export?format=csv|json
// Streams all hosts matching the active filters without pagination.
// Uses chunked reads (exportPageSize rows per page) to avoid buffering all rows.
func (h *HostHandler) ExportHosts(w http.ResponseWriter, r *http.Request) {
	format := r.URL.Query().Get("format")
	if format != exportFormatCSV && format != exportFormatJSON {
		format = exportDefaultFormat
	}

	filters := h.getHostFilters(r)
	filename := fmt.Sprintf("hosts-%s.%s", time.Now().Format("2006-01-02"), format)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))

	if format == exportFormatCSV {
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		h.exportHostsCSV(w, r.Context(), filters)
	} else {
		w.Header().Set("Content-Type", "application/json")
		h.exportHostsJSON(w, r.Context(), filters)
	}
}

// exportHostsCSV streams hosts in CSV format, paging through the DB in chunks.
func (h *HostHandler) exportHostsCSV(w http.ResponseWriter, ctx context.Context, filters *db.HostFilters) {
	cw := csv.NewWriter(w)

	// Write header row.
	header := []string{
		"ip", exportColHostname, "mac", "vendor", "os",
		responseKeyStatus, "first_seen", "last_seen", "open_port_count",
	}
	if err := cw.Write(header); err != nil {
		return
	}

	offset := 0
	for {
		hosts, _, err := h.service.ListHosts(ctx, filters, offset, exportPageSize)
		if err != nil {
			h.logger.Error("export hosts read error", "offset", offset, "error", err)
			break
		}
		if len(hosts) == 0 {
			break
		}

		for _, host := range hosts {
			if err := cw.Write(hostToCSVRow(host)); err != nil {
				return
			}
		}
		cw.Flush()

		if len(hosts) < exportPageSize {
			break
		}
		offset += exportPageSize
	}

	cw.Flush()
}

// exportHostsJSON streams hosts as a JSON array, paging through the DB in chunks.
func (h *HostHandler) exportHostsJSON(w http.ResponseWriter, ctx context.Context, filters *db.HostFilters) {
	enc := json.NewEncoder(w)
	_, _ = fmt.Fprint(w, "[")

	offset := 0
	first := true
	for {
		hosts, _, err := h.service.ListHosts(ctx, filters, offset, exportPageSize)
		if err != nil {
			h.logger.Error("export hosts read error", "offset", offset, "error", err)
			break
		}
		if len(hosts) == 0 {
			break
		}

		for _, host := range hosts {
			if !first {
				_, _ = fmt.Fprint(w, ",")
			}
			first = false
			if encErr := enc.Encode(hostToExportJSON(host)); encErr != nil {
				return
			}
		}

		if len(hosts) < exportPageSize {
			break
		}
		offset += exportPageSize
	}

	_, _ = fmt.Fprint(w, "]")
}

// hostExportRow is the JSON shape for a single exported host.
type hostExportRow struct {
	IP            string `json:"ip"`
	Hostname      string `json:"hostname"`
	MAC           string `json:"mac"`
	Vendor        string `json:"vendor"`
	OS            string `json:"os"`
	Status        string `json:"status"`
	FirstSeen     string `json:"first_seen"`
	LastSeen      string `json:"last_seen"`
	OpenPortCount int    `json:"open_port_count"`
}

func hostToExportJSON(host *db.Host) hostExportRow {
	row := hostExportRow{
		IP:            host.IPAddress.String(),
		Status:        host.Status,
		FirstSeen:     host.FirstSeen.UTC().Format(time.RFC3339),
		LastSeen:      host.LastSeen.UTC().Format(time.RFC3339),
		OpenPortCount: host.TotalPorts,
	}
	if host.Hostname != nil {
		row.Hostname = *host.Hostname
	}
	if host.MACAddress != nil {
		row.MAC = host.MACAddress.String()
	}
	if host.Vendor != nil {
		row.Vendor = *host.Vendor
	}
	if host.OSFamily != nil {
		row.OS = *host.OSFamily
	}
	return row
}

func hostToCSVRow(host *db.Host) []string {
	hostname := ""
	if host.Hostname != nil {
		hostname = *host.Hostname
	}
	mac := ""
	if host.MACAddress != nil {
		mac = host.MACAddress.String()
	}
	vendor := ""
	if host.Vendor != nil {
		vendor = *host.Vendor
	}
	osFamily := ""
	if host.OSFamily != nil {
		osFamily = *host.OSFamily
	}
	return []string{
		host.IPAddress.String(),
		hostname,
		mac,
		vendor,
		osFamily,
		host.Status,
		host.FirstSeen.UTC().Format(time.RFC3339),
		host.LastSeen.UTC().Format(time.RFC3339),
		strconv.Itoa(host.TotalPorts),
	}
}
