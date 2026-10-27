package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

type Photo struct {
	ID       string    `json:"id"`
	Path     string    `json:"path"`
	Meta     PhotoMeta `json:"meta"`
	Seen     int64     `json:"seen"`
	Size     int64     `json:"size"`
	Modified int64     `json:"modified"`
}
type History struct {
	Photos      map[string]Photo `json:"photos"`
	Directories map[string]int64 `json:"directories"`
	Rejected    map[string]int64 `json:"rejected"`
}
type dirWindow struct {
	file   *os.File
	dirs   []string
	files  []string
	eof    bool
	opened time.Time
	used   time.Time
	cursor int
}
type Sampler struct {
	mu              sync.Mutex
	root, stateFile string
	history         History
	windows         map[string]*dirWindow
	rng             *rand.Rand
	inspect         func(string) (PhotoMeta, error)
	maxCandidates   int
	reads           int
	budget          time.Duration
}

var yearName = regexp.MustCompile(`^(19|20)[0-9]{2}$`)

func NewSampler(root, state string) (*Sampler, error) {
	r, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	r, err = filepath.Abs(r)
	if err != nil {
		return nil, err
	}
	s := &Sampler{root: r, stateFile: state, history: History{Photos: map[string]Photo{}, Directories: map[string]int64{}, Rejected: map[string]int64{}}, windows: map[string]*dirWindow{}, rng: rand.New(rand.NewSource(time.Now().UnixNano())), inspect: metadata, maxCandidates: 100, budget: 10 * time.Second}
	if data, e := os.ReadFile(state); e == nil {
		if e = json.Unmarshal(data, &s.history); e != nil {
			return nil, e
		}
		if s.history.Photos == nil || s.history.Directories == nil || s.history.Rejected == nil {
			return nil, errors.New("invalid sample history")
		}
	} else if !os.IsNotExist(e) {
		return nil, e
	}
	return s, nil
}
func excluded(name string) bool {
	n := strings.ToLower(name)
	if strings.HasPrefix(n, ".") {
		return true
	}
	for _, part := range []string{"screenshot", "screen_shot", "screen-shot", "截屏", "截图", "屏幕快照", "download", "下载", "表情", "thumbnail", "@eadir"} {
		if strings.Contains(n, part) {
			return true
		}
	}
	return false
}
func (s *Sampler) window(path string) (*dirWindow, error) {
	now := time.Now()
	w := s.windows[path]
	if w != nil && now.Sub(w.opened) > 7*24*time.Hour {
		if w.file != nil {
			w.file.Close()
		}
		delete(s.windows, path)
		w = nil
	}
	if w == nil {
		if len(s.windows) >= 64 {
			var oldest string
			var oldestTime time.Time
			for k, v := range s.windows {
				if oldest == "" || v.used.Before(oldestTime) {
					oldest = k
					oldestTime = v.used
				}
			}
			if s.windows[oldest].file != nil {
				s.windows[oldest].file.Close()
			}
			delete(s.windows, oldest)
		}
		f, e := os.Open(path)
		if e != nil {
			return nil, e
		}
		w = &dirWindow{file: f, opened: now}
		s.windows[path] = w
	}
	w.used = now
	if !w.eof && s.reads < 32 {
		s.reads++
		entries, e := w.file.ReadDir(256)
		if e != nil && e != io.EOF {
			return nil, e
		}
		if e == io.EOF || len(entries) == 0 {
			w.eof = true
			w.file.Close()
			w.file = nil
		}
		for _, entry := range entries {
			if excluded(entry.Name()) || entry.Type()&os.ModeSymlink != 0 {
				continue
			}
			name := entry.Name()
			if entry.IsDir() {
				if path == s.root && !yearName.MatchString(name) {
					continue
				}
				if len(w.dirs) < 256 {
					w.dirs = append(w.dirs, name)
				}
			} else if strings.EqualFold(filepath.Ext(name), ".jpg") || strings.EqualFold(filepath.Ext(name), ".jpeg") {
				if len(w.files) < 2048 {
					w.files = append(w.files, name)
				} else {
					w.files[w.cursor%2048] = name
					w.cursor++
				}
			}
		}
	}
	return w, nil
}
func (s *Sampler) chooseDir(parent string, dirs []string) string {
	if len(dirs) == 1 {
		return filepath.Join(parent, dirs[0])
	}
	a, b := s.rng.Intn(len(dirs)), s.rng.Intn(len(dirs))
	pa, pb := filepath.Join(parent, dirs[a]), filepath.Join(parent, dirs[b])
	if s.history.Directories[pa] > s.history.Directories[pb] {
		return pb
	}
	return pa
}
func (s *Sampler) candidate() (string, string, error) {
	path := s.root
	for depth := 0; depth < 5; depth++ {
		w, e := s.window(path)
		if e != nil {
			return "", "", e
		}
		if len(w.files) > 0 && (len(w.dirs) == 0 || s.rng.Intn(4) > 0) {
			return filepath.Join(path, w.files[s.rng.Intn(len(w.files))]), path, nil
		}
		if len(w.dirs) == 0 {
			return "", "", errors.New("empty sampled directory")
		}
		path = s.chooseDir(path, w.dirs)
	}
	return "", "", errors.New("directory depth limit")
}
func (s *Sampler) safePath(relative string) (string, error) {
	if relative == "" || filepath.IsAbs(relative) {
		return "", errors.New("invalid relative path")
	}
	p, err := filepath.EvalSymlinks(filepath.Join(s.root, relative))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(s.root, p)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", errors.New("outside photo root")
	}
	return p, nil
}
func photoID(relative string, size, modified int64) string {
	data, _ := json.Marshal([]interface{}{relative, size, modified})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:16])
}
func (s *Sampler) Batch(ctx context.Context, count int, exclude []string) ([]Photo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reads = 0
	if count < 1 || count > 30 {
		return nil, errors.New("count must be 1..30")
	}
	deadline := time.Now().Add(s.budget)
	selected := map[string]bool{}
	for _, id := range exclude {
		selected[id] = true
	}
	perDir := map[string]int{}
	photos := []Photo{}
	checked, visits := 0, 0
	for len(photos) < count && checked < s.maxCandidates && visits < 1000 && time.Now().Before(deadline) {
		if e := ctx.Err(); e != nil {
			return nil, e
		}
		visits++
		path, dir, e := s.candidate()
		if e != nil || perDir[dir] >= 2 {
			continue
		}
		info, e := os.Lstat(path)
		if e != nil || !info.Mode().IsRegular() {
			continue
		}
		rel, e := filepath.Rel(s.root, path)
		if e != nil {
			continue
		}
		id := photoID(rel, info.Size(), info.ModTime().UnixNano())
		if selected[id] {
			continue
		}
		if old, recent := s.history.Photos[id]; recent && time.Since(time.Unix(0, old.Seen)) < 30*24*time.Hour {
			continue
		}
		if t, skip := s.history.Rejected[id]; skip && time.Now().Unix()-t < 7*86400 {
			continue
		}
		checked++
		meta, e := s.inspect(path)
		if e != nil || !eligible(meta) {
			s.history.Rejected[id] = time.Now().Unix()
			continue
		}
		if _, e = s.safePath(rel); e != nil {
			continue
		}
		p := Photo{ID: id, Path: rel, Meta: meta, Seen: time.Now().UnixNano(), Size: info.Size(), Modified: info.ModTime().UnixNano()}
		photos = append(photos, p)
		selected[id] = true
		perDir[dir]++
		s.history.Photos[id] = p
		s.history.Directories[dir] = p.Seen
	}
	s.prune()
	if e := s.save(); e != nil {
		return nil, e
	}
	return photos, nil
}
func (s *Sampler) Photo(id string) (Photo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.history.Photos[id]
	if !ok {
		return Photo{}, os.ErrNotExist
	}
	return p, nil
}
func (s *Sampler) prune() {
	if len(s.history.Photos) > 2000 {
		ps := make([]Photo, 0, len(s.history.Photos))
		for _, p := range s.history.Photos {
			ps = append(ps, p)
		}
		sort.Slice(ps, func(i, j int) bool { return ps[i].Seen > ps[j].Seen })
		for _, p := range ps[2000:] {
			delete(s.history.Photos, p.ID)
		}
	}
	for len(s.history.Rejected) > 4000 {
		for id := range s.history.Rejected {
			delete(s.history.Rejected, id)
			break
		}
	}
	if len(s.history.Directories) > 1000 {
		for d, t := range s.history.Directories {
			if time.Since(time.Unix(0, t)) > 30*24*time.Hour {
				delete(s.history.Directories, d)
			}
		}
	}
}
func (s *Sampler) save() error {
	if e := os.MkdirAll(filepath.Dir(s.stateFile), 0700); e != nil {
		return e
	}
	data, e := json.Marshal(s.history)
	if e != nil {
		return e
	}
	tmp := s.stateFile + ".tmp"
	f, e := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	if _, e = f.Write(data); e == nil {
		e = f.Sync()
	}
	closeErr := f.Close()
	if e != nil {
		return e
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(tmp, s.stateFile)
}
