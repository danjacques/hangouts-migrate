package main

import (
	"os"
	"github.com/danjacques/hangouts-migrate/tools/analysis"
)

func main() {
	os.Exit(analysis.Main(os.Args))
}
