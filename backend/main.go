package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
)

type Server struct {
	sampler              *Sampler
	token, cache         string
	batchGate, imageGate chan struct{}
}

func (s *Server) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"ok":true}`)
	})
	mux.HandleFunc("/v1/batch", s.batch)
	mux.HandleFunc("/v1/photos/", s.image)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.URL.Path != "/health" {
			got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if subtle.ConstantTimeCompare([]byte(got), []byte(s.token)) != 1 {
				http.Error(w, "unauthorized", 401)
				return
			}
		}
		mux.ServeHTTP(w, r)
	})
}
func (s *Server) batch(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST required", 405)
		return
	}
	select {
	case s.batchGate <- struct{}{}:
		defer func() { <-s.batchGate }()
	default:
		http.Error(w, "sampler busy", 429)
		return
	}
	var req struct {
		Count   int      `json:"count"`
		Exclude []string `json:"exclude"`
	}
	if e := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128<<10)).Decode(&req); e != nil || len(req.Exclude) > 2000 {
		http.Error(w, "invalid request", 400)
		return
	}
	if req.Count == 0 {
		req.Count = 20
	}
	if req.Count < 1 || req.Count > 30 {
		http.Error(w, "count must be 1..30", 400)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 11*time.Second)
	defer cancel()
	photos, e := s.sampler.Batch(ctx, req.Count, req.Exclude)
	if e != nil {
		log.Printf("batch failed: %v", e)
		http.Error(w, "sampling unavailable", 503)
		return
	}
	type item struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	items := []item{}
	for _, p := range photos {
		items = append(items, item{p.ID, "/v1/photos/" + p.ID})
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(map[string]interface{}{"photos": items, "refresh_after_seconds": 21600})
	log.Printf("batch: %d qualified photos, bounded random sampling", len(items))
}

var idPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

func (s *Server) image(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "GET required", 405)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/v1/photos/")
	if !idPattern.MatchString(id) {
		http.NotFound(w, r)
		return
	}
	p, e := s.sampler.Photo(id)
	if e != nil {
		http.NotFound(w, r)
		return
	}
	select {
	case s.imageGate <- struct{}{}:
		defer func() { <-s.imageGate }()
	case <-r.Context().Done():
		return
	}
	path, e := s.sampler.imageFile(r.Context(), p, s.cache)
	if e != nil {
		log.Printf("image %s unavailable", id)
		http.Error(w, "image unavailable", 503)
		return
	}
	data, e := os.ReadFile(path)
	if e != nil {
		http.Error(w, "image unavailable", 503)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Header().Set("Content-Length", fmt.Sprint(len(data)))
	w.Write(data)
}
func main() {
	root := flag.String("root", "", "photo root (read-only)")
	data := flag.String("data", "./data", "private history/cache directory")
	listen := flag.String("listen", "127.0.0.1:8791", "private LAN listen address")
	flag.Parse()
	token := os.Getenv("NAS_PHOTO_TOKEN")
	if len(token) < 24 || *root == "" {
		log.Fatal("set --root and a NAS_PHOTO_TOKEN of at least 24 characters")
	}
	host, _, err := net.SplitHostPort(*listen)
	if err != nil || net.ParseIP(host) == nil || (!net.ParseIP(host).IsPrivate() && !net.ParseIP(host).IsLoopback()) {
		log.Fatal("listen must use a private LAN or loopback IP")
	}
	if e := os.MkdirAll(*data, 0700); e != nil {
		log.Fatal(e)
	}
	sampler, e := NewSampler(*root, filepath.Join(*data, "history.json"))
	if e != nil {
		log.Fatal(e)
	}
	app := &Server{sampler: sampler, token: token, cache: filepath.Join(*data, "images"), batchGate: make(chan struct{}, 1), imageGate: make(chan struct{}, 1)}
	server := &http.Server{Addr: *listen, Handler: app.handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 90 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10}
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-stop
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(ctx)
	}()
	log.Printf("NAS photo service listening on %s; no recursive scan", *listen)
	if e = server.ListenAndServe(); e != nil && e != http.ErrServerClosed {
		log.Fatal(e)
	}
}
