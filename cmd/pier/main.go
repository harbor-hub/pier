package main

import (
	"fmt"
	"os"
	"path"

	"github.com/harbor-hub/pier/internal/app"
)

var version = "0.x-dev"

func main() {
	name := path.Base(os.Args[0])
	cmd := app.NewCmd(name, version)

	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
