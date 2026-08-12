// Command pygo is the PyGo framework CLI (v2.0 — native dual-language architecture).
//
// Subcommands:
//   pygo new <name>    Scaffold a new PyGo project
//   pygo dev            Start the dual-language dev server (:8080)
//   pygo module <cmd>   Module management
//
// Only the Go standard library is used (flag). No third-party CLI framework
// is added so go.mod stays untouched.
package main

import (
	"fmt"
	"os"
)

const usage = `pygo — PyGo framework CLI (v2.0 — native dual-language)

Usage:
  pygo <command> [arguments]

Commands:
  new <name>     Create a new PyGo project in ./<name>/
  dev            Start the dev server (:8080) — Go HTTP + Python UDS bridge
  module         Module discovery and management

Run "pygo <command> -h" for command-specific flags.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	var err error
	switch cmd {
	case "new":
		err = runNew(args)
	case "dev":
		err = runDev(args)
	case "module":
		err = runModule(args)
	case "-h", "--help", "help":
		fmt.Print(usage)
		return
	default:
		fmt.Fprintf(os.Stderr, "pygo: unknown command %q\n\n%s", cmd, usage)
		os.Exit(2)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "pygo %s: %v\n", cmd, err)
		os.Exit(1)
	}
}
