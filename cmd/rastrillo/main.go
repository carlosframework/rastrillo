// Command rastrillo is the CARLOS web framework's CLI: rastrillo new
// scaffolds an app, rastrillo generate runs the filesystem-routing
// generator. Subcommand dispatch only — everything real lives in this
// package's other files, one concern per file, per the family's own
// convention.
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "new":
		err = runNew(os.Args[2:])
	case "generate":
		err = runGenerate(os.Args[2:])
	case "-h", "--help", "help":
		usage()
		return
	default:
		fmt.Fprintf(os.Stderr, "rastrillo: unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "rastrillo: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `rastrillo — the CARLOS web framework CLI

Usage:
  rastrillo new <name>       scaffold a new app in ./<name>
  rastrillo generate [dir]   run the filesystem-routing generator (default: .)
`)
}
