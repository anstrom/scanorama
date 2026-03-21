package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anstrom/scanorama/internal/api/handlers/mocks"
	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/errors"
)

// forbiddenErr returns a ScanError with CodeForbidden, matching what the
// real DB layer returns for built-in profile guards.
func forbiddenErr(msg string) error {
	return errors.ErrForbidden(msg)
}

// ── errors package: IsForbidden / ErrForbidden ────────────────────────────────

func TestIsForbidden(t *testing.T) {
	t.Run("true for ErrForbidden", func(t *testing.T) {
		err := errors.ErrForbidden("cannot delete built-in profile")
		assert.True(t, errors.IsForbidden(err))
	})

	t.Run("false for nil", func(t *testing.T) {
		assert.False(t, errors.IsForbidden(nil))
	})

	t.Run("false for plain error", func(t *testing.T) {
		assert.False(t, errors.IsForbidden(fmt.Errorf("something went wrong")))
	})

	t.Run("false for not-found error", func(t *testing.T) {
		assert.False(t, errors.IsForbidden(errors.ErrNotFoundWithID("profile", "abc")))
	})

	t.Run("false for conflict error", func(t *testing.T) {
		assert.False(t, errors.IsForbidden(errors.ErrConflict("profile")))
	})

	t.Run("IsForbidden does not fire for CodeConflict", func(t *testing.T) {
		err := errors.NewScanError(errors.CodeConflict, "conflict")
		assert.False(t, errors.IsForbidden(err))
	})
}

func TestErrForbidden(t *testing.T) {
	err := errors.ErrForbidden("cannot update built-in profile")
	require.NotNil(t, err)
	assert.True(t, errors.IsForbidden(err))
	assert.Contains(t, err.Error(), "cannot update built-in profile")
	assert.Equal(t, errors.CodeForbidden, errors.GetCode(err))
}

// ── ScanHandler: CreateScan profile pre-flight ────────────────────────────────

