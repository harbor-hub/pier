package app

import (
	"errors"
	"testing"
)

func TestParseUpstream(t *testing.T) {
	t.Parallel()

	cases := map[string]Upstream{
		"3000":                         {Scheme: "http", Port: "3000"},
		"http://:3000":                 {Scheme: "http", Port: "3000"},
		"http://":                      {Scheme: "http", Port: "80"},
		"https://:8443":                {Scheme: "https", Port: "8443"},
		"https://":                     {Scheme: "https", Port: "443"},
		"https://:8443?insecure=true":  {Scheme: "https", Port: "8443", Insecure: true},
		"https://:8443?insecure=false": {Scheme: "https", Port: "8443"},
		"http://dev.local:5173":        {Scheme: "http", Host: "dev.local", Port: "5173"},
		"http://:3000/":                {Scheme: "http", Port: "3000"},
		"tls://:8443":                  {Scheme: "tls", Port: "8443"},
		"tls://":                       {Scheme: "tls", Port: "443"},
		"tls://:8443?proxyprotocol=1":  {Scheme: "tls", Port: "8443", ProxyProtocol: 1},
		"tls://:8443?proxyprotocol=2":  {Scheme: "tls", Port: "8443", ProxyProtocol: 2},
		"tls://db.local:8443":          {Scheme: "tls", Host: "db.local", Port: "8443"},
	}

	for in, want := range cases {
		got, err := parseUpstream(in)
		if err != nil {
			t.Errorf("parseUpstream(%q): %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("parseUpstream(%q) = %+v, want %+v", in, got, want)
		}
	}
}

func TestParseUpstream_Errors(t *testing.T) {
	t.Parallel()

	for _, in := range []string{
		"",
		"0",
		"70000",
		"ftp://:21",
		"tls://:8443?proxyprotocol=3",
		"tls://:8443?insecure=true",
		"https://:8443?proxyprotocol=2",
		"http://:3000?proxyprotocol=1",
		"http://:3000?insecure=true",
		"https://:8443?insecure=maybe",
		"https://:8443?foo=1",
		"http://:3000/path",
		"http://user@:3000",
		"http://:abc",
	} {
		if _, err := parseUpstream(in); err == nil {
			t.Errorf("parseUpstream(%q): expected error", in)
		}
	}
}

func TestSelectUpstream(t *testing.T) {
	t.Parallel()

	port80 := Upstream{Scheme: "http", Port: "80"}
	port3000 := Upstream{Scheme: "http", Port: "3000"}
	tls8443 := Upstream{Scheme: "https", Port: "8443"}

	cases := []struct {
		name   string
		labels map[string]string
		scheme string
		want   Upstream
		err    error
	}{
		{"no labels http", nil, "http", port80, nil},
		{"no labels https", nil, "https", port80, nil},
		{"other labels only", map[string]string{"pier.redirect": "https"}, "https", port80, nil},
		{"general http", map[string]string{"pier.upstream": "3000"}, "http", port3000, nil},
		{"general https", map[string]string{"pier.upstream": "3000"}, "https", port3000, nil},
		{"exact wins", map[string]string{"pier.upstream": "3000", "pier.upstream.https": "https://:8443"}, "https", tls8443, nil},
		{"exact other scheme falls back", map[string]string{"pier.upstream": "3000", "pier.upstream.https": "https://:8443"}, "http", port3000, nil},
		{"only https on http", map[string]string{"pier.upstream.https": "https://:8443"}, "http", Upstream{}, errNoUpstream},
		{"only http on https", map[string]string{"pier.upstream.http": "3000"}, "https", Upstream{}, errNoUpstream},
	}

	for _, c := range cases {
		got, err := selectUpstream(c.labels, c.scheme)
		if !errors.Is(err, c.err) {
			t.Errorf("%s: err = %v, want %v", c.name, err, c.err)
			continue
		}
		if got != c.want {
			t.Errorf("%s: got %+v, want %+v", c.name, got, c.want)
		}
	}
}

func TestSelectUpstream_InvalidValue(t *testing.T) {
	t.Parallel()

	_, err := selectUpstream(map[string]string{"pier.upstream": "ftp://:21"}, "http")
	if err == nil || errors.Is(err, errNoUpstream) {
		t.Errorf("got %v, want a parse error", err)
	}
}

func TestValidateLabels(t *testing.T) {
	t.Parallel()

	valid := map[string]string{
		"pier.upstream":       "3000",
		"pier.upstream.https": "https://:8443?insecure=true",
		"pier.redirect":       "https",
		"other.label":         "ignored",
	}
	if errs := validateLabels(valid); len(errs) != 0 {
		t.Errorf("valid labels: %v", errs)
	}

	invalid := map[string]string{
		"pier.upstream":      "ftp://:21",
		"pier.redirect":      "http",
		"pier.port":          "3000",
		"pier.upstream.http": "3000",
	}
	if errs := validateLabels(invalid); len(errs) != 3 {
		t.Errorf("invalid labels: got %d errors (%v), want 3", len(errs), errs)
	}
}
