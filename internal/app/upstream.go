package app

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

const (
	labelPrefix   = "pier."
	labelUpstream = "pier.upstream"
	labelRedirect = "pier.redirect"
)

// Upstream is where pier sends a request: Scheme is the backend protocol
// (http, https, or tls for passthrough); an empty Host means the record's
// target.
type Upstream struct {
	Scheme        string
	Host          string
	Port          string
	Insecure      bool // https: do not verify the backend certificate
	ProxyProtocol int  // tls: send a PROXY protocol header (1 or 2), 0 = none
}

var errNoUpstream = errors.New("no upstream for this scheme")

// selectUpstream applies the selection rule for a client that arrived on
// clientScheme ("http" or "https"):
//   - no pier.upstream* labels: http://:80;
//   - otherwise pier.upstream.<clientScheme>, then pier.upstream;
//   - labels present but none matches: errNoUpstream.
func selectUpstream(labels map[string]string, clientScheme string) (Upstream, error) {
	if v, ok := labels[labelUpstream+"."+clientScheme]; ok {
		return parseUpstream(v)
	}
	if v, ok := labels[labelUpstream]; ok {
		return parseUpstream(v)
	}

	for key := range labels {
		if strings.HasPrefix(key, labelUpstream+".") {
			return Upstream{}, errNoUpstream
		}
	}

	return Upstream{Scheme: "http", Port: "80"}, nil
}

// parseUpstream parses an upstream label value: a bare port ("3000") or a URL
// ("http://:3000", "https://:8443?insecure=true").
func parseUpstream(v string) (Upstream, error) {
	v = strings.TrimSpace(v)

	if isDigits(v) {
		if err := checkPort(v); err != nil {
			return Upstream{}, err
		}
		return Upstream{Scheme: "http", Port: v}, nil
	}

	u, err := url.Parse(v)
	if err != nil {
		return Upstream{}, fmt.Errorf("invalid upstream %q: %w", v, err)
	}

	up := Upstream{Scheme: u.Scheme, Host: u.Hostname(), Port: u.Port()}

	switch u.Scheme {
	case "http":
		if up.Port == "" {
			up.Port = "80"
		}
	case "https", "tls":
		if up.Port == "" {
			up.Port = "443"
		}
	default:
		return Upstream{}, fmt.Errorf("invalid upstream %q: unsupported scheme %q", v, u.Scheme)
	}

	if u.User != nil || (u.Path != "" && u.Path != "/") || u.Fragment != "" {
		return Upstream{}, fmt.Errorf("invalid upstream %q: only scheme, host, port and options are allowed", v)
	}
	if err := checkPort(up.Port); err != nil {
		return Upstream{}, fmt.Errorf("invalid upstream %q: %w", v, err)
	}

	for key, values := range u.Query() {
		switch key {
		case "insecure":
			if up.Scheme != "https" {
				return Upstream{}, fmt.Errorf("invalid upstream %q: insecure is only valid for https://", v)
			}
			insecure, err := strconv.ParseBool(values[len(values)-1])
			if err != nil {
				return Upstream{}, fmt.Errorf("invalid upstream %q: insecure must be true or false", v)
			}
			up.Insecure = insecure
		case "proxyprotocol":
			if up.Scheme != "tls" {
				return Upstream{}, fmt.Errorf("invalid upstream %q: proxyprotocol is only valid for tls://", v)
			}
			switch values[len(values)-1] {
			case "1":
				up.ProxyProtocol = 1
			case "2":
				up.ProxyProtocol = 2
			default:
				return Upstream{}, fmt.Errorf("invalid upstream %q: proxyprotocol must be 1 or 2", v)
			}
		default:
			return Upstream{}, fmt.Errorf("invalid upstream %q: unknown option %q", v, key)
		}
	}

	return up, nil
}

// validateLabels reports unknown pier.* labels and invalid values.
func validateLabels(labels map[string]string) []error {
	var errs []error

	for key, v := range labels {
		switch key {
		case labelUpstream, labelUpstream + ".http", labelUpstream + ".https":
			if _, err := parseUpstream(v); err != nil {
				errs = append(errs, fmt.Errorf("label %s: %w", key, err))
			}
		case labelRedirect:
			if v != "https" {
				errs = append(errs, fmt.Errorf("label %s: unsupported value %q (only \"https\")", key, v))
			}
		default:
			if strings.HasPrefix(key, labelPrefix) {
				errs = append(errs, fmt.Errorf("unknown label %s", key))
			}
		}
	}

	return errs
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func checkPort(p string) error {
	n, err := strconv.Atoi(p)
	if err != nil || n < 1 || n > 65535 {
		return fmt.Errorf("invalid port %q", p)
	}
	return nil
}
