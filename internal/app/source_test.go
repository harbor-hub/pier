package app

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

const testRecords = `{"records": [{"host": "a.lh", "target": "10.0.0.1"}]}`

func TestNewSource(t *testing.T) {
	t.Parallel()

	if _, ok := NewSource("http://records.example/records", "").(*httpSource); !ok {
		t.Error("http:// should be an HTTP source")
	}
	if _, ok := NewSource("https://records.example/records", "").(*httpSource); !ok {
		t.Error("https:// should be an HTTP source")
	}
	if _, ok := NewSource("/etc/pier/records.json", "").(*fileSource); !ok {
		t.Error("path should be a file source")
	}
}

func TestFileSource(t *testing.T) {
	t.Parallel()

	file := filepath.Join(t.TempDir(), "records.json")
	if err := os.WriteFile(file, []byte(testRecords), 0o644); err != nil {
		t.Fatal(err)
	}

	records, err := NewSource(file, "").Fetch()
	if err != nil || len(records) != 1 || records[0].Host != "a.lh" {
		t.Errorf("got %v, %v", records, err)
	}

	if _, err := NewSource(file+".missing", "").Fetch(); err == nil {
		t.Error("missing file: expected error")
	}
}

// recordsHandler serves testRecords to requests with "Bearer secret".
var recordsHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer secret" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	w.Write([]byte(testRecords))
})

func TestHTTPSource(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(recordsHandler)
	defer srv.Close()

	records, err := NewSource(srv.URL, "secret").Fetch()
	if err != nil || len(records) != 1 || records[0].Target != "10.0.0.1" {
		t.Errorf("got %v, %v", records, err)
	}

	if _, err := NewSource(srv.URL, "wrong").Fetch(); !errors.Is(err, errUnauthorized) {
		t.Errorf("wrong token: got %v, want errUnauthorized", err)
	}
}

func TestHTTPSource_TLS(t *testing.T) {
	t.Parallel()

	srv := httptest.NewTLSServer(recordsHandler)
	defer srv.Close()

	// The test server's certificate is self-signed: the default client rejects it.
	if _, err := NewSource(srv.URL, "secret").Fetch(); err == nil {
		t.Error("self-signed certificate: expected verification error")
	}

	// With a client that trusts the test CA, the fetch works over HTTPS.
	src := NewSource(srv.URL, "secret").(*httpSource)
	src.client = srv.Client()

	records, err := src.Fetch()
	if err != nil || len(records) != 1 || records[0].Host != "a.lh" {
		t.Errorf("got %v, %v", records, err)
	}
}

func TestHTTPSource_BadStatus(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	if _, err := NewSource(srv.URL, "").Fetch(); err == nil {
		t.Error("expected error")
	}
}
