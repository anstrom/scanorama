// Package handlers provides HTTP request handlers for the Scanorama API.
// This file implements worker management handler methods for the AdminHandler type.
package handlers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
)

// Worker status constants.
const (
	workerStatusActive = "active"
	workerStatusIdle   = "idle"
)

// GetWorkerStatus handles GET /api/v1/admin/workers - get worker pool status.
func (h *AdminHandler) GetWorkerStatus(w http.ResponseWriter, r *http.Request) {
	requestID := getRequestIDFromContext(r.Context())
	h.logger.Info("Getting worker status", "request_id", requestID)

	var workers []WorkerInfo
	var totalProcessed int64
	var totalFailed int64
	var queueSize int

	if h.scanQueue != nil {
		stats := h.scanQueue.Stats()
		queueSize = stats.QueueDepth
		totalProcessed = stats.TotalCompleted
		totalFailed = stats.TotalFailed

		snaps := h.scanQueue.Snapshot()
		workers = make([]WorkerInfo, len(snaps))
		for i := range snaps {
			snap := &snaps[i]
			info := WorkerInfo{
				ID:            snap.ID,
				Status:        snap.Status,
				JobsProcessed: snap.JobsDone,
				JobsFailed:    snap.JobsFailed,
				StartTime:     snap.WorkerStartedAt,
				Uptime:        time.Since(snap.WorkerStartedAt),
				Metrics: map[string]int{
					"scans_completed": int(snap.JobsDone),
					"errors":          int(snap.JobsFailed),
				},
			}

			total := snap.JobsDone + snap.JobsFailed
			if total > 0 {
				info.ErrorRate = float64(snap.JobsFailed) / float64(total)
			}

			if !snap.LastJobAt.IsZero() {
				info.LastJobTime = &snap.LastJobAt
			}

			if snap.Status == workerStatusActive && snap.JobStartedAt != nil {
				info.CurrentJob = &JobInfo{
					ID:        snap.JobID,
					Type:      snap.JobType,
					Target:    snap.JobTarget,
					StartTime: *snap.JobStartedAt,
					Duration:  time.Since(*snap.JobStartedAt),
				}
			}

			workers[i] = info
		}
	} else {
		workers = []WorkerInfo{}
	}

	activeCount := 0
	idleCount := 0
	for i := range workers {
		switch workers[i].Status {
		case workerStatusActive:
			activeCount++
		case workerStatusIdle:
			idleCount++
		}
	}

	var overallErrorRate float64
	if total := totalProcessed + totalFailed; total > 0 {
		overallErrorRate = float64(totalFailed) / float64(total)
	}

	response := WorkerStatusResponse{
		TotalWorkers:  len(workers),
		ActiveWorkers: activeCount,
		IdleWorkers:   idleCount,
		QueueSize:     queueSize,
		ProcessedJobs: totalProcessed,
		FailedJobs:    totalFailed,
		Workers:       workers,
		Summary: map[string]interface{}{
			"total_scans_completed":     totalProcessed,
			"total_discovery_completed": 0,
			"overall_error_rate":        overallErrorRate,
		},
		Timestamp: time.Now().UTC(),
	}

	writeJSON(w, r, http.StatusOK, response)

	if h.metrics != nil {
		h.metrics.Counter("api_admin_worker_status_total", nil)
	}
}

// StopWorker handles POST /api/v1/admin/workers/{id}/stop - stop a specific worker.
func (h *AdminHandler) StopWorker(w http.ResponseWriter, r *http.Request) {
	requestID := getRequestIDFromContext(r.Context())

	// Extract and validate worker ID from URL — still returns 400 on missing ID.
	workerID, err := h.extractWorkerID(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	h.logger.Info("StopWorker called but not yet implemented",
		"request_id", requestID,
		"worker_id", workerID)

	// Worker management is not yet implemented.
	writeError(w, r, http.StatusNotImplemented,
		fmt.Errorf("stop worker is not yet implemented"))
}

// extractWorkerID extracts the worker ID from the URL path.
func (h *AdminHandler) extractWorkerID(r *http.Request) (string, error) {
	vars := mux.Vars(r)
	workerID, exists := vars["id"]
	if !exists {
		return "", fmt.Errorf("worker ID not provided")
	}

	if workerID == "" {
		return "", fmt.Errorf("worker ID cannot be empty")
	}

	return workerID, nil
}
