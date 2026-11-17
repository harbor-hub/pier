package app

import (
	"github.com/spf13/cobra"
)

func NewRootCmd(use, version string, fn func(c *cobra.Command, args []string) error) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:                   use,
		Version:               version,
		SilenceUsage:          true,
		SilenceErrors:         true,
		DisableFlagsInUseLine: true,
		Args:                  cobra.NoArgs,
		RunE: func(c *cobra.Command, args []string) error {
			return fn(c, args)
		},
	}

	rootCmd.SetVersionTemplate("{{printf \"%s\" .Version}}\n")

	return rootCmd
}

func ApplyDefaultFlags(rootCmd *cobra.Command) {
	rootCmd.Flags().BoolP("version", "V", false, "Show version")
	rootCmd.Flags().BoolP("help", "h", false, "Show help")
}
