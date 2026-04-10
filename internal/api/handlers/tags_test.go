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

	"github.com/anstrom/scanorama/internal/errors"
)

// newTagsRouter wires all tag endpoints onto a dedicated mux.Router.
// /hosts/bulk/tags is registered before /hosts/{id}/tags so gorilla/mux
// matches the literal "bulk" segment first when the method is POST.
func newTagsRouter(h *HostHandler) *mux.Router {
	r := mux.NewRouter()
	r.HandleFunc("/tags", h.ListTags).Methods("GET")
	r.HandleFunc("/hosts/bulk/tags", h.BulkUpdateTags).Methods("POST")
	r.HandleFunc("/hosts/{id}/tags", h.ReplaceHostTags).Methods("PUT")
	r.HandleFunc("/hosts/{id}/tags", h.AddHostTags).Methods("POST")
	r.HandleFunc("/hosts/{id}/tags", h.DeleteHostTags).Methods("DELETE")
	return r
}

// ── normaliseTags ─────────────────────────────────────────────────────────────

func TestNormaliseTags(t *testing.T) {
	cases := []struct {
		name  string
		input []string
		want  []string
	}{
		{
			name:  "nil input returns empty slice",
			input: nil,
			want:  []string{},
		},
		{
			name:  "clean tags unchanged",
			input: []string{"prod", "web"},
			want:  []string{"prod", "web"},
		},
		{
			name:  "whitespace trimmed",
			input: []string{"  prod  ", " web"},
			want:  []string{"prod", "web"},
		},
		{
			name:  "empty strings removed",
			input: []string{"prod", "", "  ", "web"},
			want:  []string{"prod", "web"},
		},
		{
			name:  "all-empty input gives empty slice",
			input: []string{"", "  "},
			want:  []string{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := normaliseTags(tc.input)
			assert.Equal(t, tc.want, got)
		})
	}
}

// ── validateTagList ───────────────────────────────────────────────────────────

func TestValidateTagList(t *testing.T) {
	cases := []struct {
		name    string
		tags    []string
		wantErr string // substring that must appear in error; empty means no error expected
	}{
		{
			name: "valid tags",
			tags: []string{"prod", "web", "staging"},
		},
		{
			name: "empty list is valid",
			tags: []string{},
		},
		{
			name: "exactly 100 tags – at limit",
			tags: func() []string {
				tags := make([]string, 100)
				for i := range tags {
					tags[i] = fmt.Sprintf("tag%03d", i)
				}
				return tags
			}(),
		},
		{
			name: "101 tags – over limit",
			tags: func() []string {
				tags := make([]string, 101)
				for i := range tags {
					tags[i] = fmt.Sprintf("tag%03d", i)
				}
				return tags
			}(),
			wantErr: "too many",
		},
		{
			name: "tag of length 50 – at limit",
			tags: []string{strings.Repeat("a", 50)},
		},
		{
			name:    "tag of length 51 – over limit",
			tags:    []string{strings.Repeat("a", 51)},
			wantErr: "length",
		},
		{
			name:    "duplicate tags rejected",
			tags:    []string{"prod", "web", "prod"},
			wantErr: "duplicate",
		},
		{
			name: "mix of valid tags",
			tags: []string{"prod", "staging", "web", "backend", "k8s"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateTagList(tc.tags)
			if tc.wantErr == "" {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
			}
		})
	}
}

// ── ListTags ──────────────────────────────────────────────────────────────────

