// Package handlers provides HTTP request handlers for the Scanorama API.
// This file implements the scan export endpoint that streams scans as CSV or JSON.
package handlers

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/anstrom/scanorama/internal/db"
)

// ExportScans handles GET /api/v1/scans/export?format=csv|json
// Streams all scans matching the active filters without pagination.
// Uses chunked reads (exportPageSize rows per page) to avoid buffering all rows.
func (h *ScanHandler) ExportScans(w http.ResponseWriter, r *http.Request) {
	format := r.URL.Query().Get("format")
	if format != exportFormatCSV && format != exportFormatJSON {
		format = exportDefaultFormat
	}

	filters := h.getScanFilters(r)
	filename := fmt.Sprintf("scans-%s.%s", time.Now().Format("2006-01-02"), format)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))

	if format == exportFormatCSV {
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		h.exportScansCSV(w, r.Context(), filters)
	} else {
		w.Header().Set("Content-Type", "application/json")
		h.exportScansJSON(w, r.Context(), filters)
	}
}

// exportScansCSV streams scans in CSV format, paging through the DB in chunks.
func (h *ScanHandler) exportScansCSV(w http.ResponseWriter, ctx context.Context, filters db.ScanFilters) {
	cw := csv.NewWriter(w)

	// Write header row.
	header := []string{
		"scan_id", "targets", "profile_id", responseKeyStatus,
		"started_at", "duration", "ports_scanned",
	}
	if err := cw.Write(header); err != nil {
		return
	}

	offset := 0
	for {
		scans, _, err := h.service.ListScans(ctx, filters, offset, exportPageSize)
		if err != nil {
			h.logger.Error("export scans read error", "offset", offset, "error", err)
			break
		}
		if len(scans) == 0 {
			break
		}

		for _, scan := range scans {
			if err := cw.Write(scanToCSVRow(scan)); err != nil {
				return
			}
		}
		cw.Flush()

		if len(scans) < exportPageSize {
			break
		}
		offset += exportPageSize
	}

	cw.Flush()
}

// exportScansJSON streams scans as a JSON array, paging through the DB in chunks.
func (h *ScanHandler) exportScansJSON(w http.ResponseWriter, ctx context.Context, filters db.ScanFilters) {
	enc := json.NewEncoder(w)
	_, _ = fmt.Fprint(w, "[")

	offset := 0
	first := true
	for {
		scans, _, err := h.service.ListScans(ctx, filters, offset, exportPageSize)
		if err != nil {
			h.logger.Error("export scans read error", "offset", offset, "error", err)
			break
		}
		if len(scans) == 0 {
			break
		}

		for _, scan := range scans {
			if !first {
				_, _ = fmt.Fprint(w, ",")
			}
			first = false
			if encErr := enc.Encode(scanToExportJSON(scan)); encErr != nil {
				return
			}
		}

		if len(scans) < exportPageSize {
			break
		}
		offset += exportPageSize
	}

	_, _ = fmt.Fprint(w, "]")
}

// scanExportRow is the JSON shape for a single exported scan.
type scanExportRow struct {
	ScanID       string `json:"scan_id"`
	Targets      string `json:"targets"`
	ProfileID    string `json:"profile_id"`
	Status       string `json:"status"`
	StartedAt    string `json:"started_at"`
	Duration     string `json:"duration"`
	PortsScanned string `json:"ports_scanned"`
}

func scanToExportJSON(scan *db.Scan) scanExportRow {
	row := scanExportRow{
		ScanID:  scan.ID.String(),
		Targets: strings.Join(scan.Targets, ";"),
		Status:  scan.Status,
	}
	if scan.ProfileID != nil {
		row.ProfileID = *scan.ProfileID
	}
	if scan.StartedAt != nil {
		row.StartedAt = scan.StartedAt.UTC().Format(time.RFC3339)
	}
	if scan.DurationStr != nil {
		row.Duration = *scan.DurationStr
	}
	if scan.PortsScanned != nil {
		row.PortsScanned = *scan.PortsScanned
	}
	return row
}

func scanToCSVRow(scan *db.Scan) []string {
	profileID := ""
	if scan.ProfileID != nil {
		profileID = *scan.ProfileID
	}
	startedAt := ""
	if scan.StartedAt != nil {
		startedAt = scan.StartedAt.UTC().Format(time.RFC3339)
	}
	duration := ""
	if scan.DurationStr != nil {
		duration = *scan.DurationStr
	}
	portsScanned := ""
	if scan.PortsScanned != nil {
		portsScanned = *scan.PortsScanned
	}
	return []string{
		scan.ID.String(),
		strings.Join(scan.Targets, ";"),
		profileID,
		scan.Status,
		startedAt,
		duration,
		portsScanned,
	}
}
