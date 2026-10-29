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
	"strconv"
	"strings"
	"syscall"
	"time"
)

type Server struct {
	startedAt            time.Time
	published            *PublishedStore
	outpaint             *PaintQueue
	sampler              *Sampler
	frames               *Sampler
	token, cache         string
	batchGate, imageGate chan struct{}
}

func (s *Server) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"ok":true}`)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		http.Redirect(w, r, "/manage", http.StatusTemporaryRedirect)
	})
	mux.HandleFunc("/manage", s.managementShell)
	mux.HandleFunc("/manage/", s.managementShell)
	mux.HandleFunc("/manage/api/overview", s.managementOverview)
	mux.HandleFunc("/manage/api/images/", s.managementImage)
	mux.HandleFunc("/manage/thumbs/", s.managementThumbnail)
	mux.HandleFunc("/manage/preview/", s.managementPreview)
	mux.HandleFunc("/v1/batch", s.batch)
	mux.HandleFunc("/v1/frame/batch", s.frameBatch)
	mux.HandleFunc("/v1/frame/photos/", s.frameImage)
	mux.HandleFunc("/v2/wallpapers/latest", s.publishedLatest)
	mux.HandleFunc("/v2/wallpapers/assets/", s.publishedAsset)
	mux.HandleFunc("/v1/photos/", s.image)
	mux.HandleFunc("/v1/outpaint/", s.paint)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if (r.URL.Path == "/" || r.URL.Path == "/manage" || strings.HasPrefix(r.URL.Path, "/manage/")) && !managementLAN(r) {
			http.Error(w, "LAN access only", 403)
			return
		}
		if r.URL.Path != "/health" && !managementPublic(r.URL.Path) {
			got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if subtle.ConstantTimeCompare([]byte(got), []byte(s.token)) != 1 && !s.managementReadAllowed(r) {
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
	if s.published != nil {
		s.publishedLegacyBatch(w, r)
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
	photos := []Photo{}
	if s.outpaint != nil {
		photos = s.outpaint.selectApproved(req.Count, req.Exclude)
	}
	var e error
	if len(photos) < req.Count {
		exclude := append([]string{}, req.Exclude...)
		for _, p := range photos {
			exclude = append(exclude, p.ID)
		}
		var originals []Photo
		originals, e = s.sampler.Batch(ctx, req.Count-len(photos), exclude)
		photos = append(photos, originals...)
	}
	if e != nil && len(photos) == 0 {
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
	if r.Method != "GET" && r.Method != "HEAD" {
		http.Error(w, "GET or HEAD required", 405)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/v1/photos/")
	if !idPattern.MatchString(id) {
		http.NotFound(w, r)
		return
	}
	if s.published != nil {
		if !s.publishingEnabled() {
			http.Error(w, "published delivery paused; keep local cache", 503)
			return
		}
		if a, ok := s.published.asset(id); ok {
			s.servePublishedAsset(w, r, a)
		} else {
			http.NotFound(w, r)
		}
		return
	}
	p, e := s.selectedPhoto(id)
	if e != nil {
		http.NotFound(w, r)
		return
	}
	if s.outpaint != nil {
		if result := s.outpaint.approved(id); result != "" {
			original, err := s.sampler.safePath(p.Path)
			if err != nil {
				http.NotFound(w, r)
				return
			}
			info, err := os.Stat(original)
			if err != nil || info.Size() != p.Size || info.ModTime().UnixNano() != p.Modified {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("X-Photo-Width", "1920")
			w.Header().Set("X-Photo-Height", "1080")
			w.Header().Set("X-Photo-Variant", "outpaint")
			w.Header().Set("Cache-Control", "private, max-age=86400")
			servePaintFile(w, r, result)
			return
		}
	}
	width, height := p.Meta.Width, p.Meta.Height
	if p.Meta.Orientation >= 5 && p.Meta.Orientation <= 8 {
		width, height = height, width
	}
	w.Header().Set("X-Photo-Width", fmt.Sprint(width))
	w.Header().Set("X-Photo-Height", fmt.Sprint(height))
	w.Header().Set("Cache-Control", "private, max-age=86400")
	if r.Method == "HEAD" {
		w.WriteHeader(http.StatusOK)
		return // History already has dimensions; no JPEG read or image conversion.
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
	if raw := os.Getenv("NAS_PHOTO_MAX_AGE_YEARS"); raw != "" {
		years, err := strconv.Atoi(raw)
		if err != nil || years < 1 || years > 100 {
			log.Fatal("NAS_PHOTO_MAX_AGE_YEARS must be 1..100")
		}
		sampler.maxAgeYears = years
	}
	app := &Server{startedAt: time.Now(), sampler: sampler, token: token, cache: filepath.Join(*data, "images"), batchGate: make(chan struct{}, 1), imageGate: make(chan struct{}, 1)}
	app.frames, e = NewSampler(*root, filepath.Join(*data, "frame-history.json"))
	if e != nil {
		log.Fatal(e)
	}
	app.frames.originals = true
	if os.Getenv("NAS_PHOTO_OUTPAINT") == "1" {
		app.outpaint, e = newPaintQueue(filepath.Join(*data, "outpaint"), os.Getenv("NAS_PHOTO_OUTPAINT_AUTOFILL") == "1", os.Getenv("NAS_PHOTO_OUTPAINT_SERVE") == "1")
		if e != nil {
			log.Fatal(e)
		}
		app.outpaint.maxAgeYears = sampler.maxAgeYears
		if e = app.outpaint.reconcile(sampler); e != nil {
			log.Fatal(e)
		}
	}
	runContext, stopMaintenance := context.WithCancel(context.Background())
	defer stopMaintenance()
	if os.Getenv("NAS_PHOTO_BATCH_PUBLISH") == "1" {
		if app.outpaint == nil {
			log.Fatal("published batches require outpaint queue")
		}
		app.published, e = newPublishedStore(filepath.Join(*data, "published"), os.Getenv("NAS_PHOTO_PUBLISH_WORKFLOW"))
		if e != nil {
			log.Fatal(e)
		}
		app.published.maxAgeYears = sampler.maxAgeYears
		if os.Getenv("NAS_PHOTO_BATCH_POOL") == "1" {
			if e = app.published.enablePool(); e != nil {
				log.Fatal(e)
			}
		}
		if app.published.pool {
			app.outpaint.keepApproved = func(id string) bool {
				app.published.mu.RLock()
				defer app.published.mu.RUnlock()
				item, ok := app.published.index.Sources[id]
				return !ok || !item.Used
			}
			app.outpaint.canProduce = func() bool { return app.published.poolNeedsWork(time.Now()) }
		}
		go app.maintainPublished(runContext)
	}
	server := &http.Server{Addr: *listen, Handler: app.handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 90 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10}
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-stop
		stopMaintenance()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(ctx)
	}()
	log.Printf("NAS photo service listening on %s; no recursive scan", *listen)
	if e = server.ListenAndServe(); e != nil && e != http.ErrServerClosed {
		log.Fatal(e)
	}
}
