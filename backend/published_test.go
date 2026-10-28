package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func addPublishedTestJob(t *testing.T, q *PaintQueue, n int, workflow, status string) PaintJob {
	t.Helper()
	photo := Photo{ID: fmt.Sprintf("%032x", n), Meta: PhotoMeta{Width: 300, Height: 400, Taken: "2025:01:01 12:00:00"}}
	j, err := q.enqueue(photo, workflow, "")
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 1920, 1080))
	c := color.RGBA{uint8(n * 29), uint8(n * 37), uint8(n * 53), 255}
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i] = c.R
		img.Pix[i+1] = c.G
		img.Pix[i+2] = c.B
		img.Pix[i+3] = 255
	}
	var b bytes.Buffer
	jpeg.Encode(&b, img, &jpeg.Options{Quality: 80})
	os.WriteFile(filepath.Join(q.dir, j.ID+".jpg"), b.Bytes(), 0600)
	for i := range q.Jobs {
		if q.Jobs[i].ID == j.ID {
			q.Jobs[i].Status = status
		}
	}
	return j
}
func publishedCall(s *Server, method, path, etag string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, bytes.NewBufferString(`{"count":20,"exclude":[]}`))
	r.Header.Set("Authorization", "Bearer "+s.token)
	r.Header.Set("If-None-Match", etag)
	w := httptest.NewRecorder()
	s.handler().ServeHTTP(w, r)
	return w
}
func TestPublishedOnlyCompleteCurrentRecipeAndLegacyNoRawFill(t *testing.T) {
	q, _ := newPaintQueue(t.TempDir(), false, true)
	p, _ := newPublishedStore(t.TempDir(), "light")
	j := addPublishedTestJob(t, q, 1, "light", "approved")
	addPublishedTestJob(t, q, 2, "light", "review")
	addPublishedTestJob(t, q, 3, "old", "approved")
	if err := p.refresh(q, time.Now()); err != nil {
		t.Fatal(err)
	}
	s := &Server{published: p, outpaint: q, token: "test-only-published-token"}
	w := publishedCall(s, "GET", "/v2/wallpapers/latest", "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var b PublishedBatch
	json.Unmarshal(w.Body.Bytes(), &b)
	if len(b.Photos) != 1 {
		t.Fatal("unreviewed or old recipe leaked")
	}
	asset := b.Photos[0]
	w = publishedCall(s, "GET", asset.URL, "")
	sum := sha256.Sum256(w.Body.Bytes())
	if w.Code != 200 || hex.EncodeToString(sum[:]) != asset.SHA256 || w.Header().Get("X-Photo-Width") != "1920" {
		t.Fatal("invalid published asset")
	}
	if publishedCall(s, "GET", "/v2/wallpapers/latest", `"`+b.ID+`"`).Code != 304 {
		t.Fatal("unchanged manifest not conditional")
	}
	w = publishedCall(s, "POST", "/v1/batch", "")
	var legacy struct {
		Photos []PublishedPhoto `json:"photos"`
	}
	json.Unmarshal(w.Body.Bytes(), &legacy)
	if w.Code != 200 || len(legacy.Photos) != 1 {
		t.Fatal("legacy batch filled with raw photos")
	}
	if publishedCall(s, "GET", "/v1/photos/"+j.PhotoID, "").Code != 404 {
		t.Fatal("original image fallback leaked")
	}
	if publishedCall(s, "GET", "/v1/photos/"+asset.ID, "").Code != 200 {
		t.Fatal("legacy ready asset unavailable")
	}
	q.Serve = false
	if publishedCall(s, "GET", "/v2/wallpapers/latest", "").Code != 503 || publishedCall(s, "POST", "/v1/batch", "").Code != 503 {
		t.Fatal("disabled publishing must preserve client cache")
	}
}
func TestPublishedSoftExpiryFailureAndPersistence(t *testing.T) {
	q, _ := newPaintQueue(t.TempDir(), false, true)
	dir := t.TempDir()
	p, _ := newPublishedStore(dir, "light")
	now := time.Now()
	job := addPublishedTestJob(t, q, 1, "light", "approved")
	if err := p.refresh(q, now); err != nil {
		t.Fatal(err)
	}
	first, _ := p.latest()
	p.refresh(q, now.Add(time.Hour))
	same, _ := p.latest()
	if same.ID != first.ID {
		t.Fatal("unnecessary publication")
	}
	// Losing every producer/source file cannot destroy a completed publication.
	os.Remove(filepath.Join(q.dir, job.ID+".jpg"))
	p.refresh(q, now.Add(2*publishedTTL))
	retained, ok := p.latest()
	if !ok || retained.ID != first.ID {
		t.Fatal("soft TTL deleted last completed batch")
	}
	loaded, err := newPublishedStore(dir, "light")
	if err != nil {
		t.Fatal(err)
	}
	b, ok := loaded.latest()
	if !ok || b.ID != first.ID {
		t.Fatal("restart lost complete batch")
	}
	addPublishedTestJob(t, q, 2, "light", "approved")
	os.Mkdir(filepath.Join(dir, "index.json.tmp"), 0700)
	if p.refresh(q, now.Add(3*publishedTTL)) == nil {
		t.Fatal("expected failed publication")
	}
	b, _ = p.latest()
	if b.ID != first.ID {
		t.Fatal("failed publication replaced current pointer")
	}
}
func TestPublishedTenVersionsGarbageCollectionAndConcurrentReads(t *testing.T) {
	q, _ := newPaintQueue(t.TempDir(), false, true)
	dir := t.TempDir()
	p, _ := newPublishedStore(dir, "light")
	now := time.Now()
	var firstHash string
	var readers sync.WaitGroup
	stop := make(chan struct{})
	readers.Add(1)
	go func() {
		defer readers.Done()
		for {
			select {
			case <-stop:
				return
			default:
				if b, ok := p.latest(); ok {
					p.asset(b.Photos[0].SHA256)
				}
			}
		}
	}()
	for i := 1; i <= 12; i++ {
		q.mu.Lock()
		for j := range q.Jobs {
			q.Jobs[j].Status = "rejected"
		}
		q.mu.Unlock()
		addPublishedTestJob(t, q, i, "light", "approved")
		if err := p.refresh(q, now.Add(time.Duration(i)*publishedTTL)); err != nil {
			t.Fatal(err)
		}
		b, _ := p.latest()
		if i == 1 {
			firstHash = b.Photos[0].SHA256
		}
	}
	close(stop)
	readers.Wait()
	if len(p.index.Batches) != 10 {
		t.Fatal("retention not bounded to ten")
	}
	if _, err := os.Stat(p.assetPath(firstHash)); !os.IsNotExist(err) {
		t.Fatal("unreferenced asset retained")
	}
	for _, b := range p.index.Batches {
		if !p.complete(b) {
			t.Fatal("retained batch lost referenced asset")
		}
	}
	entries, _ := os.ReadDir(filepath.Join(dir, "assets"))
	if len(entries) != 10 {
		t.Fatal("asset GC failed", len(entries))
	}
}
func TestPublishedEmptyPoolDoesNotInventBatch(t *testing.T) {
	q, _ := newPaintQueue(t.TempDir(), false, true)
	p, _ := newPublishedStore(t.TempDir(), "light")
	p.refresh(q, time.Now())
	s := &Server{published: p, outpaint: q, token: "test"}
	if publishedCall(s, http.MethodGet, "/v2/wallpapers/latest", "").Code != 503 {
		t.Fatal("empty pool must not expose raw fallback")
	}
}

func TestCaptureDateRollingWindow(t *testing.T) {
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.Local)
	for _, tc := range []struct {
		taken string
		ok    bool
	}{{"2016:10:28 00:00:00", true}, {"2016:10:27 23:59:59", false}, {"2010:01:01 12:00:00", false}, {"2026:10:28 12:00:00", true}, {"2026:10:29 00:00:00", false}, {"", false}, {"2025:02:30 12:00:00", false}, {"2020:01:01 99:99:99", false}} {
		if got := recentTaken(tc.taken, now, 10); got != tc.ok {
			t.Fatalf("capture %q: %v", tc.taken, got)
		}
	}
	if got := captureValidUntil("2016:10:28 12:00:00", 10); got != time.Date(2026, 10, 29, 0, 0, 0, 0, time.Local).Unix() {
		t.Fatal("invalid hard validity")
	}
}
func TestPublishedRejectsOldOrUnknownCaptureEvenWithNewFileTime(t *testing.T) {
	q, _ := newPaintQueue(t.TempDir(), false, true)
	p, _ := newPublishedStore(t.TempDir(), "light")
	old := addPublishedTestJob(t, q, 1, "light", "approved")
	unknown := addPublishedTestJob(t, q, 2, "light", "approved")
	fresh := addPublishedTestJob(t, q, 3, "light", "approved")
	a := q.Sources[old.PhotoID]
	a.Meta.Taken = "2001:01:01 12:00:00"
	q.Sources[old.PhotoID] = a
	a = q.Sources[unknown.PhotoID]
	a.Meta.Taken = ""
	q.Sources[unknown.PhotoID] = a
	if err := p.refresh(q, time.Now()); err != nil {
		t.Fatal(err)
	}
	b, ok := p.latest()
	if !ok || len(b.Photos) != 1 || b.Photos[0].Taken != q.Sources[fresh.PhotoID].Meta.Taken || b.ValidUntil == 0 {
		t.Fatal("old or missing capture date published")
	}
}

