package main

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"image/jpeg"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

//go:embed manage/index.html manage/app.js manage/style.css
var managementFiles embed.FS

func (s *Server) managementShell(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "GET required", 405)
		return
	}
	path := r.URL.Path
	name := ""
	contentType := ""
	switch path {
	case "/manage", "/manage/":
		name = "index.html"
		contentType = "text/html; charset=utf-8"
	case "/manage/app.js":
		name = "app.js"
		contentType = "text/javascript; charset=utf-8"
	case "/manage/style.css":
		name = "style.css"
		contentType = "text/css; charset=utf-8"
	default:
		http.NotFound(w, r)
		return
	}
	data, err := managementFiles.ReadFile("manage/" + name)
	if err != nil {
		http.Error(w, "management files unavailable", 500)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Write(data)
}
func managementPublic(path string) bool {
	return path == "/" || path == "/manage" || path == "/manage/" || path == "/manage/app.js" || path == "/manage/style.css"
}
func managementLAN(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && (ip.IsPrivate() || ip.IsLoopback())
}
func (s *Server) managementReadAllowed(r *http.Request) bool {
	return managementLAN(r) && (r.Method == "GET" || r.Method == "HEAD") && (strings.HasPrefix(r.URL.Path, "/manage/api/") || strings.HasPrefix(r.URL.Path, "/manage/thumbs/"))
}

type managementBatch struct {
	PublishedBatch
	Available bool   `json:"available"`
	Current   bool   `json:"current"`
	Reason    string `json:"reason,omitempty"`
}
type managementJob struct {
	ID            string `json:"id"`
	Status        string `json:"status"`
	CurrentRecipe bool   `json:"current_recipe"`
	Attempts      int    `json:"attempts"`
	Updated       int64  `json:"updated"`
	Lease         int64  `json:"lease_until,omitempty"`
	Taken         string `json:"taken,omitempty"`
	ErrorCode     string `json:"error_code,omitempty"`
	HasResult     bool   `json:"has_result"`
}

