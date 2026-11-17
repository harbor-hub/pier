package app

import (
	"log"
	"reflect"
	"sort"
	"sync"
	"time"
)

// Store holds the records of the source by host. When a fetch fails it keeps
// the last good records.
type Store struct {
	source   Source
	location string

	mu    sync.RWMutex
	hosts map[string]Record

	refreshMu sync.Mutex
	lastErr   string

	missMu      sync.Mutex
	missRefresh time.Time // start of the last refresh-on-miss
}

// missInterval limits refresh-on-miss to one fetch per interval.
const missInterval = time.Second

// NewStore returns a store for the source at location (URL or file); an empty
// location means no records.
func NewStore(location, token string) *Store {
	s := &Store{location: location, hosts: map[string]Record{}}
	if location != "" {
		s.source = NewSource(location, token)
	}

	return s
}

// Run refreshes the store every interval. It never returns.
func (s *Store) Run(interval time.Duration) {
	for range time.Tick(interval) {
		s.Refresh()
	}
}

// Refresh fetches the source and swaps in the records if they changed.
func (s *Store) Refresh() {
	if s.source == nil {
		return
	}

	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()

	records, err := s.source.Fetch()
	if err != nil {
		// Log only when the error changes, not on every poll.
		if msg := err.Error(); msg != s.lastErr {
			log.Printf("[SOURCE] %s: %v (keeping last good records)", s.location, err)
			s.lastErr = msg
		}
		return
	}
	if s.lastErr != "" {
		log.Printf("[SOURCE] %s: recovered", s.location)
		s.lastErr = ""
	}

	hosts := make(map[string]Record, len(records))
	for _, r := range records {
		hosts[r.Host] = r
	}

	s.mu.Lock()
	old := s.hosts
	changed := !reflect.DeepEqual(old, hosts)
	if changed {
		s.hosts = hosts
	}
	s.mu.Unlock()

	if !changed {
		return
	}

	log.Printf("[SOURCE] %d host(s)", len(hosts))
	for host, r := range hosts {
		if prev, ok := old[host]; ok && reflect.DeepEqual(prev, r) {
			continue
		}
		for _, err := range validateLabels(r.Labels) {
			log.Printf("[SOURCE] %s: %v", host, err)
		}
	}
}

// Lookup returns the record for host. On a miss it refreshes the source and
// tries again, so a just-started backend works without waiting for the next
// poll. Refreshes on miss happen at most once per missInterval: a miss inside
// the interval waits for it to end, unless a refresh started after the miss
// arrived (its result is then fresh enough).
func (s *Store) Lookup(host string) (Record, bool) {
	if r, ok := s.get(host); ok {
		return r, true
	}

	arrived := time.Now()

	s.missMu.Lock()
	if !s.missRefresh.After(arrived) {
		if wait := missInterval - time.Since(s.missRefresh); wait > 0 {
			time.Sleep(wait)
		}
		s.missRefresh = time.Now()
		s.Refresh()
	}
	s.missMu.Unlock()

	return s.get(host)
}

// Hosts returns all known hostnames, sorted.
func (s *Store) Hosts() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	hosts := make([]string, 0, len(s.hosts))
	for host := range s.hosts {
		hosts = append(hosts, host)
	}
	sort.Strings(hosts)

	return hosts
}

func (s *Store) get(host string) (Record, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	r, ok := s.hosts[host]

	return r, ok
}
