package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/harbor-hub/pier/internal/tcp"
)

const tcpShutdownTimeout = 10 * time.Second

// NewTCPCmd returns the "tcp" subcommand: forward TCP ports to targets that
// are resolved on every connection.
func NewTCPCmd(version string) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "tcp <listen>=<host>:<port>...",
		Short:         "Forward TCP ports to targets resolved on every connection",
		Args:          cobra.MinimumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(c *cobra.Command, args []string) error {
			resolver, _ := c.Flags().GetString("resolver")
			dialTimeout, _ := c.Flags().GetDuration("dial-timeout")
			verbose, _ := c.Flags().GetBool("verbose")

			mappings, err := parseMappings(args)
			if err != nil {
				return err
			}
			if dialTimeout <= 0 {
				return errors.New("--dial-timeout must be positive")
			}

			fmt.Printf("\nPier tcp %s\n\n  resolver:     %s\n  dial-timeout: %s\n\n",
				version,
				orDefault(resolver, "(system)"),
				dialTimeout,
			)
			for _, m := range mappings {
				fmt.Printf("  %s\n", m)
			}
			fmt.Println()

			proxy := &tcp.Proxy{
				Mappings:    mappings,
				Resolver:    resolver,
				DialTimeout: dialTimeout,
				Verbose:     verbose,
			}
			if err := proxy.Start(); err != nil {
				return err
			}

			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			<-ctx.Done()

			log.Printf("[TCP] shutting down")
			proxy.Shutdown(tcpShutdownTimeout)

			return nil
		},
	}

	cmd.Flags().SortFlags = false
	cmd.Flags().String("resolver", LookupEnv("PIER_TCP_RESOLVER", "").GetString(), "DNS server for target names, host[:port] (empty = system resolver)")
	cmd.Flags().Duration("dial-timeout", LookupEnv("PIER_TCP_DIAL_TIMEOUT", "5s").GetDuration(), "Timeout for connecting to a target")
	cmd.Flags().BoolP("verbose", "v", LookupEnv("PIER_VERBOSE", false).GetBool(), "Log every connection")

	return cmd
}

func parseMappings(args []string) ([]tcp.Mapping, error) {
	mappings := make([]tcp.Mapping, 0, len(args))
	for _, a := range args {
		m, err := tcp.ParseMapping(a)
		if err != nil {
			return nil, err
		}
		mappings = append(mappings, m)
	}
	return mappings, nil
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
