package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"image"
	"image/draw"
	"image/jpeg"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Optional bounded queue. Files live outside originals; only approved results can be served.
type PaintJob struct {
	ID         string  `json:"id"`
	PhotoID    string  `json:"photo_id"`
	Workflow   string  `json:"workflow"`
	Prompt     string  `json:"prompt"`
	Status     string  `json:"status"`
	Worker     string  `json:"worker,omitempty"`
	Lease      int64   `json:"lease_until,omitempty"`
	Attempts   int     `json:"attempts"`
	Updated    int64   `json:"updated"`
	Seconds    float64 `json:"seconds,omitempty"`
	LastServed int64   `json:"last_served,omitempty"`
	ErrorCode  string  `json:"error_code,omitempty"`
}
type PaintQueue struct {
	keepApproved func(string) bool
	canProduce   func() bool
	mu           sync.Mutex
	maxAgeYears  int
	dir          string
	Jobs         []PaintJob       `json:"jobs"`
	Sources      map[string]Photo `json:"sources,omitempty"`
	Autofill     bool             `json:"-"`
	Serve        bool             `json:"-"`
}

func newPaintQueue(dir string, autofill, serve bool) (*PaintQueue, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	q := &PaintQueue{dir: dir, Autofill: autofill, Serve: serve}
	b, e := os.ReadFile(filepath.Join(dir, "jobs.json"))
	if e == nil {
		if e = json.Unmarshal(b, q); e != nil {
			return nil, e
		}
	} else if !os.IsNotExist(e) {
		return nil, e
	}
	if q.Sources == nil {
		q.Sources = map[string]Photo{}
	}
	if len(q.Jobs) > 200 {
		return nil, errors.New("outpaint queue exceeds limit")
	}
	return q, nil
}
func (q *PaintQueue) save() error {
	b, e := json.MarshalIndent(q, "", "  ")
	if e != nil {
		return e
	}
	path := filepath.Join(q.dir, "jobs.json")
	if e = os.WriteFile(path+".tmp", b, 0600); e != nil {
		return e
	}
	return os.Rename(path+".tmp", path)
}
func orientedDimensions(p Photo) (int, int) {
	w, h := p.Meta.Width, p.Meta.Height
	if p.Meta.Orientation >= 5 && p.Meta.Orientation <= 8 {
		w, h = h, w
	}
	return w, h
}
func (q *PaintQueue) enqueue(p Photo, workflow, prompt string) (PaintJob, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.maxAgeYears > 0 && !recentTaken(p.Meta.Taken, time.Now(), q.maxAgeYears) {
		return PaintJob{}, errors.New("photo capture date outside allowed range")
	}
	w, h := orientedDimensions(p)
	if w > h {
		return PaintJob{}, errors.New("only portrait or square sources are queued")
	}
	if workflow == "" || len(workflow) > 80 || len(prompt) > 2000 {
		return PaintJob{}, errors.New("invalid workflow or prompt")
	}
	sum := sha256.Sum256([]byte(p.ID + "\n" + workflow + "\n" + prompt))
	id := hex.EncodeToString(sum[:16])
	for _, j := range q.Jobs {
		if j.ID == id {
			if _, ok := q.Sources[p.ID]; !ok {
				q.Sources[p.ID] = p
				return j, q.save()
			}
			return j, nil
		}
	}
	var removed *PaintJob
	if len(q.Jobs) >= 200 {
		oldest := -1
		if q.Autofill {
			for i, candidate := range q.Jobs {
				if candidate.Status != "approved" && candidate.Status != "rejected" {
					continue
				}
				if candidate.Status == "approved" && q.keepApproved != nil && recentTaken(q.Sources[candidate.PhotoID].Meta.Taken, time.Now(), q.maxAgeYears) && q.keepApproved(candidate.ID) {
					continue
				}
				if oldest < 0 || candidate.Updated < q.Jobs[oldest].Updated {
					oldest = i
				}
			}
		}
		if oldest < 0 {
			return PaintJob{}, errors.New("queue is full; review/archive existing jobs")
		}
		victim := q.Jobs[oldest]
		removed = &victim
		q.Jobs = append(q.Jobs[:oldest], q.Jobs[oldest+1:]...)
		stillUsed := victim.PhotoID == p.ID
		for _, remaining := range q.Jobs {
			stillUsed = stillUsed || remaining.PhotoID == victim.PhotoID
		}
		if !stillUsed {
			delete(q.Sources, victim.PhotoID)
		}
	}
	j := PaintJob{ID: id, PhotoID: p.ID, Workflow: workflow, Prompt: prompt, Status: "pending", Updated: time.Now().Unix()}
	q.Sources[p.ID] = p
	q.Jobs = append(q.Jobs, j)
	err := q.save()
	if err == nil && removed != nil {
		os.Remove(filepath.Join(q.dir, removed.ID+".jpg"))
	}
	return j, err
}
func (q *PaintQueue) find(id string) (PaintJob, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, j := range q.Jobs {
		if j.ID == id {
			return j, true
		}
	}
	return PaintJob{}, false
}
func paintJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(v)
}
func (s *Server) paint(w http.ResponseWriter, r *http.Request) {
	q := s.outpaint
	if q == nil {
		http.Error(w, "outpaint disabled", 404)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/v1/outpaint/")
	if path == "jobs" && r.Method == "GET" {
		q.mu.Lock()
		defer q.mu.Unlock()
		paintJSON(w, map[string]interface{}{"jobs": q.Jobs, "serve": q.Serve, "autofill": q.Autofill})
		return
	}
	if path == "jobs" && r.Method == "POST" {
		var req struct {
			PhotoID  string `json:"photo_id"`
			Workflow string `json:"workflow"`
			Prompt   string `json:"prompt"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req) != nil {
			http.Error(w, "invalid job", 400)
			return
		}
		p, e := s.sampler.Photo(req.PhotoID)
		if e != nil {
			http.NotFound(w, r)
			return
		}
		j, e := q.enqueue(p, req.Workflow, req.Prompt)
		if e != nil {
			http.Error(w, e.Error(), 400)
			return
		}
		paintJSON(w, j)
		return
	}
	if path == "claim" && r.Method == "POST" {
		if q.canProduce != nil && !q.canProduce() {
			w.WriteHeader(204)
			return
		}
		var req struct {
			Worker   string `json:"worker"`
			Workflow string `json:"workflow"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048)).Decode(&req) != nil || len(req.Worker) < 1 || len(req.Worker) > 80 || len(req.Workflow) < 1 || len(req.Workflow) > 80 {
			http.Error(w, "invalid worker", 400)
			return
		}
		q.mu.Lock()
		now := time.Now().Unix()
		ageChanged := false
		for i := range q.Jobs {
			j := &q.Jobs[i]
			if q.maxAgeYears > 0 && (j.Status == "pending" || j.Status == "failed") && !recentTaken(q.Sources[j.PhotoID].Meta.Taken, time.Now(), q.maxAgeYears) {
				j.Status = "rejected"
				j.ErrorCode = "capture_date_out_of_range"
				j.Updated = now
				ageChanged = true
				continue
			}
			if j.Status == "processing" && j.Lease < now && j.Attempts >= 3 {
				j.Status = "failed"
				j.Updated = now
			}
			if j.Workflow == req.Workflow && (j.Status == "pending" || (j.Status == "failed" && now-j.Updated >= 3600) || (j.Status == "processing" && j.Lease < now)) && j.Attempts < 3 {
				j.Status = "processing"
				j.Worker = req.Worker
				j.Lease = now + 1800
				j.Attempts++
				j.Updated = now
				if e := q.save(); e != nil {
					q.mu.Unlock()
					http.Error(w, "cannot persist lease", 500)
					return
				}
				job := *j
				q.mu.Unlock()
				paintJSON(w, job)
				return
			}
		}
		if ageChanged {
			if err := q.save(); err != nil {
				q.mu.Unlock()
				http.Error(w, "cannot persist capture-date filtering", 500)
				return
			}
		}
		full := len(q.Jobs) >= 200
		if full && q.Autofill {
			for _, candidate := range q.Jobs {
				if candidate.Status == "approved" || candidate.Status == "rejected" {
					full = false
					break
				}
			}
		}
		q.mu.Unlock()
		if q.Autofill && !full {
			// Bounded sampler, never a recursive scan; skip already queued source/workflow pairs.
			photos, e := s.sampler.Batch(r.Context(), 4, nil)
			if e == nil {
				for _, p := range photos {
					pw, ph := orientedDimensions(p)
					if pw <= ph {
						if j, e := q.enqueue(p, req.Workflow, ""); e == nil && j.Status == "pending" {
							q.mu.Lock()
							for i := range q.Jobs {
								current := &q.Jobs[i]
								if current.ID == j.ID && current.Status == "pending" {
									current.Status = "processing"
									current.Worker = req.Worker
									current.Lease = time.Now().Unix() + 1800
									current.Attempts++
									current.Updated = time.Now().Unix()
									err := q.save()
									leased := *current
									q.mu.Unlock()
									if err != nil {
										http.Error(w, "cannot persist lease", 500)
										return
									}
									paintJSON(w, leased)
									return
								}
							}
							q.mu.Unlock()
						}
					}
				}
			}
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	parts := strings.Split(path, "/")
	if len(parts) != 3 || parts[0] != "jobs" || !idPattern.MatchString(parts[1]) {
		http.NotFound(w, r)
		return
	}
	id, action := parts[1], parts[2]
	job, ok := q.find(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if action == "input" && r.Method == "GET" {
		s.paintInput(w, r, job)
		return
	}
	if action == "result" && r.Method == "GET" {
		servePaintFile(w, r, filepath.Join(q.dir, id+".jpg"))
		return
	}
	if action == "result" && r.Method == "PUT" {
		if job.Status != "processing" || job.Worker != r.Header.Get("X-Worker-ID") || job.Lease < time.Now().Unix() {
			http.Error(w, "lease not held", 409)
			return
		}
		data, e := readPaintBody(w, r)
		if e != nil {
			http.Error(w, "invalid result image", 400)
			return
		}
		q.mu.Lock()
		defer q.mu.Unlock()
		for i := range q.Jobs {
			j := &q.Jobs[i]
			if j.ID != id {
				continue
			}
			if j.Status != "processing" || j.Worker != job.Worker || j.Lease < time.Now().Unix() {
				http.Error(w, "lease changed", 409)
				return
			}
			f := filepath.Join(q.dir, id+".jpg")
			if os.WriteFile(f+".tmp", data, 0600) != nil || os.Rename(f+".tmp", f) != nil {
				http.Error(w, "cannot save result", 500)
				return
			}
			j.Status = "review"
			j.Updated = time.Now().Unix()
			j.Lease = 0
			if e = q.save(); e != nil {
				http.Error(w, "cannot persist result", 500)
				return
			}
			paintJSON(w, j)
			return
		}
	}
	if action == "decision" && r.Method == "POST" {
		var req struct {
			Status string `json:"status"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&req) != nil || (req.Status != "approved" && req.Status != "rejected") {
			http.Error(w, "invalid decision", 400)
			return
		}
		q.mu.Lock()
		defer q.mu.Unlock()
		for i := range q.Jobs {
			j := &q.Jobs[i]
			if j.ID == id {
				if j.Status != "review" && j.Status != "approved" && j.Status != "rejected" && !(req.Status == "rejected" && (j.Status == "pending" || j.Status == "failed")) {
					http.Error(w, "result not ready", 409)
					return
				}
				j.Status = req.Status
				j.Updated = time.Now().Unix()
				if q.save() != nil {
					http.Error(w, "cannot save decision", 500)
					return
				}
				paintJSON(w, j)
				return
			}
		}
	}
	if action == "resume" && r.Method == "POST" {
		q.mu.Lock()
		defer q.mu.Unlock()
		for i := range q.Jobs {
			j := &q.Jobs[i]
			if j.ID == id && j.Worker == r.Header.Get("X-Worker-ID") && (j.Status == "failed" || j.Status == "processing") && j.Attempts < 3 {
				if j.Status == "failed" {
					j.Attempts++
				}
				j.Status = "processing"
				j.Lease = time.Now().Unix() + 1800
				j.Updated = time.Now().Unix()
				if q.save() != nil {
					http.Error(w, "cannot persist resume lease", 500)
					return
				}
				paintJSON(w, j)
				return
			}
		}
		http.Error(w, "job cannot be resumed by this worker", 409)
		return
	}
	if action == "fail" && r.Method == "POST" {
		var failure struct {
			Code string `json:"error_code"`
		}
		if r.ContentLength != 0 {
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048)).Decode(&failure); err != nil && err != io.EOF {
				http.Error(w, "invalid failure", 400)
				return
			}
		}
		switch failure.Code {
		case "", "timeout", "network_error", "validation_error", "processing_error", "unwanted_people", "no_source_person":
		default:
			http.Error(w, "invalid failure code", 400)
			return
		}
		q.mu.Lock()
		defer q.mu.Unlock()
		for i := range q.Jobs {
			j := &q.Jobs[i]
			if j.ID == id && j.Status == "processing" && j.Worker == r.Header.Get("X-Worker-ID") {
				j.Status = "failed"
				if failure.Code == "unwanted_people" || failure.Code == "no_source_person" {
					j.Status = "rejected"
				}
				j.ErrorCode = failure.Code
				j.Lease = 0
				j.Updated = time.Now().Unix()
				if q.save() != nil {
					http.Error(w, "cannot save failure", 500)
					return
				}
				paintJSON(w, j)
				return
			}
		}
		http.Error(w, "lease not held", 409)
		return
	}
	http.Error(w, "unsupported operation", 405)
}
func readPaintBody(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	r.Body = http.MaxBytesReader(w, r.Body, 4<<20)
	defer r.Body.Close()
	var b bytes.Buffer
	if _, e := b.ReadFrom(r.Body); e != nil {
		return nil, e
	}
	c, e := jpeg.DecodeConfig(bytes.NewReader(b.Bytes()))
	if e != nil || c.Width != 1920 || c.Height != 1080 {
		return nil, errors.New("expected 1920x1080 JPEG")
	}
	img, e := jpeg.Decode(bytes.NewReader(b.Bytes()))
	if e != nil {
		return nil, e
	}
	var clean bytes.Buffer
	e = jpeg.Encode(&clean, img, &jpeg.Options{Quality: 90})
	return clean.Bytes(), e
}
func (s *Server) paintInput(w http.ResponseWriter, r *http.Request, j PaintJob) {
	p, e := s.selectedPhoto(j.PhotoID)
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
		http.Error(w, "source unavailable", 503)
		return
	}
	f, e := os.Open(path)
	if e != nil {
		http.Error(w, "source unavailable", 503)
		return
	}
	defer f.Close()
	img, e := jpeg.Decode(f)
	if e != nil {
		http.Error(w, "source unavailable", 503)
		return
	}
	pw, ph := orientedDimensions(p)
	b := img.Bounds()
	factor := minPaint(float64(b.Dx())/float64(pw), float64(b.Dy())/float64(ph))
	fw, fh := int(float64(pw)*factor), int(float64(ph)*factor)
	rect := image.Rect((b.Dx()-fw)/2, (b.Dy()-fh)/2, (b.Dx()+fw)/2, (b.Dy()+fh)/2)
	dst := image.NewNRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
	draw.Draw(dst, dst.Bounds(), img, rect.Min, draw.Src)
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, no-store")
	jpeg.Encode(w, dst, &jpeg.Options{Quality: 95})
}
func minPaint(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
func (q *PaintQueue) approved(photoID string) string {
	q.mu.Lock()
	defer q.mu.Unlock()
	if !q.Serve {
		return ""
	}
	for i := len(q.Jobs) - 1; i >= 0; i-- {
		j := q.Jobs[i]
		if j.PhotoID == photoID && j.Status == "approved" {
			p := filepath.Join(q.dir, j.ID+".jpg")
			if st, e := os.Stat(p); e == nil && st.Size() > 0 {
				return p
			}
		}
	}
	return ""
}

func servePaintFile(w http.ResponseWriter, r *http.Request, path string) {
	data, e := os.ReadFile(path)
	if e != nil || len(data) > 4<<20 {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Cache-Control", "private, max-age=86400")
	if r.Method != "HEAD" {
		w.Write(data)
	}
}

func (q *PaintQueue) source(id string) (Photo, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	p, ok := q.Sources[id]
	if !ok {
		return Photo{}, os.ErrNotExist
	}
	return p, nil
}
func (q *PaintQueue) selectApproved(count int, excluded []string) []Photo {
	q.mu.Lock()
	defer q.mu.Unlock()
	if !q.Serve {
		return nil
	}
	exclude := map[string]bool{}
	for _, id := range excluded {
		exclude[id] = true
	}
	indexes := []int{}
	for i, j := range q.Jobs {
		if j.Status == "approved" && !exclude[j.PhotoID] {
			if _, e := os.Stat(filepath.Join(q.dir, j.ID+".jpg")); e == nil {
				indexes = append(indexes, i)
			}
		}
	}
	sort.SliceStable(indexes, func(i, j int) bool { return q.Jobs[indexes[i]].LastServed < q.Jobs[indexes[j]].LastServed })
	result := []Photo{}
	for _, i := range indexes {
		j := &q.Jobs[i]
		if exclude[j.PhotoID] {
			continue
		}
		if p, ok := q.Sources[j.PhotoID]; ok {
			result = append(result, p)
			exclude[j.PhotoID] = true
			j.LastServed = time.Now().UnixNano()
			if len(result) >= count {
				break
			}
		}
	}
	if len(result) > 0 {
		q.save()
	}
	return result
}
func (s *Server) selectedPhoto(id string) (Photo, error) {
	p, e := s.sampler.Photo(id)
	if e != nil && s.outpaint != nil {
		return s.outpaint.source(id)
	}
	return p, e
}

// Bounded migration for queues created before source snapshots were persisted.
func (q *PaintQueue) reconcile(s *Sampler) error {
	for _, j := range q.Jobs {
		if _, ok := q.Sources[j.PhotoID]; !ok {
			if p, e := s.Photo(j.PhotoID); e == nil {
				q.Sources[j.PhotoID] = p
			}
		}
	}
	for i := range q.Jobs {
		j := &q.Jobs[i]
		if q.maxAgeYears > 0 && (j.Status == "pending" || j.Status == "failed") && !recentTaken(q.Sources[j.PhotoID].Meta.Taken, time.Now(), q.maxAgeYears) {
			j.Status = "rejected"
			j.ErrorCode = "capture_date_out_of_range"
			j.Updated = time.Now().Unix()
		}
	}
	return q.save()
}
