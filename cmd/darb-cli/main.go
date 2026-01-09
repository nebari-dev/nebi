package main

import (
	"os"

	"github.com/aktech/darb/cmd/darb-cli/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
