package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// The frame stream has its own bounded history and never creates AI jobs,
// scans the archive, renders previews, or reads the curated publication pool.
func (s *Server) frameBatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", 405)
		return
	}
	if s.frames == nil {
		http.Error(w, "unavailable", 503)
		return
	}
	var req struct {
		Count   int      `json:"count"`
		Exclude []string `json:"exclude"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil || len(req.Exclude) > 500 {
		http.Error(w, "invalid request", 400)
		return
	}
	if req.Count == 0 {
		req.Count = 20
	}
	if req.Count < 1 || req.Count > 20 {
		http.Error(w, "invalid count", 400)
		return
	}
	for _, id := range req.Exclude {
		if !idPattern.MatchString(id) {
			http.Error(w, "invalid exclusion", 400)
			return
		}
	}
	select {
	case s.batchGate <- struct{}{}:
		defer func() { <-s.batchGate }()
	default:
		http.Error(w, "busy", 429)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 11*time.Second)
	defer cancel()
	photos, err := s.frames.Batch(ctx, req.Count, req.Exclude)
	// On a small library eventually allow repeats, but not the currently shown
	// photo (the first exclusion). Existing ordering on the TV never reshuffles.
	if err == nil && len(photos) == 0 {
		keep := req.Exclude
		if len(keep) > 1 {
			keep = keep[:1]
		}
		photos, err = s.frames.sample(ctx, req.Count, keep, true)
	}
	if err != nil && len(photos) == 0 {
		http.Error(w, "unavailable", 503)
		return
	}
	type item struct {
		ID          string `json:"id"`
		URL         string `json:"url"`
		Orientation int    `json:"orientation"`
		Width       int    `json:"width"`
		Height      int    `json:"height"`
	}
	items := make([]item, 0, len(photos))
	for _, p := range photos {
		o := p.Meta.Orientation
		if o < 1 || o > 8 {
			o = 1
		}
		items = append(items, item{p.ID, "/v1/frame/photos/" + p.ID, o, p.Meta.Width, p.Meta.Height})
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(map[string]interface{}{"photos": items})
}

func (s *Server) frameImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "GET required", 405)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/v1/frame/photos/")
	if s.frames == nil || !idPattern.MatchString(id) {
		http.NotFound(w, r)
		return
	}
	select {
	case s.imageGate <- struct{}{}:
		defer func() { <-s.imageGate }()
	default:
		http.Error(w, "busy", 429)
		return
	}
	p, err := s.frames.Photo(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	path, err := s.frames.safePath(p.Path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != p.Size || info.ModTime().UnixNano() != p.Modified || info.Size() > 40<<20 {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Header().Set("ETag", fmt.Sprintf("\"%s\"", id))
	w.Header().Set("X-Photo-Orientation", fmt.Sprint(p.Meta.Orientation))
	// Stream the original JPEG; scaling, orientation and thumbnails happen on TV.
	http.ServeContent(w, r, "photo.jpg", info.ModTime(), f)
}
