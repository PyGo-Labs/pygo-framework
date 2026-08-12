// Package dev implements the PyGo development server with hot-reload.
//
// pygo dev starts the Go web layer and Python domain server, watches for
// file changes (*.go, *.py, *.html, *.toml), and auto-restarts on modify.
// No transpiler, no DSL — native dual-language hot-reload.
package dev

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"slices"
)

// watcherExt defines file extensions that trigger reload
var watcherExt = []string{".go", ".py", ".html", ".css", ".js", ".toml"}

// Server coordinates the Go binary launch with live file-watching for hot-reload.
type Server struct {
	cmd        *exec.Cmd
	ctx        context.Context
	cancel     context.CancelFunc
	binaryPath string
	args       []string
}

// Start launches the Go binary and watches files for changes.
func Start(binaryPath string, args []string) error {
	s := &Server{
		binaryPath: binaryPath,
		args:       args,
	}

	// Initial build + launch
	if err := s.build(); err != nil {
		return fmt.Errorf("initial build failed: %w", err)
	}
	if err := s.launch(); err != nil {
		return fmt.Errorf("initial launch failed: %w", err)
	}

	// Watch files
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("file watcher: %w", err)
	}
	defer watcher.Close()

	// Watch project dirs
	dirs := []string{"app", "app/web", "app/core", "app/core/models",
		"app/core/services", "app/views", "app/views/layouts", "app/views/components"}
	for _, dir := range dirs {
		if _, err := os.Stat(dir); err == nil {
			filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
				if err == nil && d.IsDir() {
					if err := watcher.Add(path); err != nil {
						log.Printf("[dev] watching %s: %v", path, err)
					}
				}
				return nil
			})
		}
	}

	debounce := time.NewTimer(0)
	defer debounce.Stop()

	for {
		select {
		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			log.Printf("[dev] watcher error: %v", err)

		case event := <-watcher.Events:
			// Debounce rapid changes
			debounce.Reset(300 * time.Millisecond)
			<-debounce.C
			// Only react to watched extensions
			if !slices.ContainsFunc(watcherExt, func(ext string) bool {
				return strings.HasSuffix(event.Name, ext)
			}) {
				continue
			}
			if strings.HasSuffix(event.Name, ".go") {
				log.Printf("[dev] Go file changed: %s — rebuilding...", event.Name)
				if err := s.build(); err != nil {
					log.Printf("[dev] BUILD ERROR: %v", err)
					continue
				}
				log.Println("[dev] ✅ Build OK — restarting...")
			} else {
				log.Printf("[dev] File changed: %s — restarting...", event.Name)
			}
			if err := s.restart(); err != nil {
				log.Printf("[dev] RESTART ERROR: %v", err)
			}

		case <-s.ctx.Done():
			if s.cmd != nil && s.cmd.Process != nil {
				s.cmd.Process.Signal(os.Interrupt)
			}
			return nil
		}
	}
}

func (s *Server) build() error {
	log.Println("[dev] Building Go web layer...")
	buildCmd := exec.Command("go", "build", "-o", s.binaryPath, "./app/web/")
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stderr
	return buildCmd.Run()
}

func (s *Server) launch() error {
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.cmd = exec.CommandContext(s.ctx, s.binaryPath, s.args...)
	s.cmd.Stdout = os.Stdout
	s.cmd.Stderr = os.Stderr
	return s.cmd.Start()
}

func (s *Server) restart() error {
	// Kill old process
	if s.cmd != nil && s.cmd.Process != nil {
		s.cmd.Process.Signal(os.Interrupt)
		s.cmd.Wait()
	}
	// Relaunch
	return s.launch()
}
