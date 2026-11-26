package app

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

func NewCmd(name, version string) *cobra.Command {
	pier := New()

	rootCmd := NewRootCmd(name, version, func(c *cobra.Command, args []string) error {
		dnsListen, _ := c.Flags().GetStringArray("dns-listen")
		dnsDomains, _ := c.Flags().GetString("dns-domain")
		httpAddr, _ := c.Flags().GetString("http-listen")
		httpsAddr, _ := c.Flags().GetString("https-listen")
		httpsCert, _ := c.Flags().GetString("https-cert")
		httpsKey, _ := c.Flags().GetString("https-key")
		source, _ := c.Flags().GetString("source")
		sourceToken, _ := c.Flags().GetString("source-token")
		sourceTokenFile, _ := c.Flags().GetString("source-token-file")
		sourceRefresh, _ := c.Flags().GetDuration("source-refresh")
		verbose, _ := c.Flags().GetBool("verbose")

		dnsListeners, err := parseDNSListeners(dnsListen)
		if err != nil {
			return err
		}

		token, err := readToken(sourceToken, sourceTokenFile)
		if err != nil {
			return err
		}

		pier.Version = version
		pier.DNSListeners = dnsListeners
		pier.DNSDomains = splitDomains(dnsDomains)
		pier.HTTPAddr = httpAddr
		pier.HTTPSAddr = httpsAddr
		pier.HTTPSCert = httpsCert
		pier.HTTPSKey = httpsKey
		pier.Source = source
		pier.SourceToken = token
		pier.SourceRefresh = sourceRefresh
		pier.Verbose = verbose

		return pier.Run()
	})

	rootCmd.Flags().SortFlags = false
	rootCmd.Flags().StringArray("dns-listen", envList("PIER_DNS_LISTEN"), "DNS stub listen address, addr[?stub=IP|self][&upstream=auto|IP[:port]], e.g. :53 (stub default "+defaultDNSStub+", no upstream; repeatable or comma-separated; empty = disabled)")
	rootCmd.Flags().String("dns-domain", LookupEnv("PIER_DNS_DOMAIN", "pier").GetString(), "Comma-separated domains the DNS stub answers for")
	rootCmd.Flags().String("http-listen", LookupEnv("PIER_HTTP_LISTEN", "").GetString(), "HTTP listen address, e.g. :80 (empty = disabled)")
	rootCmd.Flags().String("https-listen", LookupEnv("PIER_HTTPS_LISTEN", "").GetString(), "HTTPS listen address, e.g. :443 (empty = disabled)")
	rootCmd.Flags().String("https-cert", LookupEnv("PIER_HTTPS_CERT", "").GetString(), "TLS certificate file")
	rootCmd.Flags().String("https-key", LookupEnv("PIER_HTTPS_KEY", "").GetString(), "TLS key file")
	rootCmd.Flags().String("source", LookupEnv("PIER_SOURCE", "").GetString(), "Records source: URL or JSON file")
	rootCmd.Flags().String("source-token", LookupEnv("PIER_SOURCE_TOKEN", "").GetString(), "Bearer token for a URL source (prefer the env var or --source-token-file)")
	rootCmd.Flags().String("source-token-file", LookupEnv("PIER_SOURCE_TOKEN_FILE", "").GetString(), "File containing the bearer token")
	rootCmd.Flags().Duration("source-refresh", LookupEnv("PIER_SOURCE_REFRESH", "5s").GetDuration(), "Source refresh interval (min 500ms)")
	rootCmd.Flags().BoolP("verbose", "v", LookupEnv("PIER_VERBOSE", false).GetBool(), "Enable verbose logging")

	ApplyDefaultFlags(rootCmd)

	// Help lists only our own subcommands: "completion" still works but is
	// hidden, the generated "help" command is replaced by a hidden no-op.
	rootCmd.CompletionOptions.HiddenDefaultCmd = true
	rootCmd.SetHelpCommand(&cobra.Command{Hidden: true})

	rootCmd.AddCommand(NewTCPCmd(version))

	return rootCmd
}

// readToken returns the token given directly or read from file; setting both
// is an error.
func readToken(token, file string) (string, error) {
	if file == "" {
		return token, nil
	}
	if token != "" {
		return "", errors.New("use either --source-token or --source-token-file, not both")
	}

	data, err := os.ReadFile(file)
	if err != nil {
		return "", fmt.Errorf("--source-token-file: %w", err)
	}

	return strings.TrimSpace(string(data)), nil
}

// envList returns the env var as a one-element default for a list flag, or
// nil when it is empty; the flag values are split on commas later.
func envList(key string) []string {
	if v := LookupEnv(key, "").GetString(); v != "" {
		return []string{v}
	}
	return nil
}

// parseDNSListeners parses every --dns-listen value; each may hold several
// comma-separated listeners.
func parseDNSListeners(values []string) ([]DNSListener, error) {
	var listeners []DNSListener

	for _, v := range values {
		for _, s := range strings.Split(v, ",") {
			if s = strings.TrimSpace(s); s == "" {
				continue
			}
			l, err := parseDNSListen(s)
			if err != nil {
				return nil, err
			}
			listeners = append(listeners, l)
		}
	}

	return listeners, nil
}

// parseDNSListen parses "addr[?stub=IP|self][&upstream=auto|IP[:port]...]":
// the listen address, what the stub answers with (defaultDNSStub when not
// given) and where other names are forwarded (none when not given).
func parseDNSListen(s string) (DNSListener, error) {
	addr, query, _ := strings.Cut(s, "?")
	l := DNSListener{Addr: addr, Stub: defaultDNSStub}
	if query == "" {
		return l, nil
	}
	if addr == "" {
		return DNSListener{}, fmt.Errorf("invalid --dns-listen %q: missing address", s)
	}

	values, err := url.ParseQuery(query)
	if err != nil {
		return DNSListener{}, fmt.Errorf("invalid --dns-listen %q: %w", s, err)
	}
	for key, v := range values {
		switch key {
		case "stub":
			if len(v) > 1 {
				return DNSListener{}, fmt.Errorf("invalid --dns-listen %q: stub given more than once", s)
			}
			l.Stub = v[0]
		case "upstream":
			l.Upstream = v
		default:
			return DNSListener{}, fmt.Errorf("invalid --dns-listen %q: unknown option %q", s, key)
		}
	}

	return l, nil
}

// splitDomains splits a comma-separated list of domains, trimming spaces and dots.
func splitDomains(s string) []string {
	var result []string

	for _, part := range strings.Split(s, ",") {
		part = strings.Trim(strings.TrimSpace(part), ".")
		if part != "" {
			result = append(result, part)
		}
	}

	return result
}
