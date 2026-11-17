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
		"dns with domain and stub":   {DNSAddr: ":53", DNSDomains: []string{"pier"}, DNSStub: "127.0.0.1", SourceRefresh: minSourceRefresh},
		"dns with IPv6 stub address": {DNSAddr: ":53", DNSDomains: []string{"pier"}, DNSStub: "::1", SourceRefresh: minSourceRefresh},
	}
	for name, p := range valid {
		if err := p.validate(); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}

	cases := map[string]Pier{
		"nothing enabled":    {SourceRefresh: minSourceRefresh},
		"dns without domain": {DNSAddr: ":53", DNSStub: "127.0.0.1", SourceRefresh: minSourceRefresh},
		"invalid stub":       {DNSAddr: ":53", DNSDomains: []string{"lh"}, DNSStub: "localhost", SourceRefresh: minSourceRefresh},
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
