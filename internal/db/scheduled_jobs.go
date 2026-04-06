// Package db provides scheduled job database operations for scanorama.
package db

import (
	"time"

	"github.com/google/uuid"
)

// Schedule represents a scheduled task.
type Schedule struct {
	ID             uuid.UUID              `json:"id" db:"id"`
	Name           string                 `json:"name" db:"name"`
	Description    string                 `json:"description" db:"description"`
	CronExpression string                 `json:"cron_expression" db:"cron_expression"`
	JobType        string                 `json:"job_type" db:"job_type"`
	JobConfig      map[string]interface{} `json:"job_config" db:"job_config"`
	Enabled        bool                   `json:"enabled" db:"enabled"`
	CreatedAt      time.Time              `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at" db:"updated_at"`
	LastRun        *time.Time             `json:"last_run" db:"last_run"`
	NextRun        *time.Time             `json:"next_run" db:"next_run"`
}

// ScheduleFilters represents filters for listing schedules.
type ScheduleFilters struct {
	Enabled   bool
	JobType   string
	SortBy    string
	SortOrder string
}
