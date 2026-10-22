package main

import (
	"fmt"
	"os"

	"github.com/tejasunku/caretaker/internal/cli"
)

func main() {
	app := cli.New()
	if err := app.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
