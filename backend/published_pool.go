package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image/jpeg"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Pool migration preserves one usable legacy batch while independent full
// batches are prepared. Legacy snapshots never count towards the target ten.
func (p *PublishedStore) enablePool() error {
	p.build.Lock()
	defer p.build.Unlock()
	if p.pool {
		return nil
	}
	bridge, ok := p.latest()
	p.mu.Lock()
	defer p.mu.Unlock()
	next := p.index
	next.PoolVersion = 1
	next.Batches = nil
	next.CurrentID = ""
	next.SelectedAt = 0
	next.PreparedAt = 0
	if ok {
		bridge.Bootstrap = true
		next.Bridge = &bridge
	}
	if err := atomicPublishedJSON(filepath.Join(p.dir, "index.json"), next); err != nil {
		return err
	}
	p.index = next
	p.pool = true
	return nil
}

func (p *PublishedStore) poolNeedsWork(now time.Time) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	valid := 0
	for _, b := range p.index.Batches {
		if p.complete(b) {
			valid++
		}
	}
	return valid < publishedLimit || now.Unix() >= p.index.PreparedAt+int64(publishedTTL.Seconds())
}

func (p *PublishedStore) poolHasSource(id string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	for _, b := range p.index.Batches {
		for _, a := range b.Photos {
			if a.SourceID == id {
				return true
			}
		}
	}
	return false
}

func (p *PublishedStore) poolProgress(now time.Time) (int, int64) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	reserved := map[string]bool{}
	valid := 0
	for _, b := range p.index.Batches {
		if p.complete(b) {
			valid++
		}
		for _, a := range b.Photos {
			reserved[a.SHA256] = true
		}
	}
	ready := 0
	seen := map[string]bool{}
	for _, s := range p.index.Sources {
		if !s.Used && !reserved[s.Photo.SHA256] && !seen[s.Photo.SHA256] && recentTaken(s.Photo.Taken, now, p.maxAgeYears) {
			ready++
			seen[s.Photo.SHA256] = true
		}
	}
	due := int64(0)
	if valid >= publishedLimit {
		due = p.index.PreparedAt + int64(publishedTTL.Seconds())
	}
	return ready, due
}