func (s *Server) managementOverview(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "GET required", 405)
		return
	}
	now := time.Now()
	years := 10
	workflow := ""
	currentID := ""
	batches := []managementBatch{}
	available := 0
	currentPhotos := 0
	if s.published != nil {
		years = s.published.maxAgeYears
		workflow = s.published.workflow
		if b, ok := s.published.latest(); ok {
			currentID = b.ID
			currentPhotos = len(b.Photos)
		}
		s.published.mu.RLock()
		saved := append([]PublishedBatch{}, s.published.index.Batches...)
		s.published.mu.RUnlock()
		for i := len(saved) - 1; i >= 0; i-- {
			b := saved[i]
			valid := s.published.complete(b)
			reason := ""
			if valid {
				available++
			} else {
				reason = "檔案缺失或拍攝日期不符合條件"
			}
			for _, a := range b.Photos {
				if a.Taken == "" {
					reason = "早期測試記錄，拍攝日期資料不完整"
					break
				}
			}
			batches = append(batches, managementBatch{PublishedBatch: b, Available: valid, Current: b.ID == currentID, Reason: reason})
		}
	}
	jobs := []managementJob{}
	counts := map[string]int{"pending": 0, "processing": 0, "approved": 0, "review": 0, "failed": 0, "rejected": 0}
	serve := false
	autofill := false
	lastActivity := int64(0)
	if s.outpaint != nil {
		q := s.outpaint
		q.mu.Lock()
		serve = q.Serve
		autofill = q.Autofill
		for _, j := range q.Jobs {
			current := j.Workflow == workflow
			source := q.Sources[j.PhotoID]
			_, err := os.Stat(filepath.Join(q.dir, j.ID+".jpg"))
			jobs = append(jobs, managementJob{ID: j.ID, Status: j.Status, CurrentRecipe: current, Attempts: j.Attempts, Updated: j.Updated, Lease: j.Lease, Taken: source.Meta.Taken, ErrorCode: j.ErrorCode, HasResult: err == nil && (j.Status == "approved" || j.Status == "review")})
			if current {
				counts[j.Status]++
				if j.Attempts > 0 && j.Updated > lastActivity {
					lastActivity = j.Updated
				}
			}
		}
		q.mu.Unlock()
	}
	sort.SliceStable(jobs, func(i, j int) bool { return jobs[i].Updated > jobs[j].Updated })
	uptime := int64(0)
	if !s.startedAt.IsZero() {
		uptime = int64(now.Sub(s.startedAt).Seconds())
	}
	paintJSON(w, map[string]interface{}{"server_time": now.Unix(), "uptime_seconds": uptime, "version": "1.4.0", "published_mode": s.published != nil, "serving": serve, "automatic_sampling": autofill, "max_batches": publishedLimit, "batch_target_size": publishedSize, "soft_ttl_seconds": int(publishedTTL.Seconds()), "capture_years": years, "capture_cutoff": now.AddDate(-years, 0, 0).Format("2006-01-02"), "current_batch_id": currentID, "current_photos": currentPhotos, "available_batches": available, "retained_batches": len(batches), "batches": batches, "jobs": jobs, "counts": counts, "worker_last_activity": lastActivity})
}
func (s *Server) managementThumbnail(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" && r.Method != "HEAD" {
		http.Error(w, "GET or HEAD required", 405)
		return
	}
	key := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/manage/thumbs/"), ".jpg")
	path := ""
	cacheKey := ""
	if strings.HasPrefix(key, "asset-") && s.published != nil {
		hash := strings.TrimPrefix(key, "asset-")
		if len(hash) == 64 {
			if a, ok := s.published.asset(hash); ok {
				path = s.published.assetPath(a.SHA256)
				cacheKey = "asset-" + hash
			}
		}
	} else if strings.HasPrefix(key, "job-") && s.outpaint != nil {
		id := strings.TrimPrefix(key, "job-")
		if idPattern.MatchString(id) {
			j, ok := s.outpaint.find(id)
			if ok && (j.Status == "approved" || j.Status == "review") {
				path = filepath.Join(s.outpaint.dir, id+".jpg")
				if st, err := os.Stat(path); err == nil {
					sum := sha256.Sum256([]byte(fmt.Sprint(id, st.Size(), st.ModTime().UnixNano())))
					cacheKey = "job-" + hex.EncodeToString(sum[:])
				}
			}
		}
	}
	if path == "" || cacheKey == "" {
		http.NotFound(w, r)
		return
	}
	cacheDir := filepath.Join(s.cache, "management-thumbnails")
	target := filepath.Join(cacheDir, cacheKey+".jpg")
	if _, err := os.Stat(target); err != nil {
		if s.imageGate != nil {
			select {
			case s.imageGate <- struct{}{}:
				defer func() { <-s.imageGate }()
			case <-r.Context().Done():
				return
			}
		}
		if _, err = os.Stat(target); err != nil {
			f, err := os.Open(path)
			if err != nil {
				http.NotFound(w, r)
				return
			}
			data, err := io.ReadAll(io.LimitReader(f, 4<<20+1))
			f.Close()
			if err != nil || len(data) > 4<<20 {
				http.NotFound(w, r)
				return
			}
			size, err := jpeg.DecodeConfig(bytes.NewReader(data))
			if err != nil || size.Width != 1920 || size.Height != 1080 {
				http.NotFound(w, r)
				return
			}
			img, err := jpeg.Decode(bytes.NewReader(data))
			if err != nil {
				http.NotFound(w, r)
				return
			}
			var thumbnail bytes.Buffer
			if err = jpeg.Encode(&thumbnail, scaled(img, 480, 270), &jpeg.Options{Quality: 78}); err != nil {
				http.Error(w, "preview unavailable", 500)
				return
			}
			if err = os.MkdirAll(cacheDir, 0700); err != nil {
				http.Error(w, "preview unavailable", 500)
				return
			}
			if err = atomicPublishedBytes(target, thumbnail.Bytes()); err != nil {
				http.Error(w, "preview unavailable", 500)
				return
			}
			pruneManagementThumbnails(cacheDir)
		}
	}
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Header().Set("Content-Type", "image/jpeg")
	http.ServeFile(w, r, target)
}
func pruneManagementThumbnails(dir string) {
	pruneImages(dir, 16<<20)
	entries, _ := os.ReadDir(dir)
	if len(entries) <= 400 {
		return
	}
	type item struct {
		name  string
		stamp time.Time
	}
	items := []item{}
	for _, entry := range entries {
		if !entry.IsDir() {
			if st, err := entry.Info(); err == nil {
				items = append(items, item{entry.Name(), st.ModTime()})
			}
		}
	}
	if len(items) <= 400 {
		return
	}
	sort.Slice(items, func(i, j int) bool { return items[i].stamp.Before(items[j].stamp) })
	for _, entry := range items[:len(items)-400] {
		os.Remove(filepath.Join(dir, entry.name))
	}
}

func (s *Server) managementImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" && r.Method != "HEAD" {
		http.Error(w, "GET or HEAD required", 405)
		return
	}
	if s.published == nil {
		http.NotFound(w, r)
		return
	}
	hash := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/manage/api/images/"), ".jpg")
	if len(hash) != 64 {
		http.NotFound(w, r)
		return
	}
	a, ok := s.published.asset(hash)
	if !ok {
		http.NotFound(w, r)
		return
	}
	s.servePublishedAsset(w, r, a)
}
