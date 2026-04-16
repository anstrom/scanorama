package services

import (
	"context"
	"log/slog"

	"github.com/google/uuid"

	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/errors"
)

// deviceRepository defines data-access operations required by DeviceService.
type deviceRepository interface {
	ListDevices(ctx context.Context) ([]db.DeviceSummary, error)
	CreateDevice(ctx context.Context, input db.CreateDeviceInput) (*db.Device, error)
	GetDevice(ctx context.Context, id uuid.UUID) (*db.Device, error)
	GetDeviceDetail(ctx context.Context, id uuid.UUID) (*db.DeviceDetail, error)
	UpdateDevice(ctx context.Context, id uuid.UUID, input db.UpdateDeviceInput) (*db.Device, error)
	DeleteDevice(ctx context.Context, id uuid.UUID) error
	AttachHost(ctx context.Context, deviceID, hostID uuid.UUID) error
	DetachHost(ctx context.Context, deviceID, hostID uuid.UUID) error
	AcceptSuggestion(ctx context.Context, suggestionID uuid.UUID) error
	DismissSuggestion(ctx context.Context, suggestionID uuid.UUID) error
}

// DeviceService handles business logic for device management.
type DeviceService struct {
	repo   deviceRepository
	logger *slog.Logger
}

// NewDeviceService creates a new DeviceService.
func NewDeviceService(repo deviceRepository, logger *slog.Logger) *DeviceService {
	return &DeviceService{repo: repo, logger: logger.With("service", "device")}
}

// ListDevices returns all devices with MAC and host counts.
func (s *DeviceService) ListDevices(ctx context.Context) ([]db.DeviceSummary, error) {
	return s.repo.ListDevices(ctx)
}

// CreateDevice validates the input and creates a new device record.
func (s *DeviceService) CreateDevice(ctx context.Context, input db.CreateDeviceInput) (*db.Device, error) {
	if input.Name == "" {
		return nil, errors.NewScanError(errors.CodeValidation, "device name is required")
	}
	return s.repo.CreateDevice(ctx, input)
}

// GetDevice returns a device by its UUID.
func (s *DeviceService) GetDevice(ctx context.Context, id uuid.UUID) (*db.Device, error) {
	return s.repo.GetDevice(ctx, id)
}

// GetDeviceDetail returns the full device view including MACs, names, and hosts.
func (s *DeviceService) GetDeviceDetail(ctx context.Context, id uuid.UUID) (*db.DeviceDetail, error) {
	return s.repo.GetDeviceDetail(ctx, id)
}

// UpdateDevice applies name/notes changes to an existing device.
func (s *DeviceService) UpdateDevice(
	ctx context.Context, id uuid.UUID, input db.UpdateDeviceInput,
) (*db.Device, error) {
	return s.repo.UpdateDevice(ctx, id, input)
}

// DeleteDevice removes a device; attached hosts become unidentified (device_id → NULL).
func (s *DeviceService) DeleteDevice(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteDevice(ctx, id)
}

// AttachHost manually attaches a host to a device.
func (s *DeviceService) AttachHost(ctx context.Context, deviceID, hostID uuid.UUID) error {
	return s.repo.AttachHost(ctx, deviceID, hostID)
}

// DetachHost removes a host from a device.
func (s *DeviceService) DetachHost(ctx context.Context, deviceID, hostID uuid.UUID) error {
	return s.repo.DetachHost(ctx, deviceID, hostID)
}

// AcceptSuggestion attaches the host to the device and removes the suggestion row.
func (s *DeviceService) AcceptSuggestion(ctx context.Context, suggestionID uuid.UUID) error {
	return s.repo.AcceptSuggestion(ctx, suggestionID)
}

// DismissSuggestion marks a suggestion as dismissed without attaching.
func (s *DeviceService) DismissSuggestion(ctx context.Context, suggestionID uuid.UUID) error {
	return s.repo.DismissSuggestion(ctx, suggestionID)
}
