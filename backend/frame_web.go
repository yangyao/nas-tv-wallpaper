package main

import (
	"embed"
	"net/http"
	"net/url"
	"strings"
)

//go:embed frameweb/*
var frameWebFiles embed.FS

var frameWebTypes = map[string]string{
	"index.html": "text/html; charset=utf-8", "app.js": "text/javascript; charset=utf-8",
	"core.mjs": "text/javascript; charset=utf-8", "style.css": "text/css; charset=utf-8",
	"sw.js": "text/javascript; charset=utf-8", "manifest.webmanifest": "application/manifest+json",
	"icon.svg": "image/svg+xml",
}

func frameWebPublic(path string) bool {
	if path == "/frame" || path == "/frame/" || path == "/frame/api/batch" || strings.HasPrefix(path, "/frame/photos/") {
		return true
	}
	_, ok := frameWebTypes[strings.TrimPrefix(path, "/frame/")]
	return strings.HasPrefix(path, "/frame/") && ok
}
func (s *Server) frameWeb(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" && r.Method != "HEAD" {
		http.Error(w, "GET required", 405)
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/frame/")
	if r.URL.Path == "/frame" || name == "" {
		name = "index.html"
	}
	mime, ok := frameWebTypes[name]
	if !ok {
		http.NotFound(w, r)
		return
	}
	data, err := frameWebFiles.ReadFile("frameweb/" + name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' blob:; connect-src 'self'; worker-src 'self'; object-src 'none'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
	if r.Method == "GET" {
		w.Write(data)
	}
}
func (s *Server) frameWebBatch(w http.ResponseWriter, r *http.Request) {
	// LAN access is intentionally account-free, like /manage. Never publish the
	// Android/worker bearer credential into HTML. Cross-origin sampling is denied.
	origin, err := url.Parse(r.Header.Get("Origin"))
	if err != nil || origin.Host != r.Host || (origin.Scheme != "http" && origin.Scheme != "https") || origin.Path != "" || r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		http.Error(w, "same origin required", 403)
		return
	}
	s.frameBatch(w, r)
}

type frameNoStore struct{ http.ResponseWriter }

func (w frameNoStore) WriteHeader(code int) {
	w.Header().Set("Cache-Control", "no-store")
	w.ResponseWriter.WriteHeader(code)
}
func (w frameNoStore) Write(data []byte) (int, error) {
	w.Header().Set("Cache-Control", "no-store")
	return w.ResponseWriter.Write(data)
}
func (s *Server) frameWebImage(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		http.Error(w, "same origin required", 403)
		return
	}
	copy := r.Clone(r.Context())
	u := *r.URL
	copy.URL = &u
	copy.URL.Path = "/v1/frame/photos/" + strings.TrimPrefix(r.URL.Path, "/frame/photos/")
	s.frameImage(frameNoStore{w}, copy)
}
