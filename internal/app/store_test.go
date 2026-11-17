package app

import (
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"
)

// fakeSource returns whatever is set on it; safe for concurrent use.
type fakeSource struct {
	mu      sync.Mutex
	records []Record
	err     error
	calls   int
}

func (f *fakeSource) Fetch() ([]Record, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return f.records, f.err
}

func (f *fakeSource) set(records []Record, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.records, f.err = records, err
}

func (f *fakeSource) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func newTestStore(src Source) *Store {
	s := NewStore("", "")
	s.source = src
	s.location = "fake"
	return s
}

func TestStore_Refresh(t *testing.T) {
	t.Parallel()

	src := &fakeSource{records: []Record{{Host: "a.lh", Target: "10.0.0.1"}, {Host: "b.lh", Target: "10.0.0.2"}}}
	s := newTestStore(src)
	s.Refresh()

	if got := s.Hosts(); !reflect.DeepEqual(got, []string{"a.lh", "b.lh"}) {
		t.Errorf("hosts = %v", got)
	}
	if r, ok := s.get("a.lh"); !ok || r.Target != "10.0.0.1" {
		t.Errorf("a.lh = %+v, %v", r, ok)
	}

	// Removed record disappears.
	src.set([]Record{{Host: "b.lh", Target: "10.0.0.2"}}, nil)
	s.Refresh()
	if _, ok := s.get("a.lh"); ok {
		t.Error("a.lh should be gone")
	}
}

func TestStore_KeepsLastGoodOnError(t *testing.T) {
	t.Parallel()

	src := &fakeSource{records: []Record{{Host: "a.lh", Target: "10.0.0.1"}}}
	s := newTestStore(src)
	s.Refresh()

	src.set(nil, errors.New("boom"))
	s.Refresh()

	if _, ok := s.get("a.lh"); !ok {
		t.Error("a.lh should be kept after a failed fetch")
	}
}

func TestStore_RefreshOnMiss(t *testing.T) {
	t.Parallel()

	src := &fakeSource{}
	s := newTestStore(src)
	s.Refresh()

	// Backend appears after the last poll: the miss triggers a refresh.
	src.set([]Record{{Host: "new.lh", Target: "10.0.0.3"}}, nil)
	if _, ok := s.Lookup("new.lh"); !ok {
		t.Error("new.lh should be found via refresh-on-miss")
	}

}

func TestStore_RefreshOnMiss_RightAfterAnotherMiss(t *testing.T) {
	t.Parallel()

	src := &fakeSource{}
	s := newTestStore(src)

	// An unknown host uses up the refresh budget...
	if _, ok := s.Lookup("typo.lh"); ok {
		t.Fatal("typo.lh should not exist")
	}

	// ...a backend that appears right after must still be found: the miss
	// waits for the interval instead of being skipped.
	src.set([]Record{{Host: "new.lh", Target: "10.0.0.3"}}, nil)
	start := time.Now()
	if _, ok := s.Lookup("new.lh"); !ok {
		t.Error("new.lh should be found after waiting for the miss interval")
	}
	if elapsed := time.Since(start); elapsed > 2*missInterval {
		t.Errorf("waited %s, want at most ~%s", elapsed, missInterval)
	}
}

func TestStore_RefreshOnMiss_Coalesced(t *testing.T) {
	t.Parallel()

	src := &fakeSource{}
	s := newTestStore(src)
	s.Lookup("warmup.lh") // start the interval

	before := src.callCount()

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.Lookup("missing.lh")
		}()
	}
	wg.Wait()

	// Concurrent misses share refreshes: far fewer fetches than misses.
	if calls := src.callCount() - before; calls > 2 {
		t.Errorf("20 concurrent misses caused %d fetches, want at most 2", calls)
	}
}

func TestStore_NoSource(t *testing.T) {
	t.Parallel()

	s := NewStore("", "")
	s.Refresh()
	if _, ok := s.Lookup("a.lh"); ok {
		t.Error("store without source should be empty")
	}
}
