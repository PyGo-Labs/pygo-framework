// Command pygo dev starts the development server with hot-reload.
//
// Usage:
//   pygo dev                    # auto-detects app/web/main.go
//   pygo dev --socket /path     # custom UDS socket path
//   pygo dev --port 9090        # custom port

package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"pygo-framework/cli/dev"
)

func runDev(args []string) error {
	fs := flag.NewFlagSet("dev", flag.ContinueOnError)
	socketPath := fs.String("socket", "", "UDS socket path")
	port := fs.String("port", "", "HTTP port (default :8080)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	// Auto-detect project layout
	projectDir, _ := os.Getwd()
	binaryPath := filepath.Join(projectDir, "storage", ".pygo-server")
	os.MkdirAll("storage", 0o755)

	portArg := ":8080"
	if *port != "" {
		portArg = ":" + *port
	}

	// Build args for the binary
	binaryArgs := []string{}
	if *socketPath != "" {
		binaryArgs = append(binaryArgs, "-socket", *socketPath)
	}
	binaryArgs = append(binaryArgs, "-port", portArg)

	fmt.Println("🚀 PyGo dev server starting with hot-reload...")
	fmt.Printf("  Project: %s\n", projectDir)
	fmt.Printf("  Binary:  %s\n", binaryPath)
	fmt.Printf("  Port:    %s\n", portArg)
	fmt.Println("  Watching: app/**/*.go, app/**/*.py, app/**/*.html")

	return dev.Start(binaryPath, binaryArgs)
}
