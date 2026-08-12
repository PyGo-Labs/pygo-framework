// Package web is the PyGo Framework web orchestrator.
// It implements the Go-native HTTP server, routing, middleware,
// and the UDS bridge to Python domain services.
package web

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"pygo-framework/bridge"
)

// App is the main PyGo web application
type App struct {
	router     *Router
	pool       *bridge.Pool
	socket     string
	module     string
	projectDir string
}

// Router is the PyGo HTTP router — Go native http.ServeMux wrapper
// that can delegate to Python handlers via the bridge.
type Router struct {
	mux   *http.ServeMux
	pool  *bridge.Pool
	auth  map[string]bool // protected routes
	muxed map[string]map[string]func(map[string]interface{}) (interface{}, error)
}

// ServeHTTP implements http.Handler — delegates to the underlying mux.
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.mux.ServeHTTP(w, req)
}

// NewApp creates a new PyGo web app with UDS bridge to Python.
func NewApp(socketPath, pyModule string) *App {
	if socketPath == "" {
		socketPath = filepath.Join("storage", ".pygo.sock")
	}
	os.MkdirAll(filepath.Dir(socketPath), 0o755)
	if pyModule == "" {
		pyModule = "core.main"
	}
	projectDir, _ := os.Getwd()
	return &App{
		socket:     socketPath,
		module:     pyModule,
		router:     &Router{mux: http.NewServeMux(), auth: make(map[string]bool), muxed: make(map[string]map[string]func(map[string]interface{}) (interface{}, error))},
		projectDir: projectDir,
	}
}

// Router returns the app's router for registering custom routes.
func (a *App) Router() *Router {
	return a.router
}

// Handle registers an HTTP route. Supports multiple HTTP methods on
// the same pattern by using an internal method-dispatch wrapper.
// If requiresPython is true, the handler is expected to call app.Call()
// to delegate to Python.
func (r *Router) Handle(method, pattern string, handler func(map[string]interface{}) (interface{}, error), auth, websockets bool) {
	key := method + " " + pattern
	r.auth[key] = auth

	// Check if pattern already registered — use muxer map for method dispatch
	if existing, ok := r.muxed[pattern]; ok {
		// Append method+handler to the dispatch map
		existing[method] = handler
		return
	}
	r.muxed[pattern] = map[string]func(map[string]interface{}) (interface{}, error){
		method: handler,
	}
	r.mux.HandleFunc(pattern, func(w http.ResponseWriter, req *http.Request) {
		// Method dispatch
		handlers := r.muxed[pattern]
		h, ok := handlers[req.Method]
		if !ok {
			// Try wildcard "" for methods without method filter
			if h, ok = handlers[""]; !ok {
				http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
				return
			}
		}
		if r.auth[req.Method+" "+pattern] {
			// Auth placeholder
		}
		ctx := extractParams(pattern, req.URL.Path)
		result, err := h(ctx)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if m, ok := result.(map[string]interface{}); ok {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(mustJSON(m)))
		} else if s, ok := result.(string); ok {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(s))
		} else {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(mustJSON(result)))
		}
	})
}

// Call delegates to Python domain layer via bridge.
// Usage: result, err := app.Router().Pool().Call("core.services.hello", {})
func (r *Router) Pool() *bridge.Pool {
	return r.pool
}

// extractParams extracts path params from pattern and request path.
// Supports Go 1.22+ syntax: /hello/{name}
// Falls back to legacy :name syntax for compatibility.
func extractParams(pattern, path string) map[string]interface{} {
	ctx := map[string]interface{}{}
	parts := splitPath(path)
	patParts := splitPath(pattern)
	for i, p := range patParts {
		if i >= len(parts) {
			break
		}
		// Go 1.22+ {param} syntax
		if len(p) >= 2 && p[0] == '{' && p[len(p)-1] == '}' {
			paramName := p[1 : len(p)-1]
			// Handle {name} or {name:regex}
			if idx := strings.Index(paramName, ":"); idx != -1 {
				paramName = paramName[:idx]
			}
			ctx[paramName] = parts[i]
		} else if len(p) > 1 && p[0] == ':' {
			// Legacy :param syntax (fallback)
			ctx[p[1:]] = parts[i]
		}
	}
	return ctx
}

func splitPathImpl(p string) []string {
	p = strings.Trim(p, "/")
	if p == "" {
		return []string{}
	}
	return strings.Split(p, "/")
}

func splitPath(p string) []string {
	return splitPathImpl(p)
}

func (a *App) Init() error {
	pool, err := bridge.NewPool(a.socket, a.module, a.projectDir)
	if err != nil {
		return err
	}
	a.pool = pool
	a.router.pool = pool // inject pool into router for handler access

	// Default routes are registered by the app, not here
	// Modules register their own routes via app.Router().Handle()
	return nil
}

// mustJSON encodes to JSON bytes (simple helper, no external deps).
func mustJSON(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
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
