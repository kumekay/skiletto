// Command tto is the short alias binary for skiletto. It is identical to the
// root skiletto command; `go install github.com/kumekay/skiletto/cmd/tto@latest`
// installs it as `tto`, while the root package installs the long `skiletto`
// name (Go names the binary after the package path).
package main

import (
	"os"

	"github.com/kumekay/skiletto/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