func TestScanHandler_CreateScan_ProfilePreflight(t *testing.T) {
	t.Run("returns 400 when profile_id not found", func(t *testing.T) {
		h, store, ctrl := newScanHandlerWithMock(t)
		defer ctrl.Finish()

		store.EXPECT().
			GetProfile(gomock.Any(), "no-such-profile").
			Return(nil, errors.ErrNotFoundWithID("profile", "no-such-profile"))

		body := `{"name":"S","targets":["10.0.0.1"],"scan_type":"connect","ports":"80","profile_id":"no-such-profile"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/scans", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.CreateScan(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Contains(t, resp["message"], "no-such-profile")
	})

	t.Run("returns 500 when profile lookup fails with unexpected error", func(t *testing.T) {
		h, store, ctrl := newScanHandlerWithMock(t)
		defer ctrl.Finish()

		store.EXPECT().
			GetProfile(gomock.Any(), "bad-profile").
			Return(nil, fmt.Errorf("connection reset"))

		body := `{"name":"S","targets":["10.0.0.1"],"scan_type":"connect","ports":"80","profile_id":"bad-profile"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/scans", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.CreateScan(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("skips profile check when profile_id is empty string", func(t *testing.T) {
		h, store, ctrl := newScanHandlerWithMock(t)
		defer ctrl.Finish()

		id := uuid.New()
		store.EXPECT().
			CreateScan(gomock.Any(), gomock.Any()).
			Return(makeScan(id, "S", "pending", "connect"), nil)
		// GetProfile must NOT be called when profile_id is "".

		body := `{"name":"S","targets":["10.0.0.1"],"scan_type":"connect","ports":"80","profile_id":""}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/scans", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.CreateScan(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)
	})

	t.Run("skips profile check when profile_id is absent", func(t *testing.T) {
		h, store, ctrl := newScanHandlerWithMock(t)
		defer ctrl.Finish()

		id := uuid.New()
		store.EXPECT().
			CreateScan(gomock.Any(), gomock.Any()).
			Return(makeScan(id, "S", "pending", "connect"), nil)

		body := `{"name":"S","targets":["10.0.0.1"],"scan_type":"connect","ports":"80"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/scans", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.CreateScan(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)
	})

	t.Run("proceeds and creates when profile_id exists", func(t *testing.T) {
		h, store, ctrl := newScanHandlerWithMock(t)
		defer ctrl.Finish()

		profileID := "web-scan"
		scanID := uuid.New()
		store.EXPECT().
			GetProfile(gomock.Any(), profileID).
			Return(makeProfile(profileID, "Web Scan"), nil)
		store.EXPECT().
			CreateScan(gomock.Any(), gomock.Any()).
			Return(makeScan(scanID, "S", "pending", "connect"), nil)

		body := fmt.Sprintf(
			`{"name":"S","targets":["10.0.0.1"],"scan_type":"connect","ports":"80","profile_id":%q}`, profileID)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/scans", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.CreateScan(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)
	})
}

// ── ScanHandler: GetScanResults scan pre-flight ───────────────────────────────

func TestScanHandler_GetScanResults_Preflight(t *testing.T) {
	t.Run("returns 404 when scan not found", func(t *testing.T) {
		h, store, ctrl := newScanHandlerWithMock(t)
		defer ctrl.Finish()

		scanID := uuid.New()
		store.EXPECT().
			GetScan(gomock.Any(), scanID).
			Return(nil, errors.ErrNotFoundWithID("scan", scanID.String()))

		router, _ := routerWithID(http.MethodGet, "/api/v1/scans/{id}/results", h.GetScanResults)
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/scans/%s/results", scanID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("returns 500 when scan lookup fails with unexpected error", func(t *testing.T) {
		h, store, ctrl := newScanHandlerWithMock(t)
		defer ctrl.Finish()

		scanID := uuid.New()
		store.EXPECT().
			GetScan(gomock.Any(), scanID).
			Return(nil, fmt.Errorf("database unavailable"))

		router, _ := routerWithID(http.MethodGet, "/api/v1/scans/{id}/results", h.GetScanResults)
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/scans/%s/results", scanID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("returns 404 when GetScanResults itself returns not-found", func(t *testing.T) {
		h, store, ctrl := newScanHandlerWithMock(t)
		defer ctrl.Finish()

		scanID := uuid.New()
		store.EXPECT().
			GetScan(gomock.Any(), scanID).
			Return(makeScan(scanID, "T", "completed", "connect"), nil)
		store.EXPECT().
			GetScanResults(gomock.Any(), scanID, 0, 50).
			Return(nil, int64(0), errors.ErrNotFoundWithID("scan", scanID.String()))

		router, _ := routerWithID(http.MethodGet, "/api/v1/scans/{id}/results", h.GetScanResults)
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/scans/%s/results", scanID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

// ── HostHandler: GetHostScans host pre-flight ─────────────────────────────────

func TestHostHandler_GetHostScans_Preflight(t *testing.T) {
	t.Run("returns 500 when host lookup fails with unexpected error", func(t *testing.T) {
		h, store, ctrl := newHostHandlerWithMock(t)
		defer ctrl.Finish()

		hostID := uuid.New()
		store.EXPECT().
			GetHost(gomock.Any(), hostID).
			Return(nil, fmt.Errorf("disk full"))

		router, _ := routerWithID(http.MethodGet, "/api/v1/hosts/{id}/scans", h.GetHostScans)
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/hosts/%s/scans", hostID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

// ── ProfileHandler: UpdateProfile ────────────────────────────────────────────

func TestProfileHandler_UpdateProfile_Mock(t *testing.T) {
	profileBody := `{"name":"Updated","scan_type":"connect","ports":"80"}`

	t.Run("updates profile and returns 200", func(t *testing.T) {
		h, store, ctrl := newProfileHandlerWithMock(t)
		defer ctrl.Finish()

		profile := makeProfile("my-profile", "Updated")
		store.EXPECT().
			UpdateProfile(gomock.Any(), "my-profile", gomock.Any()).
			Return(profile, nil)

		router := mux.NewRouter()
		router.HandleFunc("/api/v1/profiles/{id}", h.UpdateProfile).Methods(http.MethodPut)
		req := httptest.NewRequest(http.MethodPut, "/api/v1/profiles/my-profile", bytes.NewBufferString(profileBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "my-profile", resp["id"])
	})

	t.Run("returns 404 when profile not found", func(t *testing.T) {
		h, store, ctrl := newProfileHandlerWithMock(t)
		defer ctrl.Finish()

		store.EXPECT().
			UpdateProfile(gomock.Any(), "missing", gomock.Any()).
			Return(nil, errors.ErrNotFoundWithID("profile", "missing"))

		router := mux.NewRouter()
		router.HandleFunc("/api/v1/profiles/{id}", h.UpdateProfile).Methods(http.MethodPut)
		req := httptest.NewRequest(http.MethodPut, "/api/v1/profiles/missing", bytes.NewBufferString(profileBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("returns 403 when profile is built-in", func(t *testing.T) {
		h, store, ctrl := newProfileHandlerWithMock(t)
		defer ctrl.Finish()

		store.EXPECT().
			UpdateProfile(gomock.Any(), "linux-server", gomock.Any()).
			Return(nil, forbiddenErr("cannot update built-in profile"))

		router := mux.NewRouter()
		router.HandleFunc("/api/v1/profiles/{id}", h.UpdateProfile).Methods(http.MethodPut)
		req := httptest.NewRequest(http.MethodPut, "/api/v1/profiles/linux-server", bytes.NewBufferString(profileBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("returns 500 on unexpected store error", func(t *testing.T) {
		h, store, ctrl := newProfileHandlerWithMock(t)
		defer ctrl.Finish()

		store.EXPECT().
			UpdateProfile(gomock.Any(), "my-profile", gomock.Any()).
			Return(nil, fmt.Errorf("db error"))

		router := mux.NewRouter()
		router.HandleFunc("/api/v1/profiles/{id}", h.UpdateProfile).Methods(http.MethodPut)
		req := httptest.NewRequest(http.MethodPut, "/api/v1/profiles/my-profile", bytes.NewBufferString(profileBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("returns 400 for invalid JSON body", func(t *testing.T) {
		h, _, ctrl := newProfileHandlerWithMock(t)
		defer ctrl.Finish()

		router := mux.NewRouter()
		router.HandleFunc("/api/v1/profiles/{id}", h.UpdateProfile).Methods(http.MethodPut)
		req := httptest.NewRequest(http.MethodPut, "/api/v1/profiles/my-profile", bytes.NewBufferString("{bad}"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

// ── ProfileHandler: DeleteProfile (forbidden path) ───────────────────────────

func TestProfileHandler_DeleteProfile_Forbidden(t *testing.T) {
	t.Run("returns 403 when profile is built-in", func(t *testing.T) {
		h, store, ctrl := newProfileHandlerWithMock(t)
		defer ctrl.Finish()

		store.EXPECT().
			DeleteProfile(gomock.Any(), "linux-server").
			Return(forbiddenErr("cannot delete built-in profile"))

		router := mux.NewRouter()
		router.HandleFunc("/api/v1/profiles/{id}", h.DeleteProfile).Methods(http.MethodDelete)
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/profiles/linux-server", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusForbidden, w.Code)
	})
}

// ── validateScanRequest: new rules ───────────────────────────────────────────

func TestValidateScanRequest_NewRules(t *testing.T) {
	h := &ScanHandler{}

	t.Run("ports is required", func(t *testing.T) {
		req := &ScanRequest{Name: "S", Targets: []string{"10.0.0.1"}, ScanType: "connect", Ports: ""}
		err := h.validateScanRequest(req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ports is required")
	})

	t.Run("too many targets rejected", func(t *testing.T) {
		targets := make([]string, maxTargetCount+1)
		for i := range targets {
			targets[i] = "10.0.0.1"
		}
		req := &ScanRequest{Name: "S", Targets: targets, ScanType: "connect", Ports: "80"}
		err := h.validateScanRequest(req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "too many targets")
	})

	t.Run("exactly maxTargetCount targets accepted", func(t *testing.T) {
		targets := make([]string, maxTargetCount)
		for i := range targets {
			targets[i] = "10.0.0.1"
		}
		req := &ScanRequest{Name: "S", Targets: targets, ScanType: "connect", Ports: "80"}
		assert.NoError(t, h.validateScanRequest(req))
	})
}

// ── parsePortToken / parsePortRange edge cases ────────────────────────────────

func TestParsePortSpec_EdgeCases(t *testing.T) {
	tests := []struct {
		name      string
		ports     string
		wantError bool
		errSubstr string
	}{
		// Valid cases
		{"single port", "80", false, ""},
		{"multiple ports", "22,80,443", false, ""},
		{"valid range", "1024-9999", false, ""},
		{"T: prefix single", "T:80", false, ""},
		{"U: prefix single", "U:53", false, ""},
		{"T: prefix range", "T:1024-2048", false, ""},
		{"U: prefix range", "U:1-1024", false, ""},
		{"mixed prefixes", "T:80,U:53,443", false, ""},
		{"port 1 (min)", "1", false, ""},
		{"port 65535 (max)", "65535", false, ""},
		{"range 1-65535 (full)", "1-65535", false, ""},
		{"comma with empty token", "80,,443", false, ""},
		{"leading/trailing spaces in token", " 80 ", false, ""},

		// Invalid cases — inverted range
		{"inverted range", "9000-1", true, "start must be <= end"},
		{"inverted range 443-80", "443-80", true, "start must be <= end"},

		// Invalid cases — whitespace inside token
		{"spaces in range", "80 - 443", true, "whitespace not allowed"},
		{"tab in port", "80\t443", true, "whitespace not allowed"},

		// Invalid cases — out of range
		{"port 0", "0", true, "must be between 1 and 65535"},
		{"port 65536", "65536", true, "must be between 1 and 65535"},
		{"range start 0", "0-80", true, "must be between 1 and 65535"},
		{"range end 65536", "80-65536", true, "must be between 1 and 65535"},
		{"range start 0 inverted", "0-0", true, "must be between 1 and 65535"},

		// Invalid cases — non-numeric
		{"alpha port", "http", true, "must be a number"},
		{"alpha range start", "http-443", true, "must be a number"},
		{"alpha range end", "80-https", true, "must be a number"},
		{"T: with alpha", "T:http", true, "must be a number"},
		{"U: with alpha range", "U:53-dns", true, "must be a number"},

		// Equal range boundary (start == end) — valid
		{"equal range", "80-80", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := parsePortSpec(tt.ports)
			if tt.wantError {
				require.Error(t, err, "expected error for ports=%q", tt.ports)
				if tt.errSubstr != "" {
					assert.Contains(t, err.Error(), tt.errSubstr)
				}
			} else {
				assert.NoError(t, err, "expected no error for ports=%q", tt.ports)
			}
		})
	}
}

// ── ProfileHandler: validateProfileRequest sub-validators ────────────────────

func TestValidateProfileRequest_SubValidators(t *testing.T) {
	h := &ProfileHandler{}
	baseReq := func() *ProfileRequest {
		return &ProfileRequest{
			Name:     "Test Profile",
			ScanType: "connect",
		}
	}

	t.Run("invalid timing template rejected", func(t *testing.T) {
		req := baseReq()
		req.Timing.Template = "turbo"
		err := h.validateProfileRequest(req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid timing template")
	})

	t.Run("empty timing template accepted", func(t *testing.T) {
		req := baseReq()
		req.Timing.Template = ""
		assert.NoError(t, h.validateProfileRequest(req))
	})

	t.Run("valid timing templates accepted", func(t *testing.T) {
		for _, tmpl := range []string{"paranoid", "sneaky", "polite", "normal", "aggressive", "insane"} {
			req := baseReq()
			req.Timing.Template = tmpl
			assert.NoError(t, h.validateProfileRequest(req), "template %q should be valid", tmpl)
		}
	})

	t.Run("negative max retries rejected", func(t *testing.T) {
		req := baseReq()
		req.MaxRetries = -1
		err := h.validateProfileRequest(req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "max retries cannot be negative")
	})

	t.Run("negative max rate PPS rejected", func(t *testing.T) {
		req := baseReq()
		req.MaxRatePPS = -1
		err := h.validateProfileRequest(req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "max rate PPS cannot be negative")
	})

	t.Run("negative max host group size rejected", func(t *testing.T) {
		req := baseReq()
		req.MaxHostGroupSize = -1
		err := h.validateProfileRequest(req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "max host group size cannot be negative")
	})

	t.Run("negative min host group size rejected", func(t *testing.T) {
		req := baseReq()
		req.MinHostGroupSize = -1
		err := h.validateProfileRequest(req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "min host group size cannot be negative")
	})

	t.Run("min > max host group size rejected when max > 0", func(t *testing.T) {
		req := baseReq()
		req.MinHostGroupSize = 10
		req.MaxHostGroupSize = 5
		err := h.validateProfileRequest(req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "min host group size cannot be greater than max host group size")
	})

	t.Run("min > max host group size allowed when max == 0 (unbounded)", func(t *testing.T) {
		req := baseReq()
		req.MinHostGroupSize = 10
		req.MaxHostGroupSize = 0
		assert.NoError(t, h.validateProfileRequest(req))
	})

	t.Run("empty tag rejected", func(t *testing.T) {
		req := baseReq()
		req.Tags = []string{"valid-tag", ""}
		err := h.validateProfileRequest(req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "tag 2 is empty")
	})

	t.Run("tag too long rejected", func(t *testing.T) {
		req := baseReq()
		req.Tags = []string{strings.Repeat("a", maxProfileTagLength+1)}
		err := h.validateProfileRequest(req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "tag 1 too long")
	})

	t.Run("valid tags accepted", func(t *testing.T) {
		req := baseReq()
		req.Tags = []string{"prod", "web", "external"}
		assert.NoError(t, h.validateProfileRequest(req))
	})

	t.Run("min RTT greater than max RTT rejected when max > 0", func(t *testing.T) {
		req := baseReq()
		req.Timing.MinRTTTimeout = Duration(200 * 1e6) // 200ms in nanoseconds
		req.Timing.MaxRTTTimeout = Duration(100 * 1e6) // 100ms in nanoseconds
		err := h.validateProfileRequest(req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "min RTT timeout cannot be greater than max RTT timeout")
	})

	t.Run("min RTT greater than max RTT allowed when max is zero", func(t *testing.T) {
		req := baseReq()
		req.Timing.MinRTTTimeout = Duration(200 * 1e6)
		req.Timing.MaxRTTTimeout = Duration(0)
		assert.NoError(t, h.validateProfileRequest(req))
	})
}

// ── ScanHandler: GetScanResults mock store — MockScanStore ────────────────────

func TestScanHandler_GetScanResults_MockStore(t *testing.T) {
	t.Run("returns 200 with empty results and zero summary on GetScanSummary failure", func(t *testing.T) {
		h, store, ctrl := newScanHandlerWithMock(t)
		defer ctrl.Finish()

		scanID := uuid.New()
		store.EXPECT().
			GetScan(gomock.Any(), scanID).
			Return(makeScan(scanID, "T", "completed", "connect"), nil)
		store.EXPECT().
			GetScanResults(gomock.Any(), scanID, 0, 50).
			Return([]*db.ScanResult{}, int64(0), nil)
		store.EXPECT().
			GetScanSummary(gomock.Any(), scanID).
			Return(nil, fmt.Errorf("summary unavailable"))

		router, _ := routerWithID(http.MethodGet, "/api/v1/scans/{id}/results", h.GetScanResults)
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/scans/%s/results", scanID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// Summary failure is treated as a warning; response still 200.
		assert.Equal(t, http.StatusOK, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, float64(0), resp["total_hosts"])
	})
}

// ── ProfileHandler: GetProfile 500 path ──────────────────────────────────────

func TestProfileHandler_GetProfile_InternalError(t *testing.T) {
	t.Run("returns 500 on unexpected store error", func(t *testing.T) {
		h, store, ctrl := newProfileHandlerWithMock(t)
		defer ctrl.Finish()

		store.EXPECT().
			GetProfile(gomock.Any(), "my-profile").
			Return(nil, fmt.Errorf("db connection lost"))

		router := mux.NewRouter()
		router.HandleFunc("/api/v1/profiles/{id}", h.GetProfile).Methods(http.MethodGet)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/profiles/my-profile", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

// ── MockScanStore satisfies ScanStore interface (compile-time check) ──────────

var _ ScanStore = (*mocks.MockScanStore)(nil)
