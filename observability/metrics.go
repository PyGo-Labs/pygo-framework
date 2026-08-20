package observability

import (
	"log"
	"os"
	"sync"
	"time"
)

// Logger provides structured logging.
type Logger struct {
	*log.Logger
	level string
}

var (
	loggerInstance *Logger
	loggerOnce     sync.Once
)

// GetLogger returns the singleton logger.
func GetLogger() *Logger {
	loggerOnce.Do(func() {
		level := os.Getenv("PYGO_LOG_LEVEL")
		if level == "" {
			level = "info"
		}
		loggerInstance = &Logger{
			Logger: log.New(os.Stdout, "[PyGo] ", log.LstdFlags|log.Lmicroseconds),
			level:  level,
		}
	})
	return loggerInstance
}

// Info logs info-level messages.
func Info(format string, v ...interface{}) {
	GetLogger().Printf("[INFO] "+format, v...)
}

// Warn logs warn-level messages.
func Warn(format string, v ...interface{}) {
	if GetLogger().level == "debug" || GetLogger().level == "warn" || GetLogger().level == "info" {
		GetLogger().Printf("[WARN] "+format, v...)
	}
}

// Error logs error-level messages.
func Error(format string, v ...interface{}) {
	GetLogger().Printf("[ERROR] "+format, v...)
}

// Debug logs debug-level messages.
func Debug(format string, v ...interface{}) {
	if GetLogger().level == "debug" {
		GetLogger().Printf("[DEBUG] "+format, v...)
	}
}

// Metrics tracks request counters.
type Metrics struct {
	mu              sync.Mutex
	requestsTotal   int64
	errorsTotal     int64
	requestDuration time.Duration
	handlerCounts   map[string]int64
}

var metricsInstance = &Metrics{
	handlerCounts: make(map[string]int64),
}

// RecordRequest records a request metric.
func RecordRequest(handler string, duration time.Duration, isError bool) {
	metricsInstance.mu.Lock()
	defer metricsInstance.mu.Unlock()

	metricsInstance.requestsTotal++
	metricsInstance.requestDuration += duration
	if isError {
		metricsInstance.errorsTotal++
	}
	metricsInstance.handlerCounts[handler]++
}

// GetMetrics returns current metrics snapshot.
func GetMetrics() map[string]interface{} {
	metricsInstance.mu.Lock()
	defer metricsInstance.mu.Unlock()

	avgDuration := time.Duration(0)
	if metricsInstance.requestsTotal > 0 {
		avgDuration = metricsInstance.requestDuration / time.Duration(metricsInstance.requestsTotal)
	}

	return map[string]interface{}{
		"requests_total":   metricsInstance.requestsTotal,
		"errors_total":     metricsInstance.errorsTotal,
		"avg_duration_ms":  avgDuration.Milliseconds(),
		"handler_counts":   metricsInstance.handlerCounts,
		"uptime":          time.Since(time.Now()).String(),
	}
}
