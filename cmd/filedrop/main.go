package main

import (
	"os"

	"github.com/aimuz/filedrop-cli/internal/cli"
)

func main() {
	os.Exit(cli.Main(os.Args[1:]))
}
