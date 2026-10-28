package main

import (
	"bytes"
	"encoding/json"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func managementRequest(s *Server, method, path, body, peer string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.RemoteAddr = peer
	w := httptest.NewRecorder()
	s.handler().ServeHTTP(w, r)
	return w
}
func TestManagementLANReadOnlyWithoutLogin(t *testing.T) {
	q, _ := newPaintQueue(t.TempDir(), false, true)
	p, _ := newPublishedStore(t.TempDir(), "light")
	job := addPublishedTestJob(t, q, 1, "light", "approved")
	p.refresh(q, time.Now())
	s := &Server{published: p, outpaint: q, token: "test-management-secret-never-in-json", cache: t.TempDir(), imageGate: make(chan struct{}, 1), startedAt: time.Now()}
	peer := "192.168.1.20:12345"
	shell := managementRequest(s, "GET", "/manage", "", peer)
	if shell.Code != 200 || bytes.Contains(shell.Body.Bytes(), []byte("login-form")) {
		t.Fatal("direct LAN page unavailable or requires login")
	}
	overview := managementRequest(s, "GET", "/manage/api/overview", "", peer)
	if overview.Code != 200 || bytes.Contains(overview.Body.Bytes(), []byte(s.token)) || bytes.Contains(overview.Body.Bytes(), []byte(q.dir)) {
		t.Fatal("overview unavailable or leaked secret/path")
	}
	var payload struct {
		Available int `json:"available_batches"`
		Photos    int `json:"current_photos"`
	}
	json.Unmarshal(overview.Body.Bytes(), &payload)
	if payload.Available != 1 || payload.Photos != 1 {
		t.Fatal("wrong overview")
	}
	if managementRequest(s, "POST", "/v1/outpaint/jobs/"+job.ID+"/decision", `{"status":"rejected"}`, peer).Code != 401 {
		t.Fatal("LAN read-only access allowed mutation")
	}
	if managementRequest(s, "GET", "/v2/wallpapers/latest", "", peer).Code != 401 {
		t.Fatal("existing TV authentication weakened")
	}
	batch, _ := p.latest()
	hash := batch.Photos[0].SHA256
	thumb := managementRequest(s, "GET", "/manage/thumbs/asset-"+hash+".jpg", "", peer)
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(thumb.Body.Bytes()))
	if thumb.Code != 200 || err != nil || cfg.Width != 480 || cfg.Height != 270 {
		t.Fatal("invalid thumbnail", thumb.Code, err)
	}
	if managementRequest(s, "GET", "/manage/api/images/"+hash+".jpg", "", peer).Code != 200 {
		t.Fatal("large preview failed")
	}
	q.Serve = false
	if managementRequest(s, "GET", "/manage/api/overview", "", peer).Code != 200 {
		t.Fatal("paused delivery hid dashboard")
	}
	if managementRequest(s, "GET", "/manage/thumbs/asset-"+hash+".jpg", "", peer).Code != 200 {
		t.Fatal("paused delivery hid preview")
	}
}
func TestManagementRejectsNonLANClients(t *testing.T) {
	s := &Server{token: "test-token"}
	for _, path := range []string{"/manage", "/manage/app.js", "/manage/api/overview", "/manage/thumbs/asset-invalid.jpg"} {
		if managementRequest(s, http.MethodGet, path, "", "8.8.8.8:443").Code != 403 {
			t.Fatal("non-LAN access allowed", path)
		}
	}
	r := httptest.NewRequest("GET", "/manage/api/overview", nil)
	r.RemoteAddr = "8.8.8.8:123"
	r.Header.Set("X-Forwarded-For", "192.168.1.1")
	w := httptest.NewRecorder()
	s.handler().ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("untrusted forwarded header bypassed LAN boundary")
	}
}

func TestDashboardMigrationExcludesOldPendingAndFailureCodePersists(t *testing.T) {
	q, _ := newPaintQueue(t.TempDir(), false, true)
	old, _ := q.enqueue(Photo{ID: strings.Repeat("a", 32), Meta: PhotoMeta{Width: 300, Height: 400, Taken: "2010:01:01 00:00:00"}}, "light", "")
	fresh, _ := q.enqueue(Photo{ID: strings.Repeat("b", 32), Meta: PhotoMeta{Width: 300, Height: 400, Taken: "2025:01:01 00:00:00"}}, "light", "")
	q.maxAgeYears = 10
	if err := q.reconcile(fakeLibrary(t)); err != nil {
		t.Fatal(err)
	}
	j, _ := q.find(old.ID)
	if j.Status != "rejected" || j.ErrorCode != "capture_date_out_of_range" {
		t.Fatal("old pending task still looks runnable")
	}
	s := &Server{outpaint: q, token: "test-management-token"}
	call := func(path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+s.token)
		r.Header.Set("X-Worker-ID", "test-worker")
		w := httptest.NewRecorder()
		s.handler().ServeHTTP(w, r)
		return w
	}
	if call("/v1/outpaint/claim", `{"worker":"test-worker","workflow":"light"}`).Code != 200 {
		t.Fatal("fresh claim failed")
	}
	if call("/v1/outpaint/jobs/"+fresh.ID+"/fail", `{"error_code":"network_error"}`).Code != 200 {
		t.Fatal("failure detail rejected")
	}
	loaded, err := newPaintQueue(q.dir, false, true)
	if err != nil {
		t.Fatal(err)
	}
	j, _ = loaded.find(fresh.ID)
	if j.Status != "failed" || j.ErrorCode != "network_error" {
		t.Fatal("failure reason not persisted")
	}
}
