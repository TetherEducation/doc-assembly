package main

import (
	"fmt"
	"os"

	"github.com/TetherEducation/doc-assembly/core/cmd/api/bootstrap"
	"github.com/TetherEducation/doc-assembly/core/extensions"
)

func main() {
	// Subcommand: migrate
	if len(os.Args) > 1 && os.Args[1] == "migrate" {
		engine := bootstrap.New()
		if err := engine.RunMigrations(); err != nil {
			fmt.Fprintf(os.Stderr, "migration error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Normal startup
	engine := bootstrap.New()
	extensions.Register(engine)

	if err := engine.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}
