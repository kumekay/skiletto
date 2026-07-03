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
