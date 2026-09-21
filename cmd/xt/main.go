package main

import (
	"fmt"
	"os"

	"xt/internal/cli"
)

func main() {
	if err := cli.New(os.Stdout, os.Stderr).Run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
