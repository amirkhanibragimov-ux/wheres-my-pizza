// internal/app/orderservice/orders_http.go
package orderservice

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"git.platform.alem.school/amibragim/wheres-my-pizza/internal/domain/orders"
	"git.platform.alem.school/amibragim/wheres-my-pizza/internal/ports"
	"git.platform.alem.school/amibragim/wheres-my-pizza/internal/shared/logger"
)

// OrderHTTPHandler adapts HTTP requests to the OrderService.
type OrderHTTPHandler struct {
	svc    ports.OrderService
	logger *logger.Logger
}

// NewOrderHTTPHandler wires an HTTP handler around the OrderService.
func NewOrderHTTPHandler(svc ports.OrderService, logger *logger.Logger) *OrderHTTPHandler {
	return &OrderHTTPHandler{svc: svc, logger: logger}
}

// Register mounts the POST /orders route on the provided mux.
func (handler *OrderHTTPHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /orders", func(w http.ResponseWriter, r *http.Request) {
		handler.handleCreateOrder(r.Context(), w, r)
	})
}

// --- Request/Response DTOs (HTTP boundary) ---

type createOrderRequest struct {
	CustomerName    string                   `json:"customer_name"`
	OrderType       string                   `json:"order_type"`
	TableNumber     *int                     `json:"table_number,omitempty"`
	DeliveryAddress *string                  `json:"delivery_address,omitempty"`
	Items           []createOrderItemRequest `json:"items"`
}

type createOrderItemRequest struct {
	Name     string  `json:"name"`
	Quantity int     `json:"quantity"`
	Price    float64 `json:"price"` // decimal dollars (0.01..999.99)
}

type createOrderResponse struct {
	OrderNumber string  `json:"order_number"`
	Status      string  `json:"status"`
	TotalAmount float64 `json:"total_amount"`
	Priority    int     `json:"priority"`
}

// --- Handler ---

func (handler *OrderHTTPHandler) handleCreateOrder(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	// guard: content type + size
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MiB
	defer r.Body.Close()

	if ct := r.Header.Get("Content-Type"); ct != "" && !strings.HasPrefix(ct, "application/json") {
		handler.httpError(ctx, w, http.StatusUnsupportedMediaType, "Content-Type must be application/json", errors.New("unsupported content type: "+ct))
		return
	}

	// decode strictly
	var req createOrderRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		handler.httpError(ctx, w, http.StatusBadRequest, "invalid JSON: "+err.Error(), err)
		return
	}

	// map to service command (service performs full business validation).
	cmd, err := toCreateOrderCommand(req)
	if err != nil {
		handler.httpError(ctx, w, http.StatusBadRequest, err.Error(), err)
		return
	}

	handler.logger.Debug(ctx,
		"order_received",
		"new order request received",
		map[string]any{
			"customer_name": cmd.CustomerName,
			"order_type":    cmd.Type,
			"items_count":   len(cmd.Items),
		},
	)

	// bound request time
	ctxWithTimeout, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// call application service (validates, persists, numbers order)
	placed, err := handler.svc.PlaceOrder(ctxWithTimeout, cmd)
	if err != nil {
		// Distinguish DB failures from validation errors.
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			handler.httpError(ctxWithTimeout, w, http.StatusInternalServerError, "database error", err)
			return
		}
		handler.httpError(ctxWithTimeout, w, http.StatusBadRequest, err.Error(), err)
		return
	}

	// build response according to the spec
	resp := createOrderResponse{
		OrderNumber: placed.OrderNumber,
		Status:      string(placed.Status),
		TotalAmount: placed.TotalAmount.ToFloat2(),
		Priority:    placed.Priority,
	}
	handler.jsonResponse(ctxWithTimeout, w, http.StatusOK, resp)
}

// --- Helpers ---

func toCreateOrderCommand(req createOrderRequest) (ports.CreateOrderCommand, error) {
	typ, ok := parseOrderType(req.OrderType)
	if !ok {
		return ports.CreateOrderCommand{}, errors.New("order_type must be one of: 'dine_in', 'takeout', or 'delivery'")
	}

	items := make([]ports.ItemInput, len(req.Items))
	for i, it := range req.Items {
		items[i] = ports.ItemInput{
			Name:     it.Name,
			Quantity: it.Quantity,
			Price:    orders.NewMoneyFromFloat2(it.Price), // cents; service re-validates ranges
		}
	}

	return ports.CreateOrderCommand{
		CustomerName:    req.CustomerName,
		Type:            typ,
		TableNumber:     req.TableNumber,
		DeliveryAddress: req.DeliveryAddress,
		Items:           items,
	}, nil
}

func parseOrderType(s string) (orders.OrderType, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "dine_in", "dine-in", "dinein":
		return orders.OrderTypeDineIn, true
	case "takeout", "take_out":
		return orders.OrderTypeTakeout, true
	case "delivery":
		return orders.OrderTypeDelivery, true
	default:
		return "", false
	}
}

// httpError sends a JSON error response with a message.
func (handler *OrderHTTPHandler) httpError(ctx context.Context, w http.ResponseWriter, status int, msg string, err error) {
	// Map status -> action
	action := "request_failed"
	if status >= 500 {
		action = "http_internal_error"
	} else if status == http.StatusBadRequest {
		action = "validation_failed"
	} else if status == http.StatusUnsupportedMediaType {
		action = "unsupported_media_type"
	}
	handler.logger.Error(ctx, action, msg, err)

	type errBody struct {
		Error string `json:"error"`
	}
	handler.jsonResponse(ctx, w, status, errBody{Error: msg})
}

// jsonResponse takes any type of data and encode it to HTTP response.
func (h *OrderHTTPHandler) jsonResponse(ctx context.Context, w http.ResponseWriter, status int, data any) {
	// Encode to buffer first so we can control status on failure.
	var buf []byte
	var err error

	if data != nil {
		buf, err = json.Marshal(data)
		if err != nil {
			h.logger.Error(ctx, "response_encode_failed", "failed to encode response", err)
			http.Error(w, `{"error":"failed to encode response"}`, http.StatusInternalServerError)
			return
		}
	} else {
		// Empty JSON body if you want; or leave it blank for 204 cases.
		buf = []byte("{}")
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(buf)
}
