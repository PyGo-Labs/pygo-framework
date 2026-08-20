// Package web is the PyGo Framework web orchestrator.
package web

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"pygo-framework/bridge"
)

type App struct {
	router *Router
	pool   *bridge.Pool
	socket string
	module string
	server *http.Server
}

type Router struct {
	mux   *http.ServeMux
	pool  *bridge.Pool
	auth  map[string]bool
	muxed map[string]map[string]func(map[string]interface{}) (interface{}, error)
}

func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.mux.ServeHTTP(w, req)
}

func NewApp(socketPath, pyModule string) *App {
	if socketPath == "" {
		socketPath = filepath.Join("storage", ".pygo.sock")
	}
	os.MkdirAll(filepath.Dir(socketPath), 0o755)
	if pyModule == "" {
		pyModule = "core.main"
	}
	return &App{
		router: &Router{
			mux:   http.NewServeMux(),
			auth:  make(map[string]bool),
			muxed: make(map[string]map[string]func(map[string]interface{}) (interface{}, error)),
		},
		socket: socketPath,
		module: pyModule,
	}
}

func (a *App) Router() *Router {
	return a.router
}

func (r *Router) Handle(method, pattern string, handler func(map[string]interface{}) (interface{}, error), auth bool) {
	r.auth[pattern] = auth
	if _, ok := r.muxed[pattern]; !ok {
		r.muxed[pattern] = make(map[string]func(map[string]interface{}) (interface{}, error))
		r.mux.HandleFunc(pattern, r.routeHandler(pattern))
	}
	r.muxed[pattern][method] = handler
}

func (r *Router) routeHandler(pattern string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		// A panicking handler must not kill the connection with an empty reply.
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("handler panic on %s %s: %v", req.Method, req.URL.Path, rec)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"result": nil,
					"error":  fmt.Sprintf("internal handler error: %v", rec),
				})
			}
		}()

		methods, ok := r.muxed[pattern]
		if !ok {
			http.NotFound(w, req)
			return
		}

		ctx := extractParams(pattern, req.URL.Path)

		// Expose the request path so handlers can use it safely
		ctx["_path"] = req.URL.Path
		ctx["_method"] = req.Method

		// Inject query params
		for k, v := range req.URL.Query() {
			if len(v) > 0 {
				ctx[k] = v[0]
			}
		}

		// Parse body
		if req.Body != nil {
			if err := req.ParseForm(); err == nil {
				for k, v := range req.PostForm {
					if len(v) > 0 {
						ctx[k] = v[0]
					}
				}
			}
			body, _ := io.ReadAll(req.Body)
			if len(body) > 0 {
				var jsonData map[string]interface{}
				if err := json.Unmarshal(body, &jsonData); err == nil {
					for k, v := range jsonData {
						ctx[k] = v
					}
				}
			}
		}

		handler, ok := methods[req.Method]
		if !ok {
			http.Error(w, `{"error":"Method Not Allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		// Filter internal params
		handlerCtx := make(map[string]interface{})
		for k, v := range ctx {
			if k != "token" {
				handlerCtx[k] = v
			}
		}

		result, err := handler(handlerCtx)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]interface{}{"result": nil, "error": err.Error()})
			return
		}

		// A handler that returns raw HTML must be served as HTML, not as an
		// escaped JSON string (the browser would render the escaped source).
		if s, ok := result.(string); ok {
			trimmed := strings.TrimSpace(s)
			if strings.HasPrefix(trimmed, "<!DOCTYPE") || strings.HasPrefix(trimmed, "<html") ||
				strings.HasPrefix(trimmed, "<div") || strings.HasPrefix(trimmed, "<section") ||
				strings.HasPrefix(trimmed, "<tr") || strings.HasPrefix(trimmed, "<table") ||
				strings.HasPrefix(trimmed, "<form") || strings.HasPrefix(trimmed, "<ul") ||
				strings.HasPrefix(trimmed, "<li") || strings.HasPrefix(trimmed, "<span") ||
				strings.HasPrefix(trimmed, "<p") || strings.HasPrefix(trimmed, "<option") {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.Write([]byte(s))
				return
			}
		}

		// Single wrap: result from Python is already {result, error}
		w.Header().Set("Content-Type", "application/json")
		if m, ok := result.(map[string]interface{}); ok {
			if _, hasResult := m["result"]; hasResult {
				json.NewEncoder(w).Encode(m)
			} else {
				json.NewEncoder(w).Encode(map[string]interface{}{"result": result, "error": nil})
			}
		} else {
			json.NewEncoder(w).Encode(map[string]interface{}{"result": result, "error": nil})
		}
	}
}

func extractParams(pattern, path string) map[string]interface{} {
	ctx := make(map[string]interface{})
	parts := strings.Split(pattern, "/")
	pathParts := strings.Split(path, "/")
	for i, part := range parts {
		if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") {
			paramName := part[1 : len(part)-1]
			if i < len(pathParts) {
				ctx[paramName] = pathParts[i]
			}
		}
	}
	return ctx
}

func (a *App) Init() error {
	pool, err := bridge.NewPool(a.socket, a.module, "")
	if err != nil {
		return err
	}
	a.pool = pool
	return nil
}

func (a *App) Call(method string, ctx map[string]interface{}) (interface{}, error) {
	return a.pool.Call(method, ctx)
}

func (a *App) Run(addr string) error {
	// SO_REUSEADDR to prevent "address already in use"
	lc := net.ListenConfig{}
	ln, err := lc.Listen(context.Background(), "tcp", addr)
	if err != nil {
		return err
	}

	a.server = &http.Server{
		Handler: a.router,
	}

	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig
		log.Println("Shutting down...")
		a.server.Shutdown(context.Background())
		os.Remove(a.socket)
	}()

	log.Printf("PyGo web server ready on %s", addr)
	log.Printf("UDS bridge to Python: %s", a.socket)

	return a.server.Serve(ln)
}
