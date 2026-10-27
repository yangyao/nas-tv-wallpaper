package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func fakeLibrary(t *testing.T) *Sampler {
	t.Helper()
	root := t.TempDir()
	for m := 1; m <= 12; m++ {
		dir := filepath.Join(root, "2025", time.Month(m).String())
		os.MkdirAll(dir, 0700)
		for n := 0; n < 12; n++ {
			os.WriteFile(filepath.Join(dir, string(rune('a'+n))+".jpg"), []byte("fake JPEG for sampler-only test"), 0600)
		}
	}
	s, e := NewSampler(root, filepath.Join(t.TempDir(), "history.json"))
	if e != nil {
		t.Fatal(e)
	}
	s.rng = rand.New(rand.NewSource(31))
	s.inspect = func(string) (PhotoMeta, error) {
		return PhotoMeta{Width: 4000, Height: 3000, Orientation: 1, Model: "Camera", Taken: "2025:01:01 12:00:00", Exposure: true}, nil
	}
	return s
}
func TestSamplingHistoryAndSpread(t *testing.T) {
	s := fakeLibrary(t)
	a, e := s.Batch(context.Background(), 20, nil)
	if e != nil || len(a) != 20 {
		t.Fatalf("first batch %d: %v", len(a), e)
	}
	seen := map[string]bool{}
	dirs := map[string]int{}
	for _, p := range a {
		if seen[p.ID] {
			t.Fatal("duplicate")
		}
		seen[p.ID] = true
		dirs[filepath.Dir(p.Path)]++
	}
	for _, n := range dirs {
		if n > 2 {
			t.Fatal("directory dominates")
		}
	}
	s2, e := NewSampler(s.root, s.stateFile)
	if e != nil {
		t.Fatal(e)
	}
	s2.inspect = s.inspect
	b, e := s2.Batch(context.Background(), 20, nil)
	if e != nil || len(b) != 20 {
		t.Fatalf("second batch %d %v", len(b), e)
	}
	for _, p := range b {
		if seen[p.ID] {
			t.Fatal("restart repeated a recent photo")
		}
	}
	if len(s2.windows) > 64 || s2.reads > 32 {
		t.Fatal("unbounded directory reads")
	}
}
func TestRejectedMetadataIsBounded(t *testing.T) {
	s := fakeLibrary(t)
	inspected := 0
	s.inspect = func(string) (PhotoMeta, error) { inspected++; return PhotoMeta{Width: 4000, Height: 3000}, nil }
	s.maxCandidates = 7
	p, e := s.Batch(context.Background(), 20, nil)
	if e != nil || len(p) != 0 || inspected > 7 {
		t.Fatalf("unbounded inspection: %d %v", inspected, e)
	}
	for _, name := range []string{"Screenshots", "下载", "Screen_shot.png", ".cache", "@eaDir"} {
		if !excluded(name) {
			t.Fatalf("not excluded: %s", name)
		}
	}
	if eligible(PhotoMeta{Width: 1920, Height: 1080, Model: "Phone", Taken: "2025:01:01"}) {
		t.Fatal("no exposure metadata")
	}
}
func TestSymlinkDoesNotEscape(t *testing.T) {
	s := fakeLibrary(t)
	outside := filepath.Join(t.TempDir(), "secret.jpg")
	os.WriteFile(outside, []byte("private"), 0600)
	os.Symlink(outside, filepath.Join(s.root, "escape.jpg"))
	if _, e := s.safePath("escape.jpg"); e == nil {
		t.Fatal("symlink escaped")
	}
	if _, e := s.safePath("../outside"); e == nil {
		t.Fatal("traversal accepted")
	}
}
func TestAuthorizationAndRequestBounds(t *testing.T) {
	s := fakeLibrary(t)
	app := &Server{sampler: s, token: "test-token-not-for-production", cache: t.TempDir(), batchGate: make(chan struct{}, 1), imageGate: make(chan struct{}, 1)}
	h := app.handler()
	for _, tc := range []struct {
		auth, body string
		status     int
	}{{"", `{"count":20}`, 401}, {"Bearer wrong", `{}`, 401}, {"Bearer " + app.token, `{"count":100}`, 400}, {"Bearer " + app.token, `{"count":5}`, 200}} {
		r := httptest.NewRequest(http.MethodPost, "/v1/batch", bytes.NewBufferString(tc.body))
		r.Header.Set("Authorization", tc.auth)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("status %d want %d: %s", w.Code, tc.status, w.Body.String())
		}
		if w.Code == 200 {
			var body map[string]interface{}
			json.Unmarshal(w.Body.Bytes(), &body)
			if bytes.Contains(w.Body.Bytes(), []byte(s.root)) {
				t.Fatal("API leaked filesystem path")
			}
		}
	}
}
func TestEXIFParserAndOrientation(t *testing.T) {
	b := make([]byte, 150)
	copy(b, "II")
	binary.LittleEndian.PutUint16(b[2:], 42)
	binary.LittleEndian.PutUint32(b[4:], 8)
	binary.LittleEndian.PutUint16(b[8:], 3)
	entry := func(p int, tag, kind uint16, count, value uint32) {
		binary.LittleEndian.PutUint16(b[p:], tag)
		binary.LittleEndian.PutUint16(b[p+2:], kind)
		binary.LittleEndian.PutUint32(b[p+4:], count)
		binary.LittleEndian.PutUint32(b[p+8:], value)
	}
	entry(10, 0x110, 2, 7, 100)
	copy(b[100:], "Camera\x00")
	entry(22, 0x112, 3, 1, 6)
	entry(34, 0x8769, 4, 1, 50)
	binary.LittleEndian.PutUint16(b[50:], 2)
	entry(52, 0x9003, 2, 20, 110)
	copy(b[110:], "2025:01:01 12:00:00\x00")
	entry(64, 0x829a, 5, 1, 140)
	m := PhotoMeta{}
	readTIFF(b, &m)
	if m.Model != "Camera" || m.Orientation != 6 || m.Taken != "2025:01:01 12:00:00" || !m.Exposure {
		t.Fatalf("metadata: %+v", m)
	}
	for i := 0; i < len(b); i++ {
		readTIFF(b[:i], &PhotoMeta{})
	}
	src := image.NewNRGBA(image.Rect(0, 0, 2, 3))
	src.Set(0, 2, color.NRGBA{255, 0, 0, 255})
	o := orientedImage{src, 6}
	if o.Bounds().Dx() != 3 || o.Bounds().Dy() != 2 {
		t.Fatal("rotated dimensions")
	}
	r, _, _, _ := o.At(0, 0).RGBA()
	if r != 65535 {
		t.Fatal("EXIF rotation wrong")
	}
}
func TestRenderFitsWithoutCropping(t *testing.T) {
	path := filepath.Join(t.TempDir(), "portrait.jpg")
	f, _ := os.Create(path)
	src := image.NewNRGBA(image.Rect(0, 0, 100, 200))
	for y := 0; y < 200; y++ {
		for x := 0; x < 100; x++ {
			src.Set(x, y, color.NRGBA{210, 30, 40, 255})
		}
	}
	jpeg.Encode(f, src, nil)
	f.Close()
	out, e := renderPhoto(context.Background(), path, 1)
	if e != nil || out.Bounds() != image.Rect(0, 0, 1920, 1080) {
		t.Fatal(e)
	}
	center := out.NRGBAAt(960, 540)
	side := out.NRGBAAt(50, 540)
	if center.R < 180 || side.R >= center.R {
		t.Fatal("portrait not contained over dim backdrop")
	}
}

