// Package main — PyGo hello-world example (native dual-language architecture).
// Go HTTP server delegates business logic to Python via UDS + msgpack.
package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"

	"pygo-framework/web"
)

func main() {
	projectDir, _ := os.Getwd()
	socketPath := projectDir + "/storage/.pygo.sock"
	os.MkdirAll("storage", 0o755)
	os.MkdirAll("app/views", 0o755)

	pygoApp := web.NewApp(socketPath, "app.core.main")
	if err := pygoApp.Init(); err != nil {
		log.Fatalf("PyGo init error: %v", err)
	}

	// Route: GET / → dashboard (Go native)
	pygoApp.Router().Handle("GET", "/", func(ctx map[string]interface{}) (interface{}, error) {
		html := `<!DOCTYPE html>
<html><head><title>PyGo Hello World</title></head>
<body><h1>PyGo ERP — Running</h1><p>Server: Go 1.23 + Python bridge ready</p></body></html>`
		return html, nil
	}, false, false)

	// Route: GET /health → Go native health check
	pygoApp.Router().Handle("GET", "/health", func(ctx map[string]interface{}) (interface{}, error) {
		return map[string]interface{}{"status": "ok", "runtime": "hybrid"}, nil
	}, false, false)

	// Route: GET /hello/{name} → Python handler "hello.greet" via bridge
	pygoApp.Router().Handle("GET", "/hello/{name}", func(ctx map[string]interface{}) (interface{}, error) {
		name := ctx["name"]
		result, err := pygoApp.Call("hello.greet", map[string]interface{}{"name": name})
		if err != nil {
			return nil, err
		}
		return result, nil
	}, false, false)

	log.Println("PyGo hello-world server ready on http://127.0.0.1:8080")
	log.Println("  Try: curl http://127.0.0.1:8080/health")
	log.Println("  Try: curl http://127.0.0.1:8080/hello/World")
	log.Fatal(pygoApp.Run(":8080"))
}

// unused import guard
var _ = json.Marshal
var _ = http.StatusOK
var _ = strings.TrimSpace
