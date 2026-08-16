// Command mison is a CLI tool for reproducing and synchronizing
// development environments across machines, using mise as its engine.
package main

import (
	"fmt"
	"os"

	"github.com/dev-hann/mison/internal/cli"
)

// version is overridden at build time via
// -ldflags "-X main.version=..." (see .goreleaser.yml).
var version = "dev"

func main() {
	if err := cli.Execute(version); err != nil {
		fmt.Fprintln(os.Stderr, "mison:", err)
		os.Exit(1)
	}
}
