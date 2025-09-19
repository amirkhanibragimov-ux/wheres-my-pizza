package logger

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime/debug"
	"time"
)

// ErrorObject represents the error format.
type ErrorObject struct {
	Msg   string `json:"msg"`
	Stack string `json:"stack"`
}

// LogEntry represents the structured log format.
type LogEntry struct {
	Timestamp string       `json:"timestamp"`
	Level     string       `json:"level"`
	Service   string       `json:"service"`
	Action    string       `json:"action"`
	Message   string       `json:"message"`
	Hostname  string       `json:"hostname"`
	RequestID string       `json:"request_id"`
	Error     *ErrorObject `json:"error,omitempty"`
	Details   any          `json:"details,omitempty"` // optional - used only in specific examples
}

// Logger represents a custom structured logger.
type Logger struct {
	service  string
	hostname string
}

// New creates a new structured logger.
func NewLogger(service string) *Logger {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	return &Logger{
		service:  service,
		hostname: hostname,
	}
}

// Define an unexported type for context keys.
type ctxKey string

// requestIDKey is the context key for the request ID.
const requestIDKey ctxKey = "request_id"

// WithRequestID returns a context carrying a request id (useful for HTTP/mq hops).
func (logger *Logger) WithRequestID(ctx context.Context, rid string) context.Context {
	return context.WithValue(ctx, requestIDKey, rid)
}

// requestIDFrom returns a value saved in the context.
func requestIDFrom(ctx context.Context) string {
	if v := ctx.Value(requestIDKey); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// emit marshals the provided log entry.
func (logger *Logger) emit(entry LogEntry) {
	b, err := json.Marshal(entry)
	if err != nil {
		fmt.Fprintf(os.Stderr, "log marshal failed: %v\n", err)
		return
	}
	fmt.Println(string(b)) // writes to stdout per spec
}

// -- Logger helper functions --

func (logger *Logger) Info(ctx context.Context, action, msg string, details any) {
	logger.emit(LogEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Level:     "INFO",
		Service:   logger.service,
		Action:    action,
		Message:   msg,
		Hostname:  logger.hostname,
		RequestID: requestIDFrom(ctx),
		Details:   details,
	})
}

func (logger *Logger) Debug(ctx context.Context, action, msg string, details any) {
	logger.emit(LogEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Level:     "DEBUG",
		Service:   logger.service,
		Action:    action,
		Message:   msg,
		Hostname:  logger.hostname,
		RequestID: requestIDFrom(ctx),
		Details:   details,
	})
}

func (logger *Logger) Error(ctx context.Context, action, msg string, err error) {
	logger.emit(LogEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Level:     "ERROR",
		Service:   logger.service,
		Action:    action,
		Message:   msg,
		Hostname:  logger.hostname,
		RequestID: requestIDFrom(ctx),
		Error: &ErrorObject{
			Msg:   err.Error(),
			Stack: string(debug.Stack()),
		},
	})
}
