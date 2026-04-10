package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anstrom/scanorama/internal/api/handlers/mocks"
	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/metrics"
)

// ── helpers ───────────────────────────────────────────────────────────────────

func newGroupHandlerWithMock(t *testing.T) (*GroupHandler, *mocks.MockGroupServicer, *gomock.Controller) {
	t.Helper()
	ctrl := gomock.NewController(t)
	svc := mocks.NewMockGroupServicer(ctrl)
	h := NewGroupHandler(svc, createTestLogger(), metrics.NewRegistry())
	return h, svc, ctrl
}

func newGroupRouter(h *GroupHandler) *mux.Router {
	r := mux.NewRouter()
	r.HandleFunc("/groups", h.ListGroups).Methods("GET")
	r.HandleFunc("/groups", h.CreateGroup).Methods("POST")
	r.HandleFunc("/groups/{id}", h.GetGroup).Methods("GET")
	r.HandleFunc("/groups/{id}", h.UpdateGroup).Methods("PUT")
	r.HandleFunc("/groups/{id}", h.DeleteGroup).Methods("DELETE")
	r.HandleFunc("/groups/{id}/hosts", h.ListGroupMembers).Methods("GET")
	r.HandleFunc("/groups/{id}/hosts", h.AddGroupMembers).Methods("POST")
	r.HandleFunc("/groups/{id}/hosts", h.RemoveGroupMembers).Methods("DELETE")
	return r
}

