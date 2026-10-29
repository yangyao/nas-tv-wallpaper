package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestFrameOriginalsIndependentOfCuratedEligibility(t *testing.T) {
	s := fakeLibrary(t)
	s.originals = true
	s.inspect = func(string) (PhotoMeta, error) {
		return PhotoMeta{Width: 4000, Height: 3000, Orientation: 6, Taken: "2001:01:01"}, nil
	}
	app := &Server{frames: s, token: "test-token-not-for-production", batchGate: make(chan struct{}, 1), imageGate: make(chan struct{}, 1)}
	request := func(method, path, body, auth string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		r.Header.Set("Authorization", auth)
		w := httptest.NewRecorder()
		app.handler().ServeHTTP(w, r)
		return w
	}
	if w := request("POST", "/v1/frame/batch", `{}`, ""); w.Code != 401 {
		t.Fatal(w.Code)
	}
	for _, body := range []string{`{"count":21}`, `{"count":-1}`, `{"exclude":["../escape"]}`} {
		if w := request("POST", "/v1/frame/batch", body, "Bearer "+app.token); w.Code != 400 {
			t.Fatal(w.Code)
		}
	}
	w := request("POST", "/v1/frame/batch", `{"count":5}`, "Bearer "+app.token)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var batch struct {
		Photos []struct {
			ID, URL     string
			Orientation int
		}
	}
	if err := json.Unmarshal(w.Body.Bytes(), &batch); err != nil || len(batch.Photos) != 5 {
		t.Fatal(err, w.Body.String())
	}
	if bytes.Contains(w.Body.Bytes(), []byte(s.root)) {
		t.Fatal("path leaked")
	}
	p := batch.Photos[0]
	if p.Orientation != 6 {
		t.Fatal("orientation lost")
	}
	image := request("GET", p.URL, "", "Bearer "+app.token)
	if image.Code != http.StatusOK || image.Body.String() != "fake JPEG for sampler-only test" {
		t.Fatal("original not streamed", image.Code)
	}
	original, _ := s.Photo(p.ID)
	os.WriteFile(filepath.Join(s.root, original.Path), []byte("changed"), 0600)
	if image = request("GET", p.URL, "", "Bearer "+app.token); image.Code != 404 {
		t.Fatal("changed source served")
	}
	if s.reads > 32 || len(s.windows) > 64 {
		t.Fatal("unbounded sampling")
	}
}

func TestFrameRecentRepeatKeepsCurrentExcluded(t *testing.T) {
	s := fakeLibrary(t)
	s.originals = true
	first, err := s.sample(context.Background(), 20, nil, true)
	if err != nil || len(first) != 20 {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		_, _ = s.sample(context.Background(), 20, nil, true)
	}
	current := first[0].ID
	next, err := s.sample(context.Background(), 20, []string{current}, true)
	if err != nil || len(next) == 0 {
		t.Fatal(err)
	}
	for _, p := range next {
		if p.ID == current {
			t.Fatal("current repeated")
		}
	}
}
