package trackingservice

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"git.platform.alem.school/amibragim/wheres-my-pizza/internal/ports"
	"git.platform.alem.school/amibragim/wheres-my-pizza/internal/shared/logger"
	"github.com/jackc/pgx/v5"
)

// TrackingHTTPHandler adapts HTTP requests to the TrackingService.
type TrackingHTTPHandler struct {
	logger *logger.Logger
	svc    ports.TrackingService
}

// NewHandler wires an HTTP handler around the TrackingService.
func NewHandler(logger *logger.Logger, svc ports.TrackingService) *TrackingHTTPHandler {
	return &TrackingHTTPHandler{logger: logger, svc: svc}
}

// Register mounts several HTTP routes on the provided mux.
func (handler *TrackingHTTPHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /orders/{order_number}/status", handler.getOrderStatus)
	mux.HandleFunc("GET /orders/{order_number}/history", handler.getOrderHistory)
	mux.HandleFunc("GET /workers/status", handler.listWorkers)
}

// --- Handlers ---

// getOrderStatus handles GET /orders/{order_number}/status and returns the current status of the order.
func (handler *TrackingHTTPHandler) getOrderStatus(w http.ResponseWriter, r *http.Request) {
	ctx := handler.withReqID(r.Context(), r)
	number := r.PathValue("order_number")
	handler.logger.Debug(ctx, "request_received", "GET /orders/{order_number}/status", map[string]any{"order_number": number})

	view, err := handler.svc.GetOrderStatus(ctx, number)
	if err != nil {
		handler.maybeNotFound(ctx, w, err)
		return
	}

	resp := map[string]any{
		"order_number":         view.OrderNumber,
		"current_status":       string(view.CurrentStatus),
		"updated_at":           view.UpdatedAt,
		"estimated_completion": view.EstimatedCompletion,
		"processed_by":         view.ProcessedBy,
	}

	handler.writeJSON(w, http.StatusOK, resp)
}

// getOrderHistory handles GET /orders/{order_number}/history and returns a list of status changes.
func (handler *TrackingHTTPHandler) getOrderHistory(w http.ResponseWriter, r *http.Request) {
	ctx := handler.withReqID(r.Context(), r)
	number := r.PathValue("order_number")
	handler.logger.Debug(ctx, "request_received", "GET /orders/{order_number}/history", map[string]any{"order_number": number})

	hist, err := handler.svc.GetOrderHistory(ctx, number)
	if err != nil {
		handler.maybeNotFound(ctx, w, err)
		return
	}

	if len(hist) == 0 {
		handler.writeErr(w, http.StatusNotFound, "not found")
		return
	}

	// build the response view
	out := make([]map[string]any, 0, len(hist))
	for i := range hist {
		out = append(out, map[string]any{
			"status":     string(hist[i].Status),
			"timestamp":  hist[i].ChangedAt,
			"changed_by": hist[i].ChangedBy,
		})
	}
	handler.writeJSON(w, http.StatusOK, out)
}

// listWorkers handles GET /workers/status?offline_after=60s and returns a list of workers with their status.
func (handler *TrackingHTTPHandler) listWorkers(w http.ResponseWriter, r *http.Request) {
	ctx := handler.withReqID(r.Context(), r)
	handler.logger.Debug(ctx, "request_received", "GET /workers/status", nil)

	// list workers from the service with a default offline_after duration
	offlineAfter := 60 * time.Second // default ~= 2 * 30s heartbeat
	now := time.Now().UTC()
	views, err := handler.svc.ListWorkers(ctx, offlineAfter, now)
	if err != nil {
		handler.writeErr(w, http.StatusInternalServerError, "internal server error")
		return
	}

	// build the response view
	out := make([]map[string]any, 0, len(views))
	for i := range views {
		out = append(out, map[string]any{
			"worker_name":      views[i].WorkerName,
			"status":           string(views[i].Status),
			"orders_processed": views[i].OrdersProcessed,
			"last_seen":        views[i].LastSeen,
		})
	}
	handler.writeJSON(w, http.StatusOK, out)
}

// --- Helpers ---

// maybeNotFound checks if the error indicates a not-found condition and writes a 404, otherwise logs the error and writes a 500.
func (handler *TrackingHTTPHandler) maybeNotFound(ctx context.Context, w http.ResponseWriter, err error) {
	// if repos return pgx.ErrNoRows on not found, then pass through
	if errors.Is(err, pgx.ErrNoRows) {
		handler.writeErr(w, http.StatusNotFound, "not found")
		return
	}

	// otherwise log and return a generic 500
	handler.logger.Error(ctx, "db_query_failed", "Database query failed", err)
	handler.writeErr(w, http.StatusInternalServerError, "internal server error")
}

// writeJSON writes the provided value as a JSON response with the given status code.
func (handler *TrackingHTTPHandler) writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// writeErr writes a JSON error response with a message.
func (handler *TrackingHTTPHandler) writeErr(w http.ResponseWriter, code int, msg string) {
	handler.writeJSON(w, code, map[string]any{"error": msg})
}

// withReqID extracts or generates a request ID and adds it to the context.
func (handler *TrackingHTTPHandler) withReqID(ctx context.Context, r *http.Request) context.Context {
	reqID := r.Header.Get("X-Request-ID")
	if reqID == "" {
		reqID = randID()
	}
	return handler.logger.WithRequestID(ctx, reqID)
}

// randID generates a random 24-char hex string suitable for request IDs.
func randID() string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
