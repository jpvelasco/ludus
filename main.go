package main

import (
	"os"

	"github.com/jpvelasco/ludus/cmd/root"
)

func main() {
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