func TestCaptureLeapDayExpiry(t *testing.T) {
	if captureValidUntil("2020:02:29 12:00:00", 10) != time.Date(2030, 3, 1, 0, 0, 0, 0, time.Local).Unix() {
		t.Fatal("leap-day capture retained past ten-year window")
	}
}

func TestSoftExpiryWithoutDifferentReadyContentKeepsSameVersion(t *testing.T) {
	q, _ := newPaintQueue(t.TempDir(), false, true)
	p, _ := newPublishedStore(t.TempDir(), "light")
	now := time.Now()
	addPublishedTestJob(t, q, 1, "light", "approved")
	if err := p.refresh(q, now); err != nil {
		t.Fatal(err)
	}
	first, _ := p.latest()
	if err := p.refresh(q, now.Add(2*publishedTTL)); err != nil {
		t.Fatal(err)
	}
	same, _ := p.latest()
	if same.ID != first.ID || len(p.index.Batches) != 1 {
		t.Fatal("no new content must keep old batch, not invent duplicate versions")
	}
}

func TestPublishedNewResultsDoNotWaitForRotationWindow(t *testing.T) {
	q, _ := newPaintQueue(t.TempDir(), false, true)
	p, _ := newPublishedStore(t.TempDir(), "light")
	now := time.Now()
	addPublishedTestJob(t, q, 1, "light", "approved")
	if err := p.refresh(q, now); err != nil {
		t.Fatal(err)
	}
	first, _ := p.latest()
	addPublishedTestJob(t, q, 2, "light", "approved")
	if err := p.refresh(q, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	waiting, _ := p.latest()
	if waiting.ID == first.ID || len(waiting.Photos) != 2 {
		t.Fatal("new completed result was held behind the refresh window")
	}
	if err := p.refresh(q, now.Add(publishedTTL)); err != nil {
		t.Fatal(err)
	}
	next, _ := p.latest()
	if next.ID != waiting.ID || len(p.index.Batches) != 2 {
		t.Fatal("unchanged content produced an extra publication")
	}
}
