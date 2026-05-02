package main

import (
	"os"

	"github.com/shahneil76/kubectl-inventory/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
