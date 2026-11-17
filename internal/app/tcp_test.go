package app

import (
	"strings"
	"testing"
)

func TestTCPCmd_Errors(t *testing.T) {
	t.Parallel()

	cases := map[string][]string{
		"no mappings":       {"tcp"},
		"malformed mapping": {"tcp", "3306"},
		"bad dial timeout":  {"tcp", "127.0.0.1:0=db:3306", "--dial-timeout", "0s"},
	}

	for name, args := range cases {
		cmd := NewCmd("pier", "test")
		cmd.SetArgs(args)
		if err := cmd.Execute(); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestRootCmd_OnlyOwnSubcommands(t *testing.T) {
	t.Parallel()

	var names []string
	for _, c := range NewCmd("pier", "test").Commands() {
		if c.IsAvailableCommand() {
			names = append(names, c.Name())
		}
	}
	if strings.Join(names, ",") != "tcp" {
		t.Errorf("subcommands = %v, want only tcp", names)
	}
}
