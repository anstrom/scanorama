package handlers

import (
	"context"
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

	"github.com/anstrom/scanorama/internal/db"
	apierrors "github.com/anstrom/scanorama/internal/errors"
	"github.com/anstrom/scanorama/internal/metrics"
)

// captureUpdateHostServicer records the UpdateHostInput the handler builds.
// It returns host when set, otherwise a bare host with the requested ID.
type captureUpdateHostServicer struct {
	nilHostServicer
	got  *db.UpdateHostInput
	host *db.Host
	err  error
}

func (m *captureUpdateHostServicer) UpdateHost(
	_ context.Context, id uuid.UUID, input db.UpdateHostInput,
) (*db.Host, error) {
	m.got = &input
	if m.err != nil {
		return nil, m.err
	}
	if m.host != nil {
		return m.host, nil
	}
	return &db.Host{ID: id}, nil
}

func putHost(t *testing.T, svc HostServicer, body string) *httptest.ResponseRecorder {
	t.Helper()
	handler := NewHostHandler(svc, createTestLogger(), metrics.NewRegistry())
	id := uuid.New().String()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/hosts/"+id, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = mux.SetURLVars(req, map[string]string{"id": id})
	w := httptest.NewRecorder()
	handler.UpdateHost(w, req)
	return w
}

func TestHostHandler_UpdateHost_OmittedFieldsAreUnchanged(t *testing.T) {
	svc := &captureUpdateHostServicer{}

	w := putHost(t, svc, `{"hostname":"new-name"}`)

	require.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, svc.got)
	require.NotNil(t, svc.got.Hostname)
	assert.Equal(t, "new-name", *svc.got.Hostname)
	assert.Nil(t, svc.got.IgnoreScanning, "omitted active must not change ignore_scanning")
	assert.Nil(t, svc.got.OSFamily)
	assert.Nil(t, svc.got.OSName)
	assert.Nil(t, svc.got.Tags)
	assert.Nil(t, svc.got.Notes, "omitted notes must not change notes")
}

func TestHostHandler_UpdateHost_ActiveMapsToIgnoreScanning(t *testing.T) {
	tests := []struct {
		body string
		want bool
	}{
		{`{"active":true}`, false},
		{`{"active":false}`, true},
	}
	for _, tt := range tests {
		t.Run(tt.body, func(t *testing.T) {
			svc := &captureUpdateHostServicer{}

			w := putHost(t, svc, tt.body)

			require.Equal(t, http.StatusOK, w.Code)
			require.NotNil(t, svc.got.IgnoreScanning)
			assert.Equal(t, tt.want, *svc.got.IgnoreScanning)
		})
	}
}

func TestHostHandler_UpdateHost_EmptyHostnameClears(t *testing.T) {
	svc := &captureUpdateHostServicer{}

	w := putHost(t, svc, `{"hostname":""}`)

	require.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, svc.got.Hostname, "an explicit empty hostname must reach the DB layer")
	assert.Empty(t, *svc.got.Hostname)
}

func TestHostHandler_UpdateHost_NullHostnameIsUnchanged(t *testing.T) {
	svc := &captureUpdateHostServicer{}

	w := putHost(t, svc, `{"hostname":null,"active":true}`)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Nil(t, svc.got.Hostname)
}

func TestHostHandler_UpdateHost_HostnameTooLong(t *testing.T) {
	body, err := json.Marshal(map[string]string{"hostname": strings.Repeat("a", maxHostnameLength+1)})
	require.NoError(t, err)

	w := putHost(t, &captureUpdateHostServicer{}, string(body))

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHostHandler_UpdateHost_NoUpdatableFields(t *testing.T) {
	for _, body := range []string{`{}`, `{"description":"notes"}`, `{"hostname":null}`, `{"notes":null}`} {
		t.Run(body, func(t *testing.T) {
			w := putHost(t, &captureUpdateHostServicer{}, body)

			assert.Equal(t, http.StatusBadRequest, w.Code)
			var raw map[string]any
			require.NoError(t, json.NewDecoder(w.Body).Decode(&raw))
			assert.Contains(t, raw["message"], "no updatable fields")
		})
	}
}

func TestHostHandler_UpdateHost_NotesOnlySavesNotes(t *testing.T) {
	notes := "rack 4, owned by infra"
	svc := &captureUpdateHostServicer{host: &db.Host{ID: uuid.New(), Notes: &notes}}

	w := putHost(t, svc, `{"notes":"rack 4, owned by infra"}`)

	require.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, svc.got.Notes)
	assert.Equal(t, notes, *svc.got.Notes)
	assert.Nil(t, svc.got.Hostname, "a notes-only update must not touch hostname")

	var raw map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&raw))
	assert.Equal(t, notes, raw["notes"])
}

func TestHostHandler_UpdateHost_EmptyNotesClear(t *testing.T) {
	svc := &captureUpdateHostServicer{}

	w := putHost(t, svc, `{"notes":""}`)

	require.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, svc.got.Notes, "an explicit empty notes value must reach the DB layer")
	assert.Empty(t, *svc.got.Notes)

	var raw map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&raw))
	assert.NotContains(t, raw, "notes", "cleared notes are omitted from the response")
}

func TestHostHandler_UpdateHost_NotesTooLong(t *testing.T) {
	body, err := json.Marshal(map[string]string{"notes": strings.Repeat("a", maxHostNotesLength+1)})
	require.NoError(t, err)
	svc := &captureUpdateHostServicer{}

	w := putHost(t, svc, string(body))

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Nil(t, svc.got, "an invalid request must not reach the service")
	var raw map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&raw))
	assert.Contains(t, raw["message"], "notes too long")
}

func TestHostHandler_UpdateHost_NotesAtLimitAccepted(t *testing.T) {
	body, err := json.Marshal(map[string]string{"notes": strings.Repeat("a", maxHostNotesLength)})
	require.NoError(t, err)
	svc := &captureUpdateHostServicer{}

	w := putHost(t, svc, string(body))

	require.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, svc.got.Notes)
	assert.Len(t, *svc.got.Notes, maxHostNotesLength)
}

func TestHostHandler_UpdateHost_NotesLimitCountsCharacters(t *testing.T) {
	// Multi-byte characters count once each, matching the error message.
	body, err := json.Marshal(map[string]string{"notes": strings.Repeat("ø", maxHostNotesLength)})
	require.NoError(t, err)
	svc := &captureUpdateHostServicer{}

	w := putHost(t, svc, string(body))

	require.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, svc.got)
}

func TestHostHandler_UpdateHost_ServiceErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"not found", apierrors.ErrNotFoundWithID("host", uuid.New().String()), http.StatusNotFound},
		{"database failure", fmt.Errorf("connection reset"), http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := putHost(t, &captureUpdateHostServicer{err: tt.err}, `{"hostname":"web-01"}`)

			assert.Equal(t, tt.want, w.Code)
		})
	}
}
