package services

import (
	"context"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/errors"
)

// mockDeviceRepo is a test double for deviceRepository.
type mockDeviceRepo struct {
	device    *db.Device
	createErr error
	getErr    error
	deleteErr error
}

func (m *mockDeviceRepo) ListDevices(_ context.Context) ([]db.DeviceSummary, error) {
	return make([]db.DeviceSummary, 0), nil
}
func (m *mockDeviceRepo) CreateDevice(_ context.Context, _ db.CreateDeviceInput) (*db.Device, error) {
	return m.device, m.createErr
}
func (m *mockDeviceRepo) GetDevice(_ context.Context, _ uuid.UUID) (*db.Device, error) {
	return m.device, m.getErr
}
func (m *mockDeviceRepo) GetDeviceDetail(_ context.Context, _ uuid.UUID) (*db.DeviceDetail, error) {
	if m.device == nil {
		return nil, m.getErr
	}
	return &db.DeviceDetail{
		Device:     *m.device,
		KnownMACs:  make([]db.DeviceKnownMAC, 0),
		KnownNames: make([]db.DeviceKnownName, 0),
		Hosts:      make([]db.AttachedHostSummary, 0),
	}, nil
}
func (m *mockDeviceRepo) UpdateDevice(_ context.Context, _ uuid.UUID, _ db.UpdateDeviceInput) (*db.Device, error) {
	return m.device, nil
}
func (m *mockDeviceRepo) DeleteDevice(_ context.Context, _ uuid.UUID) error  { return m.deleteErr }
func (m *mockDeviceRepo) AttachHost(_ context.Context, _, _ uuid.UUID) error { return nil }
func (m *mockDeviceRepo) DetachHost(_ context.Context, _, _ uuid.UUID) error { return nil }
func (m *mockDeviceRepo) AcceptSuggestion(_ context.Context, _ uuid.UUID) error {
	return nil
}
func (m *mockDeviceRepo) DismissSuggestion(_ context.Context, _ uuid.UUID) error { return nil }

// ── CreateDevice ──────────────────────────────────────────────────────────

func TestDeviceService_CreateDevice_RequiresName(t *testing.T) {
	svc := NewDeviceService(&mockDeviceRepo{}, slog.Default())
	_, err := svc.CreateDevice(context.Background(), db.CreateDeviceInput{Name: ""})
	require.Error(t, err)
	assert.True(t, errors.IsCode(err, errors.CodeValidation))
}

func TestDeviceService_CreateDevice_Success(t *testing.T) {
	id := uuid.New()
	repo := &mockDeviceRepo{device: &db.Device{ID: id, Name: "Router"}}
	svc := NewDeviceService(repo, slog.Default())

	d, err := svc.CreateDevice(context.Background(), db.CreateDeviceInput{Name: "Router"})
	require.NoError(t, err)
	assert.Equal(t, id, d.ID)
}

// ── GetDevice ─────────────────────────────────────────────────────────────

func TestDeviceService_GetDevice_NotFound(t *testing.T) {
	repo := &mockDeviceRepo{getErr: errors.NewScanError(errors.CodeNotFound, "device not found")}
	svc := NewDeviceService(repo, slog.Default())

	_, err := svc.GetDevice(context.Background(), uuid.New())
	require.Error(t, err)
	assert.True(t, errors.IsCode(err, errors.CodeNotFound))
}

// ── DeleteDevice ─────────────────────────────────────────────────────────

func TestDeviceService_DeleteDevice_NotFound(t *testing.T) {
	repo := &mockDeviceRepo{deleteErr: errors.NewScanError(errors.CodeNotFound, "device not found")}
	svc := NewDeviceService(repo, slog.Default())

	err := svc.DeleteDevice(context.Background(), uuid.New())
	require.Error(t, err)
	assert.True(t, errors.IsCode(err, errors.CodeNotFound))
}

// ── ListDevices ───────────────────────────────────────────────────────────

func TestDeviceService_ListDevices_ReturnsList(t *testing.T) {
	svc := NewDeviceService(&mockDeviceRepo{}, slog.Default())
	result, err := svc.ListDevices(context.Background())
	require.NoError(t, err)
	assert.NotNil(t, result) // never nil — must serialize as []
}
