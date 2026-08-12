// Package bridge implements the Unix Domain Socket (UDS) connection pool
// between Go (web/orchestration layer) and Python (domain/services layer).
// It uses MessagePack for compact binary serialization.
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
	cmd            *exec.Cmd
	socket         string
	module         string
	projectDir     string
	ready          bool
	shutdownCancel context.CancelFunc // cancels the context that owns the process
}

// Start launches python3 -m <module> (the domain server)
func (p *PythonProcess) Start(ctx context.Context) error {
	if p.module == "" {
		p.module = "core.main" // default
	}
	p.cmd = exec.CommandContext(ctx, "python3", "-m", p.module, "--socket", p.socket)

	// Set PYTHONPATH to include project dir and env PYTHONPATH
	pythonPath := p.projectDir
	if envPP := os.Getenv("PYTHONPATH"); envPP != "" {
		pythonPath = envPP + string(os.PathListSeparator) + pythonPath
	}
	p.cmd.Env = append(os.Environ(), "PYTHONPATH="+pythonPath)
	p.cmd.Dir = p.projectDir
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
	// Cancel the context (kills the subprocess via exec.CommandContext)
	if p.shutdownCancel != nil {
		p.shutdownCancel()
	}
	// Fallback: explicit signal
	if p.cmd != nil && p.cmd.Process != nil {
		return p.cmd.Process.Signal(os.Interrupt)
	}
	return nil
}

// Pool is a pool of UDS connections to the Python runtime
type Pool struct {
	mu      sync.Mutex
	socket  string
	module  string
	projectDir string
	process *PythonProcess
}

// NewPool creates and starts the Python process, then opens pooled UDS conns.
// modulePath is the Python module to run (e.g. "app.core.main").
// projectDir is the working directory (for PYTHONPATH resolution).
func NewPool(socketPath, modulePath, projectDir string) (*Pool, error) {
	proc := &PythonProcess{
		socket:     socketPath,
		module:     modulePath,
		projectDir: projectDir,
	}
	// Use a cancellable context stored in the process for later shutdown.
	// Do NOT defer cancel() here — the process must outlive NewPool().
	ctx, cancel := context.WithCancel(context.Background())
	proc.shutdownCancel = cancel
	if err := proc.Start(ctx); err != nil {
		return nil, err
	}
	p := &Pool{
		socket:     socketPath,
		module:     modulePath,
		projectDir: projectDir,
		process:    proc,
	}
	return p, nil
}

// Call invokes a Python handler by qualified name.
// Uses connection-per-request (dial → send → recv → close) to avoid
// stale connections in the pool. This is robust against Python restarts.
func (p *Pool) Call(method string, args map[string]interface{}) (interface{}, error) {
	start := time.Now()
	conn, err := net.Dial("unix", p.socket)
	if err != nil {
		return nil, fmt.Errorf("dial UDS: %w", err)
	}
	defer conn.Close()

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
}

// Close shuts down the pool and Python process
func (p *Pool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.process.Stop()
}
