package main

import (
	"os"

	"github.com/devrecon/ludus/cmd/root"
)

func main() {
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
