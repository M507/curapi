package main

import (
	"os"

	"github.com/tageecc/cursor-agent-api-proxy/internal/cli"
)

var version = "2.0.0"

func main() {
	if version != "" {
		cli.Version = version
	}
	os.Exit(cli.Execute(os.Args))
}
