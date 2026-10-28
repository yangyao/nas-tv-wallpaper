package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPoolMigrationKeepsPlaybackButWaitsForTwentyIndependentPhotos(t *testing.T) {
	q, _ := newPaintQueue(t.TempDir(), true, true)
	p, _ := newPublishedStore(t.TempDir(), "light")
	now := time.Now()
	for n := 1; n <= 3; n++ {
		addPublishedTestJob(t, q, n, "light", "approved")
	}
	if err := p.refresh(q, now); err != nil {
		t.Fatal(err)
	}
	legacy, _ := p.latest()
	if err := p.enablePool(); err != nil {
		t.Fatal(err)
	}
	if b, ok := p.latest(); !ok || b.ID != legacy.ID || !b.Bootstrap {
		t.Fatal("migration lost playback")
	}
	if err := p.refresh(q, now); err != nil {
		t.Fatal(err)
	}
	if len(p.index.Batches) != 0 {
		t.Fatal("partial legacy snapshot counted as prepared pool")
	}
	if ready, _ := p.poolProgress(now); ready != 3 {
		t.Fatal("incorrect preparation progress", ready)
	}
	for n := 4; n <= 20; n++ {
		addPublishedTestJob(t, q, n, "light", "approved")
	}
	if err := p.refresh(q, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	first, _ := p.latest()
	if len(p.index.Batches) != 1 || len(first.Photos) != 20 || first.Bootstrap {
		t.Fatal("first batch not complete")
	}
	if !p.poolHasSource(fmt.Sprintf("%032x", 1)) {
		t.Fatal("active source is not excluded from resampling")
	}
	addPublishedTestJob(t, q, 21, "light", "approved")
	if err := p.refresh(q, now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if len(p.index.Batches) != 1 {
		t.Fatal("partial second batch was published")
	}
	reloaded, err := newPublishedStore(p.dir, "light")
	if err != nil || !reloaded.pool {
		t.Fatal("pool state not durable", err)
	}
	if ready, _ := reloaded.poolProgress(now); ready != 1 {
		t.Fatal("staging lost on restart", ready)
	}
	for n := 22; n <= 40; n++ {
		addPublishedTestJob(t, q, n, "light", "approved")
	}
	if err := reloaded.refresh(q, now.Add(3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if len(reloaded.index.Batches) != 2 {
		t.Fatal("bootstrap waited for replacement interval")
	}
	seen := map[string]bool{}
	for _, b := range reloaded.index.Batches {
		for _, a := range b.Photos {
			if seen[a.SHA256] {
				t.Fatal("pool batches share a photo")
			}
			seen[a.SHA256] = true
		}
	}
	if b, _ := reloaded.latest(); b.ID != first.ID {
		t.Fatal("preparing a reserve batch changed TV selection too soon")
	}
	if err := reloaded.refresh(q, now.Add(publishedTTL+time.Minute)); err != nil {
		t.Fatal(err)
	}
	if b, _ := reloaded.latest(); b.ID == first.ID {
		t.Fatal("TV did not rotate prepared batch")
	}
}

func TestFullPoolKeepsTenUntilReplacementIsCompleteAndCommitted(t *testing.T) {
	q, _ := newPaintQueue(t.TempDir(), true, true)
	q.maxAgeYears = 10
	p, _ := newPublishedStore(t.TempDir(), "light")
	p.enablePool()
	q.keepApproved = func(id string) bool { item, ok := p.index.Sources[id]; return !ok || !item.Used }
	now := time.Now()
	for batch := 0; batch < 10; batch++ {
		for j := 1; j <= 20; j++ {
			addPublishedTestJob(t, q, batch*20+j, "light", "approved")
		}
		if err := p.refresh(q, now.Add(time.Duration(batch)*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	if len(p.index.Batches) != 10 {
		t.Fatal("pool not filled")
	}
	oldest := p.index.Batches[0].ID
	if p.poolNeedsWork(now.Add(10 * time.Minute)) {
		t.Fatal("full pool still requests generation before interval")
	}
	// Corrupt a candidate to prove that nineteen valid results cannot retire a batch.
	for j := 201; j <= 220; j++ {
		addPublishedTestJob(t, q, j, "light", "approved")
	}
	latestJob := q.Jobs[len(q.Jobs)-1]
	path := filepath.Join(q.dir, latestJob.ID+".jpg")
	saved, _ := os.ReadFile(path)
	os.WriteFile(path, []byte("broken"), 0600)
	due := now.Add(9*time.Minute + publishedTTL)
	if err := p.refresh(q, due); err != nil {
		t.Fatal(err)
	}
	if len(p.index.Batches) != 10 || p.index.Batches[0].ID != oldest {
		t.Fatal("incomplete batch retired old content")
	}
	os.WriteFile(path, saved, 0600)
	// A failed atomic index write must retain all ten previous batches.
	os.Mkdir(filepath.Join(p.dir, "index.json.tmp"), 0700)
	if err := p.refresh(q, due); err == nil {
		t.Fatal("expected publication failure")
	}
	if len(p.index.Batches) != 10 || p.index.Batches[0].ID != oldest {
		t.Fatal("failed publication shrank pool")
	}
	os.Remove(filepath.Join(p.dir, "index.json.tmp"))
	if err := p.refresh(q, due); err != nil {
		t.Fatal(err)
	}
	if len(p.index.Batches) != 10 || p.index.Batches[0].ID == oldest {
		t.Fatal("complete replacement did not evict exactly oldest")
	}
	seen := map[string]bool{}
	for _, b := range p.index.Batches {
		if !p.complete(b) || len(b.Photos) != 20 {
			t.Fatal("incomplete pool batch")
		}
		for _, a := range b.Photos {
			if seen[a.SHA256] {
				t.Fatal("duplicate pool image")
			}
			seen[a.SHA256] = true
		}
	}
	before := p.index.Batches[0].ID
	if err := p.refresh(q, due.Add(publishedTTL)); err != nil {
		t.Fatal(err)
	}
	if p.index.Batches[0].ID != before || len(p.index.Batches) != 10 {
		t.Fatal("expiry without new results retired a batch")
	}
}

func TestPoolGenerationClaimPausesWithoutDroppingPendingJobs(t *testing.T) {
	q, _ := newPaintQueue(t.TempDir(), true, true)
	addPublishedTestJob(t, q, 1, "light", "pending")
	q.canProduce = func() bool { return false }
	s := &Server{outpaint: q, token: "test-token"}
	if w := publishedCall(s, "POST", "/v1/outpaint/claim", ""); w.Code != 204 {
		t.Fatal(w.Code)
	}
	if q.Jobs[0].Status != "pending" {
		t.Fatal("pause changed pending job")
	}
}
