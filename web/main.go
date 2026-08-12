// Package web is the PyGo Framework web orchestrator.
// It implements the Go-native HTTP server, routing, middleware,
// and the UDS bridge to Python domain services.
package web

import (
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"pygo-framework/bridge"
)

// App is the main PyGo web application
type App struct {
	router   http.Handler
	pool     *bridge.Pool
	socket   string
	module   string // Python entry point, e.g. "app.core.main"
}

// NewApp creates a new PyGo web app with UDS bridge to Python.
// socketPath: UDS socket path (empty = default).
// pyModule: Python entry module, e.g. "app.core.main" (empty = "core.main").
func NewApp(socketPath, pyModule string) *App {
	if socketPath == "" {
		socketPath = filepath.Join("storage", ".pygo.sock")
	}
	os.MkdirAll(filepath.Dir(socketPath), 0o755)
	if pyModule == "" {
		pyModule = "core.main"
	}
	return &App{
		socket: socketPath,
		module: pyModule,
	}
}

// Init starts the Python subprocess and opens UDS connection pool
func (a *App) Init() error {
	pool, err := bridge.NewPool(a.socket, a.module)
	if err != nil {
		return err
	}
	a.pool = pool

	// Setup router
	mux := http.NewServeMux()
	mux.HandleFunc("/", a.handleRoot)
	mux.HandleFunc("/health", a.handleHealth)
	// Routes are registered by modules via RegisterRoutes()
	a.router = mux
	return nil
}

// Run starts the HTTP server on :8080
func (a *App) Run(addr string) error {
	if addr == "" {
		addr = ":8080"
	}
	srv := &http.Server{
		Addr:    addr,
		Handler: a.router,
	}

	// Graceful shutdown
	go func() {
		// Wait for interrupt signal
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		log.Println("Shutting down...")
		os.Remove(a.socket)
		a.pool.Close()
		os.Exit(0)
	}()

	log.Printf("PyGo web server ready on %s", addr)
	log.Printf("UDS bridge to Python: %s", a.socket)
	return srv.ListenAndServe()
}

// Call delegates to Python domain layer via bridge
// Usage: result, err := app.Call("core.services.users.get_profile", map[string]any{"id": 123})
func (a *App) Call(method string, args map[string]interface{}) (interface{}, error) {
	return a.pool.Call(method, args)
}

// handleRoot is a simple dashboard route
func (a *App) handleRoot(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(`<!DOCTYPE html>
<html><head><title>PyGo ERP</title></head>
<body><h1>PyGo ERP — Running</h1>
<p>Server: Go 1.23 + Python bridge ready</p>
</body></html>`))
}

// handleHealth returns JSON health status
func (a *App) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok","runtime":"hybrid","languages":["go","python"]}`))
}

// Main is the entry point — should be called from app/web/main.go
// Usage:
//   func main() {
//       app := web.NewApp("")
//       app.Init()
//       app.Run(":8080")
//   }
func Main() {
	app := NewApp("", "core.main")  // default module
	if err := app.Init(); err != nil {
		log.Fatalf("failed to init: %v", err)
	}
	if err := app.Run(os.Getenv("PORT")); err != nil {
		log.Fatalf("failed to run: %v", err)
	}
}
