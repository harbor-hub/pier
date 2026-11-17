package app

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// Source provides the current records. It is called every --source-refresh;
// change detection is done by the caller.
type Source interface {
	Fetch() ([]Record, error)
}

// NewSource returns an HTTP source for http(s):// URLs, a file source otherwise.
func NewSource(location, token string) Source {
	if strings.HasPrefix(location, "http://") || strings.HasPrefix(location, "https://") {
		return &httpSource{
			url:    location,
			token:  token,
			client: &http.Client{Timeout: 5 * time.Second},
		}
	}

	return &fileSource{path: location}
}

// fileSource reads records from a JSON file.
type fileSource struct {
	path string
}

func (s *fileSource) Fetch() ([]Record, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return nil, err
	}

	return parseRecords(data)
}

// errUnauthorized is returned when the source rejects the token.
var errUnauthorized = errors.New("401 Unauthorized: the source rejected --source-token")

// httpSource reads records from an HTTP endpoint.
type httpSource struct {
	url    string
	token  string
	client *http.Client
}

func (s *httpSource) Fetch() ([]Record, error) {
	req, err := http.NewRequest(http.MethodGet, s.url, nil)
	if err != nil {
		return nil, err
	}
	if s.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized:
		return nil, errUnauthorized
	default:
		return nil, fmt.Errorf("unexpected status %s", resp.Status)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, err
	}

	return parseRecords(data)
}
