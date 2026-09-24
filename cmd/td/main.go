package main

import (
	"os"

	"github.com/jonezzyboy/tandem/internal/cli"
)

func main() {
	os.Exit(cli.Main(os.Args[1:]))
}
