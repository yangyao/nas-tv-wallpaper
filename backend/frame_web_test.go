package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFrameWebLANAndOriginBoundaries(t *testing.T) {
	sampler := fakeLibrary(t)
	sampler.originals = true
	app := &Server{frames: sampler, token: "test-token-not-for-production", batchGate: make(chan struct{}, 1), imageGate: make(chan struct{}, 1)}
	call := func(method, path, remote, origin string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "http://frame.local"+path, bytes.NewBufferString(`{"count":3}`))
		r.RemoteAddr = remote
		r.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		app.handler().ServeHTTP(w, r)
		return w
	}
	for _, path := range []string{"/frame/", "/frame/app.js", "/frame/core.mjs", "/frame/style.css", "/frame/sw.js", "/frame/manifest.webmanifest", "/frame/icon.svg"} {
		if w := call("GET", path, "203.0.113.2:2345", ""); w.Code != 403 {
			t.Fatal("public access", path, w.Code)
		}
		w := call("GET", path, "10.0.0.2:2345", "")
		if w.Code != 200 {
			t.Fatal(path, w.Code)
		}
		if strings.Contains(w.Body.String(), app.token) {
			t.Fatal("token leak")
		}
	}
	for _, origin := range []string{"", "http://unrelated.example", "http://frame.local.evil"} {
		if w := call("POST", "/frame/api/batch", "10.0.0.2:2345", origin); w.Code != 403 {
			t.Fatal("cross origin sampling", origin, w.Code)
		}
	}
	w := call("POST", "/frame/api/batch", "10.0.0.2:2345", "http://frame.local")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var batch struct {
		Photos []struct {
			ID            string
			Width, Height int
		}
	}
	if err := json.Unmarshal(w.Body.Bytes(), &batch); err != nil || len(batch.Photos) != 3 {
		t.Fatal(err, w.Body.String())
	}
	if batch.Photos[0].Width != 4000 {
		t.Fatal("decode bounds missing")
	}
	image := call("GET", "/frame/photos/"+batch.Photos[0].ID, "10.0.0.2:2345", "")
	if image.Code != 200 || image.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("original caching policy", image.Code, image.Header())
	}
	if w := call("POST", "/v1/frame/batch", "10.0.0.2:2345", ""); w.Code != http.StatusUnauthorized {
		t.Fatal("Android bearer boundary changed")
	}
	if w := call("GET", "/frame/not-a-file", "10.0.0.2:2345", ""); w.Code == 200 {
		t.Fatal("arbitrary embed access")
	}
}
