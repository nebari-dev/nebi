package main

import (
	"os"

	"github.com/openteams-ai/darb/cmd/darb-cli/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