func TestListTags_ReturnsTagList(t *testing.T) {
	h, svc, ctrl := newHostHandlerWithMock(t)
	defer ctrl.Finish()

	svc.EXPECT().
		ListTags(gomock.Any()).
		Return([]string{"prod", "web"}, nil)

	r := newTagsRouter(h)
	req := httptest.NewRequest(http.MethodGet, "/tags", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	tags, ok := resp["tags"].([]interface{})
	require.True(t, ok, "expected 'tags' key with JSON array value")
	require.Len(t, tags, 2)
	assert.Equal(t, "prod", tags[0])
	assert.Equal(t, "web", tags[1])
}

func TestListTags_EmptyWhenNoTags(t *testing.T) {
	h, svc, ctrl := newHostHandlerWithMock(t)
	defer ctrl.Finish()

	svc.EXPECT().
		ListTags(gomock.Any()).
		Return([]string{}, nil)

	r := newTagsRouter(h)
	req := httptest.NewRequest(http.MethodGet, "/tags", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	// Must decode as an array (not null) even when there are no tags.
	tags, ok := resp["tags"].([]interface{})
	require.True(t, ok, "empty tag list must serialize as [] not null")
	assert.Empty(t, tags)
}

func TestListTags_ServiceError(t *testing.T) {
	h, svc, ctrl := newHostHandlerWithMock(t)
	defer ctrl.Finish()

	svc.EXPECT().
		ListTags(gomock.Any()).
		Return(nil, fmt.Errorf("database unavailable"))

	r := newTagsRouter(h)
	req := httptest.NewRequest(http.MethodGet, "/tags", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ── ReplaceHostTags ───────────────────────────────────────────────────────────

func TestReplaceHostTags_Valid(t *testing.T) {
	h, svc, ctrl := newHostHandlerWithMock(t)
	defer ctrl.Finish()

	id := uuid.New()
	host := makeHost(id, "10.0.0.1")

	svc.EXPECT().UpdateHostTags(gomock.Any(), id, gomock.Any()).Return(nil)
	svc.EXPECT().GetHost(gomock.Any(), id).Return(host, nil)

	r := newTagsRouter(h)
	body := bytes.NewBufferString(`{"tags":["prod"]}`)
	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/hosts/%s/tags", id), body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, id.String(), resp["id"])
}

func TestReplaceHostTags_InvalidUUID(t *testing.T) {
	h, _, ctrl := newHostHandlerWithMock(t)
	defer ctrl.Finish()

	r := newTagsRouter(h)
	req := httptest.NewRequest(http.MethodPut, "/hosts/not-a-uuid/tags",
		bytes.NewBufferString(`{"tags":["prod"]}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestReplaceHostTags_InvalidBody(t *testing.T) {
	h, _, ctrl := newHostHandlerWithMock(t)
	defer ctrl.Finish()

	id := uuid.New()
	r := newTagsRouter(h)
	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/hosts/%s/tags", id),
		bytes.NewBufferString(`{invalid json}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestReplaceHostTags_TooManyTags(t *testing.T) {
	h, _, ctrl := newHostHandlerWithMock(t)
	defer ctrl.Finish()

	// Build a body with 101 tags.
	tags := make([]string, 101)
	for i := range tags {
		tags[i] = fmt.Sprintf(`"tag%03d"`, i)
	}
	body := fmt.Sprintf(`{"tags":[%s]}`, strings.Join(tags, ","))

	id := uuid.New()
	r := newTagsRouter(h)
	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/hosts/%s/tags", id),
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Contains(t, resp["message"], "too many")
}

func TestReplaceHostTags_DuplicateTags(t *testing.T) {
	h, _, ctrl := newHostHandlerWithMock(t)
	defer ctrl.Finish()

	id := uuid.New()
	r := newTagsRouter(h)
	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/hosts/%s/tags", id),
		bytes.NewBufferString(`{"tags":["prod","web","prod"]}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Contains(t, resp["message"], "duplicate")
}

func TestReplaceHostTags_HostNotFound(t *testing.T) {
	h, svc, ctrl := newHostHandlerWithMock(t)
	defer ctrl.Finish()

	id := uuid.New()
	svc.EXPECT().
		UpdateHostTags(gomock.Any(), id, gomock.Any()).
		Return(errors.ErrNotFoundWithID("host", id.String()))

	r := newTagsRouter(h)
	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/hosts/%s/tags", id),
		bytes.NewBufferString(`{"tags":["prod"]}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// ── AddHostTags ───────────────────────────────────────────────────────────────

func TestAddHostTags_Valid(t *testing.T) {
	h, svc, ctrl := newHostHandlerWithMock(t)
	defer ctrl.Finish()

	id := uuid.New()
	host := makeHost(id, "10.0.0.2")

	svc.EXPECT().AddHostTags(gomock.Any(), id, gomock.Any()).Return(nil)
	svc.EXPECT().GetHost(gomock.Any(), id).Return(host, nil)

	r := newTagsRouter(h)
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/hosts/%s/tags", id),
		bytes.NewBufferString(`{"tags":["new-tag"]}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAddHostTags_InvalidUUID(t *testing.T) {
	h, _, ctrl := newHostHandlerWithMock(t)
	defer ctrl.Finish()

	r := newTagsRouter(h)
	req := httptest.NewRequest(http.MethodPost, "/hosts/not-a-uuid/tags",
		bytes.NewBufferString(`{"tags":["new-tag"]}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAddHostTags_EmptyTagsFiltered(t *testing.T) {
	h, svc, ctrl := newHostHandlerWithMock(t)
	defer ctrl.Finish()

	id := uuid.New()
	host := makeHost(id, "10.0.0.3")

	// After normalisation, only "valid-tag" survives; the whitespace-only entry
	// is stripped before the service call.
	svc.EXPECT().
		AddHostTags(gomock.Any(), id, []string{"valid-tag"}).
		Return(nil)
	svc.EXPECT().GetHost(gomock.Any(), id).Return(host, nil)

	r := newTagsRouter(h)
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/hosts/%s/tags", id),
		bytes.NewBufferString(`{"tags":["  ","valid-tag"]}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// ── DeleteHostTags ────────────────────────────────────────────────────────────

func TestDeleteHostTags_Valid(t *testing.T) {
	h, svc, ctrl := newHostHandlerWithMock(t)
	defer ctrl.Finish()

	id := uuid.New()
	host := makeHost(id, "10.0.0.4")

	svc.EXPECT().RemoveHostTags(gomock.Any(), id, gomock.Any()).Return(nil)
	svc.EXPECT().GetHost(gomock.Any(), id).Return(host, nil)

	r := newTagsRouter(h)
	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/hosts/%s/tags", id),
		bytes.NewBufferString(`{"tags":["prod"]}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestDeleteHostTags_InvalidUUID(t *testing.T) {
	h, _, ctrl := newHostHandlerWithMock(t)
	defer ctrl.Finish()

	r := newTagsRouter(h)
	req := httptest.NewRequest(http.MethodDelete, "/hosts/not-a-uuid/tags",
		bytes.NewBufferString(`{"tags":["prod"]}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ── BulkUpdateTags ────────────────────────────────────────────────────────────

func TestBulkUpdateTags_AddAction(t *testing.T) {
	h, svc, ctrl := newHostHandlerWithMock(t)
	defer ctrl.Finish()

	id1 := uuid.New()
	id2 := uuid.New()

	svc.EXPECT().
		BulkUpdateTags(gomock.Any(), gomock.Any(), []string{"prod"}, "add").
		Return(nil)

	r := newTagsRouter(h)
	body := fmt.Sprintf(`{"host_ids":[%q,%q],"tags":["prod"],"action":"add"}`, id1, id2)
	req := httptest.NewRequest(http.MethodPost, "/hosts/bulk/tags",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(2), resp["updated"])
	assert.Equal(t, "add", resp["action"])
}

func TestBulkUpdateTags_InvalidAction(t *testing.T) {
	h, _, ctrl := newHostHandlerWithMock(t)
	defer ctrl.Finish()

	id := uuid.New()
	r := newTagsRouter(h)
	body := fmt.Sprintf(`{"host_ids":[%q],"tags":["prod"],"action":"nuke"}`, id)
	req := httptest.NewRequest(http.MethodPost, "/hosts/bulk/tags",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestBulkUpdateTags_EmptyHostIDs(t *testing.T) {
	h, _, ctrl := newHostHandlerWithMock(t)
	defer ctrl.Finish()

	r := newTagsRouter(h)
	req := httptest.NewRequest(http.MethodPost, "/hosts/bulk/tags",
		bytes.NewBufferString(`{"host_ids":[],"tags":["prod"],"action":"set"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestBulkUpdateTags_InvalidHostIDFormat(t *testing.T) {
	h, _, ctrl := newHostHandlerWithMock(t)
	defer ctrl.Finish()

	r := newTagsRouter(h)
	req := httptest.NewRequest(http.MethodPost, "/hosts/bulk/tags",
		bytes.NewBufferString(`{"host_ids":["not-a-uuid"],"tags":["prod"],"action":"add"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestBulkUpdateTags_SetAction(t *testing.T) {
	h, svc, ctrl := newHostHandlerWithMock(t)
	defer ctrl.Finish()

	id1 := uuid.New()
	id2 := uuid.New()
	id3 := uuid.New()

	svc.EXPECT().
		BulkUpdateTags(gomock.Any(), gomock.Any(), gomock.Any(), "set").
		Return(nil)

	r := newTagsRouter(h)
	body := fmt.Sprintf(`{"host_ids":[%q,%q,%q],"tags":["infra"],"action":"set"}`, id1, id2, id3)
	req := httptest.NewRequest(http.MethodPost, "/hosts/bulk/tags",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(3), resp["updated"])
	assert.Equal(t, "set", resp["action"])
}
