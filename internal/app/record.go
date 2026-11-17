package app

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Record maps a hostname to a backend target (IP or hostname) plus the
// labels pier interprets (pier.upstream*, pier.redirect).
type Record struct {
	Host   string            `json:"host"`
	Target string            `json:"target"`
	Labels map[string]string `json:"labels"`
}

type recordsDoc struct {
	Records []Record `json:"records"`
}

// parseRecords decodes the records format used by all sources:
// {"records": [{"host": ..., "target": ..., "labels": {...}}]}.
func parseRecords(data []byte) ([]Record, error) {
	var doc recordsDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("invalid records JSON: %w", err)
	}

	for i := range doc.Records {
		r := &doc.Records[i]
		r.Host = normalizeHost(r.Host)
		r.Target = strings.TrimSpace(r.Target)

		if r.Host == "" {
			return nil, fmt.Errorf("record %d: empty host", i)
		}
		if r.Target == "" {
			return nil, fmt.Errorf("record %d (%s): empty target", i, r.Host)
		}
	}

	return doc.Records, nil
}

// normalizeHost lowercases a hostname and strips a trailing dot.
func normalizeHost(host string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
}