func (p *PublishedStore) refreshPool(q *PaintQueue, now time.Time) error {
	p.build.Lock()
	defer p.build.Unlock()
	q.mu.Lock()
	if !q.Serve {
		q.mu.Unlock()
		return nil
	}
	jobs := []PaintJob{}
	taken := map[string]string{}
	origins := map[string]string{}
	for _, j := range q.Jobs {
		date := q.Sources[j.PhotoID].Meta.Taken
		if j.Status == "approved" && j.Workflow == p.workflow && recentTaken(date, now, p.maxAgeYears) {
			jobs = append(jobs, j)
			taken[j.ID] = date
			origins[j.ID] = j.PhotoID
		}
	}
	q.mu.Unlock()
	sort.Slice(jobs, func(i, j int) bool {
		if jobs[i].Updated == jobs[j].Updated {
			return jobs[i].ID < jobs[j].ID
		}
		return jobs[i].Updated < jobs[j].Updated
	})
	p.mu.RLock()
	old := p.index
	p.mu.RUnlock()
	next := old
	next.Batches = append([]PublishedBatch{}, old.Batches...)
	next.Sources = map[string]publishedSource{}
	reserved := map[string]bool{}
	valid := 0
	for _, b := range old.Batches {
		if p.complete(b) {
			valid++
		}
		for _, a := range b.Photos {
			reserved[a.SHA256] = true
		}
	}
	candidates := []string{}
	seen := map[string]bool{}
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
			sum := sha256.Sum256(data)
			key := hex.EncodeToString(sum[:])
			cached = publishedSource{Used: cached.Used, Size: st.Size(), Modified: st.ModTime().UnixNano(), Photo: PublishedPhoto{ID: key[:32], URL: "/v2/wallpapers/assets/" + key + ".jpg", SHA256: key, Bytes: int64(len(data)), Width: 1920, Height: 1080}}
		}
		cached.Photo.Taken = taken[j.ID]
		cached.Photo.SourceID = origins[j.ID]
		next.Sources[j.ID] = cached
		if !cached.Used && !reserved[cached.Photo.SHA256] && !seen[cached.Photo.SHA256] {
			candidates = append(candidates, j.ID)
			seen[cached.Photo.SHA256] = true
		}
	}
	prepared := false
	due := valid < publishedLimit || now.Unix() >= old.PreparedAt+int64(publishedTTL.Seconds())
	if due && len(candidates) >= publishedSize {
		photos := []PublishedPhoto{}
		validUntil := int64(0)
		for _, id := range candidates[:publishedSize] {
			item := next.Sources[id]
			a := item.Photo
			if st, err := os.Stat(p.assetPath(a.SHA256)); err != nil || st.Size() != a.Bytes {
				data, err := os.ReadFile(filepath.Join(q.dir, id+".jpg"))
				if err != nil {
					return err
				}
				sum := sha256.Sum256(data)
				if int64(len(data)) != a.Bytes || hex.EncodeToString(sum[:]) != a.SHA256 {
					return fmt.Errorf("candidate changed while preparing batch")
				}
				if err = atomicPublishedBytes(p.assetPath(a.SHA256), data); err != nil {
					return err
				}
			}
			photos = append(photos, a)
			until := captureValidUntil(a.Taken, p.maxAgeYears)
			if validUntil == 0 || until < validUntil {
				validUntil = until
			}
			for otherID, other := range next.Sources {
				if other.Photo.SHA256 == a.SHA256 {
					other.Used = true
					next.Sources[otherID] = other
				}
			}
		}
		seed, _ := json.Marshal(photos)
		sum := sha256.Sum256(append(seed, []byte(fmt.Sprint(now.UnixNano()))...))
		batch := PublishedBatch{ID: hex.EncodeToString(sum[:16]), Published: now.Unix(), Expires: now.Add(publishedTTL).Unix(), Refresh: 900, ValidUntil: validUntil, Photos: photos}
		// Prefer replacing an invalid batch; otherwise replace the oldest. Do this
		// only after all twenty new immutable assets have been written successfully.
		if len(next.Batches) >= publishedLimit {
			victim := 0
			for i, b := range next.Batches {
				if !p.complete(b) {
					victim = i
					break
				}
			}
			next.Batches = append(next.Batches[:victim], next.Batches[victim+1:]...)
		}
		next.Batches = append(next.Batches, batch)
		next.PreparedAt = now.Unix()
		prepared = true
	}
	// TV selection rotates independently of producing/replacing pool entries.
	current := -1
	for i, b := range next.Batches {
		if b.ID == next.CurrentID && p.complete(b) {
			current = i
			break
		}
	}
	if current < 0 || now.Unix() >= next.SelectedAt+int64(publishedTTL.Seconds()) {
		for n := 1; n <= len(next.Batches); n++ {
			i := (current + n) % len(next.Batches)
			if p.complete(next.Batches[i]) {
				next.CurrentID = next.Batches[i].ID
				next.SelectedAt = now.Unix()
				break
			}
		}
	}
	before, _ := json.Marshal(old)
	after, _ := json.Marshal(next)
	if bytes.Equal(before, after) {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := atomicPublishedJSON(filepath.Join(p.dir, "index.json"), next); err != nil {
		return err
	}
	p.index = next
	if prepared {
		referenced := map[string]bool{}
		for _, b := range p.allBatches() {
			for _, a := range b.Photos {
				referenced[a.SHA256+".jpg"] = true
			}
		}
		entries, _ := os.ReadDir(filepath.Join(p.dir, "assets"))
		for _, e := range entries {
			if !e.IsDir() && !referenced[e.Name()] {
				os.Remove(filepath.Join(p.dir, "assets", e.Name()))
			}
		}
	}
	return nil
}