func makeGroup(id uuid.UUID, name string) *db.HostGroup {
	return &db.HostGroup{
		ID:          id,
		Name:        name,
		Description: "test group",
		Color:       "#3b82f6",
		MemberCount: 0,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
}

// ── ListGroups ────────────────────────────────────────────────────────────────

func TestListGroups_ReturnsGroups(t *testing.T) {
	h, svc, ctrl := newGroupHandlerWithMock(t)
	defer ctrl.Finish()

	id1 := uuid.New()
	id2 := uuid.New()
	g1 := makeGroup(id1, "infra")
	g2 := makeGroup(id2, "web")

	svc.EXPECT().
		ListGroups(gomock.Any()).
		Return([]*db.HostGroup{g1, g2}, nil)

	r := newGroupRouter(h)
	req := httptest.NewRequest(http.MethodGet, "/groups", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	groups, ok := resp["groups"].([]interface{})
	require.True(t, ok, "expected 'groups' key with a JSON array value")
	require.Len(t, groups, 2)

	names := []string{
		groups[0].(map[string]interface{})["name"].(string),
		groups[1].(map[string]interface{})["name"].(string),
	}
	assert.Contains(t, names, "infra")
	assert.Contains(t, names, "web")
}

func TestListGroups_EmptyList(t *testing.T) {
	h, svc, ctrl := newGroupHandlerWithMock(t)
	defer ctrl.Finish()

	svc.EXPECT().
		ListGroups(gomock.Any()).
		Return([]*db.HostGroup{}, nil)

	r := newGroupRouter(h)
	req := httptest.NewRequest(http.MethodGet, "/groups", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	// Must decode as an array (not null) even when the list is empty.
	groups, ok := resp["groups"].([]interface{})
	require.True(t, ok, "empty group list must serialize as [] not null")
	assert.Empty(t, groups)
}

func TestListGroups_ServiceError(t *testing.T) {
	h, svc, ctrl := newGroupHandlerWithMock(t)
	defer ctrl.Finish()

	svc.EXPECT().
		ListGroups(gomock.Any()).
		Return(nil, fmt.Errorf("database unavailable"))

	r := newGroupRouter(h)
	req := httptest.NewRequest(http.MethodGet, "/groups", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ── CreateGroup ───────────────────────────────────────────────────────────────

func TestCreateGroup_Valid(t *testing.T) {
	h, svc, ctrl := newGroupHandlerWithMock(t)
	defer ctrl.Finish()

	id := uuid.New()
	group := makeGroup(id, "infra")
	group.Description = "infra team"

	svc.EXPECT().
		CreateGroup(gomock.Any(), gomock.Any()).
		Return(group, nil)

	r := newGroupRouter(h)
	body := `{"name":"infra","description":"infra team","color":"#3b82f6"}`
	req := httptest.NewRequest(http.MethodPost, "/groups", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, id.String(), resp["id"])
	assert.Equal(t, "infra", resp["name"])
}

func TestCreateGroup_EmptyName(t *testing.T) {
	h, _, ctrl := newGroupHandlerWithMock(t)
	defer ctrl.Finish()

	r := newGroupRouter(h)
	body := `{"name":"","description":"some desc"}`
	req := httptest.NewRequest(http.MethodPost, "/groups", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateGroup_InvalidBody(t *testing.T) {
	h, _, ctrl := newGroupHandlerWithMock(t)
	defer ctrl.Finish()

	r := newGroupRouter(h)
	req := httptest.NewRequest(http.MethodPost, "/groups",
		bytes.NewBufferString(`{not valid json}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateGroup_DuplicateName(t *testing.T) {
	h, svc, ctrl := newGroupHandlerWithMock(t)
	defer ctrl.Finish()

	svc.EXPECT().
		CreateGroup(gomock.Any(), gomock.Any()).
		Return(nil, conflictErr("group", "name exists"))

	r := newGroupRouter(h)
	body := `{"name":"infra","description":"infra team"}`
	req := httptest.NewRequest(http.MethodPost, "/groups", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusConflict, w.Code)
}

// ── GetGroup ──────────────────────────────────────────────────────────────────

func TestGetGroup_ReturnsGroup(t *testing.T) {
	h, svc, ctrl := newGroupHandlerWithMock(t)
	defer ctrl.Finish()

	id := uuid.New()
	group := makeGroup(id, "backend")

	svc.EXPECT().
		GetGroup(gomock.Any(), id).
		Return(group, nil)

	r := newGroupRouter(h)
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/groups/%s", id), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, id.String(), resp["id"])
}

func TestGetGroup_InvalidUUID(t *testing.T) {
	h, _, ctrl := newGroupHandlerWithMock(t)
	defer ctrl.Finish()

	r := newGroupRouter(h)
	req := httptest.NewRequest(http.MethodGet, "/groups/not-a-uuid", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetGroup_NotFound(t *testing.T) {
	h, svc, ctrl := newGroupHandlerWithMock(t)
	defer ctrl.Finish()

	id := uuid.New()
	svc.EXPECT().
		GetGroup(gomock.Any(), id).
		Return(nil, notFoundErr("group", id.String()))

	r := newGroupRouter(h)
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/groups/%s", id), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// ── UpdateGroup ───────────────────────────────────────────────────────────────

func TestUpdateGroup_Valid(t *testing.T) {
	h, svc, ctrl := newGroupHandlerWithMock(t)
	defer ctrl.Finish()

	id := uuid.New()
	updated := makeGroup(id, "new-name")

	svc.EXPECT().
		UpdateGroup(gomock.Any(), id, gomock.Any()).
		Return(updated, nil)

	r := newGroupRouter(h)
	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/groups/%s", id),
		bytes.NewBufferString(`{"name":"new-name"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "new-name", resp["name"])
}

func TestUpdateGroup_InvalidUUID(t *testing.T) {
	h, _, ctrl := newGroupHandlerWithMock(t)
	defer ctrl.Finish()

	r := newGroupRouter(h)
	req := httptest.NewRequest(http.MethodPut, "/groups/not-a-uuid",
		bytes.NewBufferString(`{"name":"new-name"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ── DeleteGroup ───────────────────────────────────────────────────────────────

func TestDeleteGroup_ReturnsNoContent(t *testing.T) {
	h, svc, ctrl := newGroupHandlerWithMock(t)
	defer ctrl.Finish()

	id := uuid.New()
	svc.EXPECT().
		DeleteGroup(gomock.Any(), id).
		Return(nil)

	r := newGroupRouter(h)
	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/groups/%s", id), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestDeleteGroup_NotFound(t *testing.T) {
	h, svc, ctrl := newGroupHandlerWithMock(t)
	defer ctrl.Finish()

	id := uuid.New()
	svc.EXPECT().
		DeleteGroup(gomock.Any(), id).
		Return(notFoundErr("group", id.String()))

	r := newGroupRouter(h)
	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/groups/%s", id), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// ── ListGroupMembers ──────────────────────────────────────────────────────────

func TestListGroupMembers_ReturnsPaginatedHosts(t *testing.T) {
	h, svc, ctrl := newGroupHandlerWithMock(t)
	defer ctrl.Finish()

	groupID := uuid.New()
	host1 := makeHost(uuid.New(), "10.0.0.10")
	host2 := makeHost(uuid.New(), "10.0.0.11")

	// page=1, page_size=10 → offset=0, limit=10
	svc.EXPECT().
		GetGroupMembers(gomock.Any(), groupID, 0, 10).
		Return([]*db.Host{host1, host2}, int64(2), nil)

	r := newGroupRouter(h)
	req := httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/groups/%s/hosts?page=1&page_size=10", groupID), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	data, ok := resp["data"].([]interface{})
	require.True(t, ok, "expected 'data' key with a JSON array value")
	require.Len(t, data, 2)

	ips := []string{
		data[0].(map[string]interface{})["ip_address"].(string),
		data[1].(map[string]interface{})["ip_address"].(string),
	}
	assert.Contains(t, ips, "10.0.0.10")
	assert.Contains(t, ips, "10.0.0.11")
}

func TestListGroupMembers_InvalidUUID(t *testing.T) {
	h, _, ctrl := newGroupHandlerWithMock(t)
	defer ctrl.Finish()

	r := newGroupRouter(h)
	req := httptest.NewRequest(http.MethodGet, "/groups/not-a-uuid/hosts", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ── AddGroupMembers ───────────────────────────────────────────────────────────

func TestAddGroupMembers_Valid(t *testing.T) {
	h, svc, ctrl := newGroupHandlerWithMock(t)
	defer ctrl.Finish()

	groupID := uuid.New()
	hostID1 := uuid.New()
	hostID2 := uuid.New()

	svc.EXPECT().
		AddHostsToGroup(gomock.Any(), groupID, gomock.Any()).
		Return(nil)

	r := newGroupRouter(h)
	body := fmt.Sprintf(`{"host_ids":[%q,%q]}`, hostID1, hostID2)
	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/groups/%s/hosts", groupID),
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAddGroupMembers_InvalidHostUUID(t *testing.T) {
	h, _, ctrl := newGroupHandlerWithMock(t)
	defer ctrl.Finish()

	groupID := uuid.New()
	r := newGroupRouter(h)
	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/groups/%s/hosts", groupID),
		bytes.NewBufferString(`{"host_ids":["not-a-uuid"]}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAddGroupMembers_EmptyList(t *testing.T) {
	h, _, ctrl := newGroupHandlerWithMock(t)
	defer ctrl.Finish()

	groupID := uuid.New()
	r := newGroupRouter(h)
	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/groups/%s/hosts", groupID),
		bytes.NewBufferString(`{"host_ids":[]}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ── RemoveGroupMembers ────────────────────────────────────────────────────────

func TestRemoveGroupMembers_Valid(t *testing.T) {
	h, svc, ctrl := newGroupHandlerWithMock(t)
	defer ctrl.Finish()

	groupID := uuid.New()
	hostID := uuid.New()

	svc.EXPECT().
		RemoveHostsFromGroup(gomock.Any(), groupID, gomock.Any()).
		Return(nil)

	r := newGroupRouter(h)
	body := fmt.Sprintf(`{"host_ids":[%q]}`, hostID)
	req := httptest.NewRequest(http.MethodDelete,
		fmt.Sprintf("/groups/%s/hosts", groupID),
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRemoveGroupMembers_InvalidUUID(t *testing.T) {
	h, _, ctrl := newGroupHandlerWithMock(t)
	defer ctrl.Finish()

	hostID := uuid.New()
	r := newGroupRouter(h)
	body := fmt.Sprintf(`{"host_ids":[%q]}`, hostID)
	req := httptest.NewRequest(http.MethodDelete,
		"/groups/not-a-uuid/hosts",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUpdateGroup_InvalidBody(t *testing.T) {
	h, _, ctrl := newGroupHandlerWithMock(t)
	defer ctrl.Finish()

	id := uuid.New()
	req := httptest.NewRequest(http.MethodPut, "/groups/"+id.String(),
		strings.NewReader("{not valid json}"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	newGroupRouter(h).ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUpdateGroup_ServiceError(t *testing.T) {
	h, mock, ctrl := newGroupHandlerWithMock(t)
	defer ctrl.Finish()

	id := uuid.New()
	mock.EXPECT().UpdateGroup(gomock.Any(), id, gomock.Any()).
		Return(nil, fmt.Errorf("db timeout"))

	body := strings.NewReader(`{"name":"new-name"}`)
	req := httptest.NewRequest(http.MethodPut, "/groups/"+id.String(), body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	newGroupRouter(h).ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestListGroupMembers_ServiceError(t *testing.T) {
	h, mock, ctrl := newGroupHandlerWithMock(t)
	defer ctrl.Finish()

	id := uuid.New()
	mock.EXPECT().GetGroupMembers(gomock.Any(), id, gomock.Any(), gomock.Any()).
		Return(nil, int64(0), fmt.Errorf("db timeout"))

	req := httptest.NewRequest(http.MethodGet, "/groups/"+id.String()+"/hosts", nil)
	w := httptest.NewRecorder()
	newGroupRouter(h).ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestAddGroupMembers_ServiceError(t *testing.T) {
	h, mock, ctrl := newGroupHandlerWithMock(t)
	defer ctrl.Finish()

	groupID := uuid.New()
	hostID := uuid.New()
	mock.EXPECT().AddHostsToGroup(gomock.Any(), groupID, gomock.Any()).
		Return(fmt.Errorf("db timeout"))

	body := fmt.Sprintf(`{"host_ids":[%q]}`, hostID)
	req := httptest.NewRequest(http.MethodPost, "/groups/"+groupID.String()+"/hosts",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	newGroupRouter(h).ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestRemoveGroupMembers_ServiceError(t *testing.T) {
	h, mock, ctrl := newGroupHandlerWithMock(t)
	defer ctrl.Finish()

	groupID := uuid.New()
	hostID := uuid.New()
	mock.EXPECT().RemoveHostsFromGroup(gomock.Any(), groupID, gomock.Any()).
		Return(fmt.Errorf("db timeout"))

	body := fmt.Sprintf(`{"host_ids":[%q]}`, hostID)
	req := httptest.NewRequest(http.MethodDelete, "/groups/"+groupID.String()+"/hosts",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	newGroupRouter(h).ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