func TestPhotoDimensionsHeadWithoutReadingOriginal(t *testing.T) {
	s := fakeLibrary(t)
	id := "0123456789abcdef0123456789abcdef"
	for _, tc := range []struct {
		orientation   int
		width, height string
	}{{1, "4000", "3000"}, {6, "3000", "4000"}, {8, "3000", "4000"}} {
		s.history.Photos[id] = Photo{ID: id, Path: "does-not-exist.jpg", Meta: PhotoMeta{Width: 4000, Height: 3000, Orientation: tc.orientation}}
		app := &Server{sampler: s, token: "test-token-not-for-production", cache: t.TempDir(), imageGate: make(chan struct{}, 1)}
		for _, auth := range []bool{false, true} {
			r := httptest.NewRequest(http.MethodHead, "/v1/photos/"+id, nil)
			if auth {
				r.Header.Set("Authorization", "Bearer "+app.token)
			}
			w := httptest.NewRecorder()
			app.handler().ServeHTTP(w, r)
			if !auth {
				if w.Code != 401 || w.Header().Get("X-Photo-Width") != "" {
					t.Fatal("metadata must require authorization")
				}
				continue
			}
			if w.Code != 200 || w.Body.Len() != 0 || w.Header().Get("X-Photo-Width") != tc.width || w.Header().Get("X-Photo-Height") != tc.height {
				t.Fatalf("bad HEAD: %d %v", w.Code, w.Header())
			}
		}
	}
}
