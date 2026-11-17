package app

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

func NewCmd(name, version string) *cobra.Command {
	pier := New()

	rootCmd := NewRootCmd(name, version, func(c *cobra.Command, args []string) error {
		dnsAddr, _ := c.Flags().GetString("dns-listen")
		dnsDomains, _ := c.Flags().GetString("dns-domain")
		dnsStub, _ := c.Flags().GetString("dns-stub")
		httpAddr, _ := c.Flags().GetString("http-listen")
		httpsAddr, _ := c.Flags().GetString("https-listen")
		httpsCert, _ := c.Flags().GetString("https-cert")
		httpsKey, _ := c.Flags().GetString("https-key")
		source, _ := c.Flags().GetString("source")
		sourceToken, _ := c.Flags().GetString("source-token")
		sourceTokenFile, _ := c.Flags().GetString("source-token-file")
		sourceRefresh, _ := c.Flags().GetDuration("source-refresh")
		verbose, _ := c.Flags().GetBool("verbose")

		token, err := readToken(sourceToken, sourceTokenFile)
		if err != nil {
			return err
		}

		pier.Version = version
		pier.DNSAddr = dnsAddr
		pier.DNSDomains = splitDomains(dnsDomains)
		pier.DNSStub = dnsStub
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
	rootCmd.Flags().String("dns-listen", LookupEnv("PIER_DNS_LISTEN", "").GetString(), "DNS stub listen address, e.g. :53 (empty = disabled)")
	rootCmd.Flags().String("dns-domain", LookupEnv("PIER_DNS_DOMAIN", "pier").GetString(), "Comma-separated domains the DNS stub answers for")
	rootCmd.Flags().String("dns-stub", LookupEnv("PIER_DNS_STUB", "127.0.0.1").GetString(), "IP address the DNS stub answers with")
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
