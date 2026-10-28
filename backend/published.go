package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image/jpeg"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const publishedLimit = 10
const publishedSize = 20
const publishedTTL = 2 * time.Hour

type PublishedPhoto struct {
	ID     string `json:"id"`
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Taken  string `json:"taken"`
}
type PublishedBatch struct {
	ID         string           `json:"batch_id"`
	Published  int64            `json:"published_at"`
	Expires    int64            `json:"expires_at"`
	Refresh    int              `json:"refresh_after_seconds"`
	ValidUntil int64            `json:"valid_until"`
	Photos     []PublishedPhoto `json:"photos"`
}
type publishedSource struct {
	Size     int64          `json:"size"`
	Modified int64          `json:"modified"`
	Photo    PublishedPhoto `json:"photo"`
}
type publishedIndex struct {
	Batches   []PublishedBatch           `json:"batches"`
	Sources   map[string]publishedSource `json:"sources"`
	Signature string                     `json:"signature"`
}
type PublishedStore struct {
	mu            sync.RWMutex
	build         sync.Mutex
	dir, workflow string
	maxAgeYears   int
	index         publishedIndex
}

func newPublishedStore(dir, workflow string) (*PublishedStore, error) {
	if workflow == "" || len(workflow) > 80 {
		return nil, fmt.Errorf("explicit published workflow required")
	}
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0700); err != nil {
		return nil, err
	}
	p := &PublishedStore{dir: dir, workflow: workflow, maxAgeYears: 10, index: publishedIndex{Sources: map[string]publishedSource{}}}
	data, err := os.ReadFile(filepath.Join(dir, "index.json"))
	if err == nil {
		if err = json.Unmarshal(data, &p.index); err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if len(p.index.Batches) > publishedLimit || len(p.index.Sources) > 200 {
		return nil, fmt.Errorf("published index exceeds limits")
	}
	if p.index.Sources == nil {
		p.index.Sources = map[string]publishedSource{}
	}
	for _, b := range p.index.Batches {
		if !idPattern.MatchString(b.ID) || len(b.Photos) == 0 || len(b.Photos) > publishedSize {
			return nil, fmt.Errorf("invalid published batch")
		}
		for _, a := range b.Photos {
			if !validPublishedPhoto(a) {
				return nil, fmt.Errorf("invalid published asset")
			}
		}
	}
	return p, nil
}
func validPublishedPhoto(a PublishedPhoto) bool {
	_, err := hex.DecodeString(a.SHA256)
	return err == nil && len(a.SHA256) == 64 && a.ID == a.SHA256[:32] && a.URL == "/v2/wallpapers/assets/"+a.SHA256+".jpg" && a.Width == 1920 && a.Height == 1080 && a.Bytes > 0 && a.Bytes <= 3<<20
}
func (p *PublishedStore) assetPath(hash string) string {
	return filepath.Join(p.dir, "assets", hash+".jpg")
}
func (p *PublishedStore) complete(b PublishedBatch) bool {
	for _, a := range b.Photos {
		if !recentTaken(a.Taken, time.Now(), p.maxAgeYears) {
			return false
		}
		st, err := os.Stat(p.assetPath(a.SHA256))
		if err != nil || st.Size() != a.Bytes {
			return false
		}
	}
	return len(b.Photos) > 0
}
func (p *PublishedStore) latest() (PublishedBatch, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	for i := len(p.index.Batches) - 1; i >= 0; i-- {
		if p.complete(p.index.Batches[i]) {
			return p.index.Batches[i], true
		}
	}
	return PublishedBatch{}, false
}
func (p *PublishedStore) asset(id string) (PublishedPhoto, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	for i := len(p.index.Batches) - 1; i >= 0; i-- {
		for _, a := range p.index.Batches[i].Photos {
			if (a.SHA256 == id || a.ID == id) && recentTaken(a.Taken, time.Now(), p.maxAgeYears) {
				return a, true
			}
		}
	}
	return PublishedPhoto{}, false
}
func atomicPublishedJSON(path string, value interface{}) error {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return atomicPublishedBytes(path, b)
}
func atomicPublishedBytes(path string, b []byte) error {
	f, err := os.OpenFile(path+".tmp", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	if _, err = f.Write(b); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(path+".tmp", path)
}

// A soft expiry never destroys the last completed batch. Only bounded ready files
// are inspected, and unchanged sources are not read or hashed again.
func (p *PublishedStore) refresh(q *PaintQueue, now time.Time) error {
	p.build.Lock()
	defer p.build.Unlock()
	q.mu.Lock()
	if !q.Serve {
		q.mu.Unlock()
		return nil
	}
	jobs := []PaintJob{}
	taken := map[string]string{}
	for _, j := range q.Jobs {
		if j.Status == "approved" && j.Workflow == p.workflow && recentTaken(q.Sources[j.PhotoID].Meta.Taken, now, p.maxAgeYears) {
			taken[j.ID] = q.Sources[j.PhotoID].Meta.Taken
			jobs = append(jobs, j)
		}
	}
	q.mu.Unlock()
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].ID < jobs[j].ID })
	p.mu.RLock()
	old := p.index
	p.mu.RUnlock()
	next := publishedIndex{Batches: append([]PublishedBatch{}, old.Batches...), Sources: map[string]publishedSource{}}
	ready := map[string]PublishedPhoto{}
	signature := sha256.New()
	for _, j := range jobs {
		path := filepath.Join(q.dir, j.ID+".jpg")
		st, err := os.Stat(path)
		if err != nil || st.Size() <= 0 || st.Size() > 3<<20 {
			continue
		}
		cached, ok := old.Sources[j.ID]
		if !ok || cached.Size != st.Size() || cached.Modified != st.ModTime().UnixNano() || !validPublishedPhoto(cached.Photo) {
			data, err := os.ReadFile(path)
			if err != nil || len(data) > 3<<20 {
				continue
			}
			dimensions, err := jpeg.DecodeConfig(bytes.NewReader(data))
			if err != nil || dimensions.Width != 1920 || dimensions.Height != 1080 {
				continue
			}
			hash := sha256.Sum256(data)
			key := hex.EncodeToString(hash[:])
			asset := PublishedPhoto{ID: key[:32], URL: "/v2/wallpapers/assets/" + key + ".jpg", SHA256: key, Bytes: int64(len(data)), Width: 1920, Height: 1080}
			cached = publishedSource{Size: st.Size(), Modified: st.ModTime().UnixNano(), Photo: asset}
		}

		cached.Photo.Taken = taken[j.ID]
		next.Sources[j.ID] = cached
		ready[j.ID] = cached.Photo
		fmt.Fprintf(signature, "%s:%s:%s\n", j.ID, cached.Photo.SHA256, cached.Photo.Taken)
	}
	if len(ready) == 0 {
		return nil
	}
	next.Signature = fmt.Sprint(p.maxAgeYears) + ":" + p.workflow + ":" + hex.EncodeToString(signature.Sum(nil))
	latest, has := p.latest()
	if has && now.Unix() < latest.Published+int64(publishedTTL.Seconds()) {
		return nil
	}
	sort.SliceStable(jobs, func(i, j int) bool { return jobs[i].LastServed < jobs[j].LastServed })
	photos := []PublishedPhoto{}
	seen := map[string]bool{}
	selected := map[string]bool{}
	for _, j := range jobs {
		a, ok := ready[j.ID]
		if !ok || seen[a.ID] {
			continue
		}
		if st, err := os.Stat(p.assetPath(a.SHA256)); err != nil || st.Size() != a.Bytes {
			data, err := os.ReadFile(filepath.Join(q.dir, j.ID+".jpg"))
			if err != nil {
				continue
			}
			sum := sha256.Sum256(data)
			if hex.EncodeToString(sum[:]) != a.SHA256 {
				continue
			}
			if err = atomicPublishedBytes(p.assetPath(a.SHA256), data); err != nil {
				return err
			}
		}
		photos = append(photos, a)
		seen[a.ID] = true
		selected[j.ID] = true
		if len(photos) == publishedSize {
			break
		}
	}
	if len(photos) == 0 {
		return nil
	}
	validUntil := int64(0)
	for _, a := range photos {
		until := captureValidUntil(a.Taken, p.maxAgeYears)
		if validUntil == 0 || until < validUntil {
			validUntil = until
		}
	}
	seed, _ := json.Marshal(photos)
	previousSeed, _ := json.Marshal(latest.Photos)
	if has && latest.ValidUntil == validUntil && bytes.Equal(seed, previousSeed) {
		// No genuinely different completed batch: keep its identity even beyond soft TTL.
		// Cache new duplicate-source fingerprints without inventing a new publication.
		if next.Signature != old.Signature {
			p.mu.Lock()
			err := atomicPublishedJSON(filepath.Join(p.dir, "index.json"), next)
			if err == nil {
				p.index = next
			}
			p.mu.Unlock()
			return err
		}
		return nil
	}
	sum := sha256.Sum256(append(seed, []byte(fmt.Sprint(now.UnixNano()))...))
	batch := PublishedBatch{ID: hex.EncodeToString(sum[:16]), Published: now.Unix(), Expires: now.Add(publishedTTL).Unix(), Refresh: 900, Photos: photos}
	batch.ValidUntil = validUntil
	next.Batches = append(next.Batches, batch)
	if len(next.Batches) > publishedLimit {
		next.Batches = next.Batches[len(next.Batches)-publishedLimit:]
	}
	p.mu.Lock()
	if err := atomicPublishedJSON(filepath.Join(p.dir, "index.json"), next); err != nil {
		p.mu.Unlock()
		return err
	}
	p.index = next
	referenced := map[string]bool{}
	for _, b := range next.Batches {
		for _, a := range b.Photos {
			referenced[a.SHA256+".jpg"] = true
		}
	}
	entries, _ := os.ReadDir(filepath.Join(p.dir, "assets"))
	for _, entry := range entries {
		if !entry.IsDir() && !referenced[entry.Name()] {
			os.Remove(filepath.Join(p.dir, "assets", entry.Name()))
		}
	}
	p.mu.Unlock()
	q.mu.Lock()
	for i := range q.Jobs {
		if selected[q.Jobs[i].ID] {
			q.Jobs[i].LastServed = now.UnixNano()
		}
	}
	err := q.save()
	q.mu.Unlock()
	log.Printf("published batch %s: %d completed assets, %d retained batches", batch.ID, len(photos), len(next.Batches))
	return err
}
func (s *Server) publishingEnabled() bool {
	if s.published == nil || s.outpaint == nil {
		return false
	}
	s.outpaint.mu.Lock()
	defer s.outpaint.mu.Unlock()
	return s.outpaint.Serve
}
func (s *Server) publishedLatest(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "GET required", 405)
		return
	}
	if !s.publishingEnabled() {
		http.Error(w, "no published batch available; keep local cache", 503)
		return
	}
	b, ok := s.published.latest()
	if !ok {
		http.Error(w, "no published batch available; keep local cache", 503)
		return
	}
	tag := `"` + b.ID + `"`
	w.Header().Set("ETag", tag)
	w.Header().Set("Cache-Control", "private, no-cache")
	if r.Header.Get("If-None-Match") == tag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	paintJSON(w, b)
}
func (s *Server) publishedAsset(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" && r.Method != "HEAD" {
		http.Error(w, "GET or HEAD required", 405)
		return
	}
	if !s.publishingEnabled() {
		http.Error(w, "published delivery paused; keep local cache", 503)
		return
	}
	hash := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/v2/wallpapers/assets/"), ".jpg")
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
func (s *Server) servePublishedAsset(w http.ResponseWriter, r *http.Request, a PublishedPhoto) {
	f, err := os.Open(s.published.assetPath(a.SHA256))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || st.Size() != a.Bytes {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("X-Photo-Width", "1920")
	w.Header().Set("X-Photo-Height", "1080")
	w.Header().Set("X-Photo-Variant", "published")
	w.Header().Set("ETag", `"`+a.SHA256+`"`)
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	w.Header().Set("Content-Type", "image/jpeg")
	http.ServeContent(w, r, a.SHA256+".jpg", st.ModTime(), f)
}
func (s *Server) publishedLegacyBatch(w http.ResponseWriter, r *http.Request) {
	if !s.publishingEnabled() {
		http.Error(w, "published delivery paused; keep local cache", 503)
		return
	}
	b, ok := s.published.latest()
	if !ok {
		http.Error(w, "no completed batch; keep local cache", 503)
		return
	}
	type item struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	items := []item{}
	for _, a := range b.Photos {
		items = append(items, item{a.ID, "/v1/photos/" + a.ID})
	}
	paintJSON(w, map[string]interface{}{"batch_id": b.ID, "photos": items, "refresh_after_seconds": 900})
}
func (s *Server) maintainPublished(ctx context.Context) {
	tick := func() {
		if err := s.published.refresh(s.outpaint, time.Now()); err != nil {
			log.Printf("batch publication retained previous version: %v", err)
		}
		s.refillPaintQueue(ctx)
	}
	tick()
	timer := time.NewTicker(time.Minute)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			tick()
		}
	}
}
func (s *Server) refillPaintQueue(parent context.Context) {
	q := s.outpaint
	q.mu.Lock()
	pending := 0
	if !q.Autofill {
		q.mu.Unlock()
		return
	}
	for _, j := range q.Jobs {
		if j.Workflow == s.published.workflow && (j.Status == "pending" || j.Status == "processing" || (j.Status == "failed" && j.Attempts < 3)) {
			pending++
		}
	}
	q.mu.Unlock()
	if pending >= 6 {
		return
	}
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	photos, err := s.sampler.Batch(ctx, 4, nil)
	if err != nil {
		return
	}
	for _, photo := range photos {
		w, h := orientedDimensions(photo)
		if w > h {
			continue
		}
		job, err := q.enqueue(photo, s.published.workflow, "")
		if err == nil && job.Status == "pending" {
			pending++
			if pending >= 6 {
				return
			}
		}
	}
}
