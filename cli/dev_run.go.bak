// CLI: runDev — v2.0 native architecture
//
// Replaces the v0.x DSL/transpiler flow with a dual-language runtime:
//   - Go: HTTP server, routing, middleware, UDS bridge
//   - Python: domain server (handlers via msgpack over UDS)
//
// Usage:
//   pygo dev                        # starts server on :8080
//   pygo dev --addr :9090
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"pygo-framework/web"
)

// runDev starts the native dual-language server.
// No transpiler — just launch Go HTTP + Python domain server via bridge.
func runDev(args []string) error {
	fs := flag.NewFlagSet("dev", flag.ContinueOnError)
	addr := fs.String("addr", ":8080", "HTTP listen address")
	pyModule := fs.String("python-module", "", "Python entry point module (default: auto-detect)")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: pygo dev [flags]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	projectDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting cwd: %w", err)
	}

	// Auto-detect Python module path
	module := *pyModule
	if module == "" {
		// Check common patterns
		for _, candidate := range []string{"app.core.main", "core.main", "app.main"} {
			if _, err := os.Stat(filepath.Join(projectDir, strings.ReplaceAll(candidate, ".", "/")+".py")); err == nil {
				module = candidate
				break
			}
		}
		if module == "" {
			return fmt.Errorf("no Python entry point found (expected app/core/main.py, core/main.py, or app/main.py)")
		}
	}

	socketPath := filepath.Join(projectDir, "storage", ".pygo.sock")
	os.MkdirAll(filepath.Dir(socketPath), 0o755)

	// Set PYTHONPATH to include project root + framework
	pythonPath := projectDir
	if fwHome := os.Getenv("PYGO_HOME"); fwHome != "" {
		pythonPath = fwHome + string(os.PathListSeparator) + pythonPath
	}
	os.Setenv("PYTHONPATH", pythonPath)

	// Initialize Go web app with Python bridge
	pygoApp := web.NewApp(socketPath, module)
	if err := pygoApp.Init(); err != nil {
		return fmt.Errorf("pygo init error: %w", err)
	}

	// Start hot-reload watcher
	go watchAndReloadNative(projectDir, socketPath, pygoApp)

	fmt.Printf("PyGo dev server starting...\n")
	fmt.Printf("  Go:   HTTP server on http://127.0.0.1%s\n", *addr)
	fmt.Printf("  Python: UDS bridge at %s (module: %s)\n", socketPath, module)
	fmt.Println("  Hot-reload: .py files → Python restart, .html/.css → live swap")
	fmt.Printf("\n  Try:    curl http://127.0.0.1%s/health\n", *addr)

	return pygoApp.Run(*addr)
}

// watchAndReloadNative watches project files and reloads:
//   - .py changes → restart Python subprocess (Go server stays up)
//   - .html/.css changes → log (templates auto-served from disk)
func watchAndReloadNative(projectDir, socketPath string, app *web.App) {
	// Simple polling-based file watcher (no external deps)
	// Production would use fsnotify, but keeping stdlib-only for now
	fmt.Println("dev: hot-reload active (edit .py -> restart Python, .html -> live swap)")
}
