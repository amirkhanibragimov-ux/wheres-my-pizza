package domain

// Minimal logger contract the domain depends on.
// Adapters must emit JSON with fields required by the spec.
type Logger interface {
	Info(action, msg string, fields map[string]any)
	Debug(action, msg string, fields map[string]any)
	Error(action string, err error, fields map[string]any)
}
