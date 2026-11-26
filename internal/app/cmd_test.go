package app

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSplitDomains(t *testing.T) {
	t.Parallel()

	cases := map[string][]string{
		"lh,test":        {"lh", "test"},
		" lh , .test. ,": {"lh", "test"},
		"":               nil,
		",,":             nil,
		"lh.":            {"lh"},
	}

	for in, want := range cases {
		if got := splitDomains(in); !reflect.DeepEqual(got, want) {
			t.Errorf("splitDomains(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestParseDNSListen(t *testing.T) {
	t.Parallel()

	valid := map[string]DNSListener{
		":53":                                      {Addr: ":53", Stub: defaultDNSStub},
		":53?stub=192.168.1.10":                    {Addr: ":53", Stub: "192.168.1.10"},
		"127.0.0.1:53?stub=::1":                    {Addr: "127.0.0.1:53", Stub: "::1"},
		"[::1]:53?stub=10.0.0.1":                   {Addr: "[::1]:53", Stub: "10.0.0.1"},
		":53?stub=self&upstream=auto":              {Addr: ":53", Stub: selfStub, Upstream: []string{autoUpstream}},
		":53?upstream=1.1.1.1&upstream=8.8.8.8:53": {Addr: ":53", Stub: defaultDNSStub, Upstream: []string{"1.1.1.1", "8.8.8.8:53"}},
	}
	for in, want := range valid {
		got, err := parseDNSListen(in)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("parseDNSListen(%q) = %+v, %v; want %+v", in, got, err, want)
		}
	}

	for _, in := range []string{
		"?stub=127.0.0.1",           // no address
		":53?domain=lh",             // unknown option
		":53?stub=1.1.1.1&stub=::1", // repeated
		":53?stub=%zz",              // bad escape
	} {
		if _, err := parseDNSListen(in); err == nil {
			t.Errorf("parseDNSListen(%q): expected error", in)
		}
	}
}

func TestParseDNSListeners(t *testing.T) {
	t.Parallel()

	got, err := parseDNSListeners([]string{":5153, :53?stub=self", "", "127.0.0.1:54?stub=10.0.0.1"})
	want := []DNSListener{{Addr: ":5153", Stub: defaultDNSStub}, {Addr: ":53", Stub: selfStub}, {Addr: "127.0.0.1:54", Stub: "10.0.0.1"}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, %v; want %v", got, err, want)
	}

	if got, err := parseDNSListeners(nil); err != nil || got != nil {
		t.Errorf("nil: got %v, %v", got, err)
	}
	if _, err := parseDNSListeners([]string{":53,?stub=self"}); err == nil {
		t.Error("missing address: expected error")
	}
}

func TestReadToken(t *testing.T) {
	t.Parallel()

	file := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(file, []byte("  secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if got, err := readToken("direct", ""); err != nil || got != "direct" {
		t.Errorf("direct: got %q, %v", got, err)
	}
	if got, err := readToken("", file); err != nil || got != "secret" {
		t.Errorf("file: got %q, %v", got, err)
	}
	if _, err := readToken("direct", file); err == nil {
		t.Error("both: expected error")
	}
	if _, err := readToken("", file+".missing"); err == nil {
		t.Error("missing file: expected error")
	}
}

func TestValidate(t *testing.T) {
	t.Parallel()

	valid := map[string]Pier{
		"http only":                  {HTTPAddr: ":80", SourceRefresh: minSourceRefresh},
		"https without cert":         {HTTPSAddr: ":443", SourceRefresh: minSourceRefresh}, // passthrough only
		"https with cert and key":    {HTTPSAddr: ":443", HTTPSCert: "c.pem", HTTPSKey: "k.pem", SourceRefresh: minSourceRefresh},
		"dns with domain and stub":   {DNSListeners: []DNSListener{{Addr: ":53", Stub: "127.0.0.1"}}, DNSDomains: []string{"pier"}, SourceRefresh: minSourceRefresh},
		"dns with IPv6 stub address": {DNSListeners: []DNSListener{{Addr: ":53", Stub: "::1"}}, DNSDomains: []string{"pier"}, SourceRefresh: minSourceRefresh},
		"dns with two listeners":     {DNSListeners: []DNSListener{{Addr: ":5153", Stub: "127.0.0.1"}, {Addr: ":53", Stub: selfStub, Upstream: []string{autoUpstream}}}, DNSDomains: []string{"lh"}, SourceRefresh: minSourceRefresh},
		"dns with upstream servers":  {DNSListeners: []DNSListener{{Addr: ":53", Stub: "127.0.0.1", Upstream: []string{"1.1.1.1", "[::1]:5353"}}}, DNSDomains: []string{"lh"}, SourceRefresh: minSourceRefresh},
	}
	for name, p := range valid {
		if err := p.validate(); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}

	cases := map[string]Pier{
		"nothing enabled":    {SourceRefresh: minSourceRefresh},
		"dns without domain": {DNSListeners: []DNSListener{{Addr: ":53", Stub: "127.0.0.1"}}, SourceRefresh: minSourceRefresh},
		"invalid stub":       {DNSListeners: []DNSListener{{Addr: ":53", Stub: "localhost"}}, DNSDomains: []string{"lh"}, SourceRefresh: minSourceRefresh},
		"duplicate listener": {DNSListeners: []DNSListener{{Addr: ":53", Stub: "127.0.0.1"}, {Addr: ":53", Stub: selfStub}}, DNSDomains: []string{"lh"}, SourceRefresh: minSourceRefresh},
		"auto with servers":  {DNSListeners: []DNSListener{{Addr: ":53", Stub: "127.0.0.1", Upstream: []string{autoUpstream, "1.1.1.1"}}}, DNSDomains: []string{"lh"}, SourceRefresh: minSourceRefresh},
		"upstream hostname":  {DNSListeners: []DNSListener{{Addr: ":53", Stub: "127.0.0.1", Upstream: []string{"dns.google"}}}, DNSDomains: []string{"lh"}, SourceRefresh: minSourceRefresh},
		"cert without key":   {HTTPSAddr: ":443", HTTPSCert: "c.pem", SourceRefresh: minSourceRefresh},
		"key without cert":   {HTTPSAddr: ":443", HTTPSKey: "k.pem", SourceRefresh: minSourceRefresh},
		"invalid https addr": {HTTPSAddr: "443", SourceRefresh: minSourceRefresh},
		"refresh too small":  {HTTPAddr: ":80", SourceRefresh: minSourceRefresh / 2},
	}

	for name, p := range cases {
		if err := p.validate(); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}
