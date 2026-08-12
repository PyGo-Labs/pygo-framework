// Package bridge implements the Unix Domain Socket (UDS) connection pool
// between Go (web/orchestration layer) and Python (domain/services layer).
// It uses MessagePack for compact binary serialization (<1ms latency).
package bridge

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/vmihailenco/msgpack/v5"
)

// MAX_POOL conns por proceso Python
const MAX_POOL = 4

// PythonProcess manages the Python subprocess lifecycle
type PythonProcess struct {
	cmd       *exec.Cmd
	socket    string
	module    string // e.g. "app.core.main"
	ready     bool
}

// Start launches python3 -m <module> (the domain server)
func (p *PythonProcess) Start(ctx context.Context) error {
	if p.module == "" {
		p.module = "core.main" // default
	}
	p.cmd = exec.CommandContext(ctx, "python3", "-m", p.module, "--socket", p.socket)
	p.cmd.Stdout = os.Stdout
	p.cmd.Stderr = os.Stderr
	if err := p.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start python: %w", err)
	}
	// Wait for socket file to appear
	for i := 0; i < 50; i++ {
		if _, err := os.Stat(p.socket); err == nil {
			p.ready = true
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("python did not create socket in 5s")
}

func (p *PythonProcess) Stop() error {
	if p.cmd != nil && p.cmd.Process != nil {
		return p.cmd.Process.Signal(os.Interrupt)
	}
	return nil
}

// Pool is a pool of UDS connections to the Python runtime
type Pool struct {
	mu       sync.Mutex
	conns    chan net.Conn
	process  *PythonProcess
}

// NewPool creates and starts the Python process, then opens pooled UDS conns.
// modulePath is the Python module to run (e.g. "app.core.main").
func NewPool(socketPath, modulePath string) (*Pool, error) {
	proc := &PythonProcess{
		socket: socketPath,
		module: modulePath,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := proc.Start(ctx); err != nil {
		return nil, err
	}
	p := &Pool{
		conns:   make(chan net.Conn, MAX_POOL),
		process: proc,
	}
	// Pre-open connections
	for i := 0; i < MAX_POOL; i++ {
		conn, err := net.Dial("unix", socketPath)
		if err != nil {
			return nil, fmt.Errorf("dial error: %w", err)
		}
		p.conns <- conn
	}
	return p, nil
}

// Call invokes a Python handler by qualified name: "core.services.users.get_profile"
// Returns the deserialized result or an error.
func (p *Pool) Call(method string, args map[string]interface{}) (interface{}, error) {
	start := time.Now()
	select {
	case conn := <-p.conns:
		defer func() { p.conns <- conn }()
		msg := map[string]interface{}{
			"method": method,
			"args":   args,
		}
		encoded, err := msgpack.Marshal(msg)
		if err != nil {
			return nil, fmt.Errorf("msgpack marshal: %w", err)
		}
		// 4-byte length prefix
		header := make([]byte, 4)
		header[0] = byte(len(encoded) >> 24)
		header[1] = byte(len(encoded) >> 16)
		header[2] = byte(len(encoded) >> 8)
		header[3] = byte(len(encoded))
		if _, err := conn.Write(header); err != nil {
			return nil, fmt.Errorf("write header: %w", err)
		}
		if _, err := conn.Write(encoded); err != nil {
			return nil, fmt.Errorf("write body: %w", err)
		}
		// Read response
		respHeader := make([]byte, 4)
		if _, err := conn.Read(respHeader); err != nil {
			return nil, fmt.Errorf("read response header: %w", err)
		}
		respLen := int(respHeader[0])<<24 | int(respHeader[1])<<16 |
			int(respHeader[2])<<8 | int(respHeader[3])
		respBody := make([]byte, respLen)
		if _, err := conn.Read(respBody); err != nil {
			return nil, fmt.Errorf("read response body: %w", err)
		}
		var result interface{}
		if err := msgpack.Unmarshal(respBody, &result); err != nil {
			return nil, fmt.Errorf("msgpack unmarshal: %w", err)
		}
		latency := time.Since(start)
		_ = latency // used by observability middleware
		return result, nil
	case <-time.After(5 * time.Second):
		return nil, fmt.Errorf("timeout calling %s after 5s", method)
	}
}

// Close shuts down the pool and Python process
func (p *Pool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i := 0; i < cap(p.conns); i++ {
		conn := <-p.conns
		conn.Close()
	}
	return p.process.Stop()
}
