package main

import (
	"bytes"
	"encoding/json"
	"image"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestOutpaintLeaseReviewAndPersistence(t *testing.T) {
	dir := t.TempDir()
	q, e := newPaintQueue(dir, false, false)
	if e != nil {
		t.Fatal(e)
	}
	p := Photo{ID: "0123456789abcdef0123456789abcdef", Meta: PhotoMeta{Width: 3000, Height: 4000}}
	job, e := q.enqueue(p, "test-v1", "")
	if e != nil {
		t.Fatal(e)
	}
	same, _ := q.enqueue(p, "test-v1", "")
	if job.ID != same.ID || len(q.Jobs) != 1 {
		t.Fatal("duplicate job")
	}
	app := &Server{outpaint: q, token: "test-worker-token-not-real"}
	h := app.handler()
	call := func(method, path string, body []byte, worker string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "/v1/outpaint/"+path, bytes.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+app.token)
		r.Header.Set("X-Worker-ID", worker)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	w := call("POST", "claim", []byte(`{"worker":"one","workflow":"test-v1"}`), "")
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var claimed PaintJob
	json.Unmarshal(w.Body.Bytes(), &claimed)
	if claimed.Worker != "one" || claimed.Lease <= time.Now().Unix() {
		t.Fatal("lease missing")
	}
	if call("POST", "claim", []byte(`{"worker":"two","workflow":"test-v1"}`), "").Code != 204 {
		t.Fatal("double claim")
	}
	if call("PUT", "jobs/"+job.ID+"/result", []byte("bad"), "two").Code != 409 {
		t.Fatal("wrong worker accepted")
	}
	if call("PUT", "jobs/"+job.ID+"/result", []byte("bad"), "one").Code != 400 {
		t.Fatal("invalid JPEG accepted")
	}
	var imageData bytes.Buffer
	jpeg.Encode(&imageData, image.NewRGBA(image.Rect(0, 0, 1920, 1080)), nil)
	if w = call("PUT", "jobs/"+job.ID+"/result", imageData.Bytes(), "one"); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if q.approved(p.ID) != "" {
		t.Fatal("unreviewed result served")
	}
	if call("POST", "jobs/"+job.ID+"/decision", []byte(`{"status":"approved"}`), "").Code != 200 {
		t.Fatal("approval failed")
	}
	if q.approved(p.ID) != "" {
		t.Fatal("serve switch ignored")
	}
	reloaded, e := newPaintQueue(dir, false, true)
	if e != nil || reloaded.approved(p.ID) == "" {
		t.Fatal("approval not persisted")
	}
	if len(reloaded.selectApproved(1, nil)) != 1 || len(reloaded.selectApproved(1, []string{p.ID})) != 0 {
		t.Fatal("approved pool/exclusion failed")
	}
	if call("POST", "jobs/"+job.ID+"/decision", []byte(`{"status":"rejected"}`), "").Code != 200 {
		t.Fatal("reject failed")
	}
	q.Serve = true
	if q.approved(p.ID) != "" {
		t.Fatal("rejected served")
	}
}
func TestOutpaintDisabledAndAuth(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		s := &Server{token: "test-auth-token-not-real"}
		if enabled {
			s.outpaint, _ = newPaintQueue(t.TempDir(), false, false)
		}
		r := httptest.NewRequest(http.MethodGet, "/v1/outpaint/jobs", nil)
		w := httptest.NewRecorder()
		s.handler().ServeHTTP(w, r)
		if w.Code != 401 {
			t.Fatal("unprotected queue")
		}
		r.Header.Set("Authorization", "Bearer "+s.token)
		w = httptest.NewRecorder()
		s.handler().ServeHTTP(w, r)
		want := 404
		if enabled {
			want = 200
		}
		if w.Code != want {
			t.Fatal("feature gate")
		}
	}
}

func TestOutpaintAutofillClaimsImmediatelyAndRecoversLease(t *testing.T) {
	s := fakeLibrary(t)
	inspect := s.inspect
	s.inspect = func(path string) (PhotoMeta, error) { m, e := inspect(path); m.Orientation = 6; return m, e }
	q, _ := newPaintQueue(t.TempDir(), true, false)
	app := &Server{sampler: s, outpaint: q, token: "test-queue-auth-not-real"}
	claim := func(worker string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/v1/outpaint/claim", bytes.NewBufferString(`{"worker":"`+worker+`","workflow":"v1"}`))
		r.Header.Set("Authorization", "Bearer "+app.token)
		w := httptest.NewRecorder()
		app.handler().ServeHTTP(w, r)
		return w
	}
	w := claim("first")
	if w.Code != 200 {
		t.Fatalf("autofill did not lease a portrait: %d %s", w.Code, w.Body.String())
	}
	var job PaintJob
	json.Unmarshal(w.Body.Bytes(), &job)
	if job.Status != "processing" || job.Attempts != 1 {
		t.Fatal("bad initial lease")
	}
	q.Autofill = false
	q.Jobs[0].Lease = time.Now().Unix() - 1
	w = claim("replacement")
	json.Unmarshal(w.Body.Bytes(), &job)
	if w.Code != 200 || job.Worker != "replacement" || job.Attempts != 2 {
		t.Fatal("expired lease not recovered")
	}
	q.Jobs[0].Status = "failed"
	q.Jobs[0].Updated = time.Now().Unix()
	if claim("too-soon").Code != 204 {
		t.Fatal("failure backoff ignored")
	}
	q.Jobs[0].Updated = time.Now().Unix() - 3601
	w = claim("retry")
	json.Unmarshal(w.Body.Bytes(), &job)
	if w.Code != 200 || job.Attempts != 3 {
		t.Fatal("delayed retry failed")
	}
	q.Jobs[0].Lease = time.Now().Unix() - 1
	if claim("exhausted").Code != 204 {
		t.Fatal("attempt budget ignored")
	}
	r := httptest.NewRequest("GET", "/v1/outpaint/jobs", nil)
	r.Header.Set("Authorization", "Bearer "+app.token)
	w = httptest.NewRecorder()
	app.handler().ServeHTTP(w, r)
	for _, source := range q.Sources {
		if bytes.Contains(w.Body.Bytes(), []byte(source.Path)) {
			t.Fatal("source filesystem path leaked")
		}
	}
}
