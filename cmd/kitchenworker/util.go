package kitchenworker

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	service "git.platform.alem.school/amibragim/wheres-my-pizza/internal/app/kitchenworker"
	"git.platform.alem.school/amibragim/wheres-my-pizza/internal/shared/contracts"
	"git.platform.alem.school/amibragim/wheres-my-pizza/internal/shared/logger"

	"github.com/rabbitmq/amqp091-go"
)

// detectWorkerType returns a canonical CSV list of supported order types.
func detectWorkerType(orderTypes *string) string {
	allowed := map[string]struct{}{
		"dine_in":  {},
		"takeout":  {},
		"delivery": {},
	}
	order := []string{"dine_in", "takeout", "delivery"}

	// helper function to render canonical list from a presence set
	render := func(have map[string]bool) string {
		var out []string
		for _, k := range order {
			if have[k] {
				out = append(out, k)
			}
		}
		// join everything in a single string
		return strings.Join(out, ",")
	}

	// default: all supported
	if orderTypes == nil || strings.TrimSpace(*orderTypes) == "" {
		return render(map[string]bool{
			"dine_in":  true,
			"takeout":  true,
			"delivery": true,
		})
	}

	parts := strings.Split(*orderTypes, ",")
	have := map[string]bool{}

	for _, p := range parts {
		p = strings.ToLower(strings.TrimSpace(p))
		if p == "" {
			continue
		}
		if _, ok := allowed[p]; ok {
			have[p] = true
		}
	}

	// if user input yields nothing valid, treat as "all supported"
	if len(have) == 0 {
		return render(map[string]bool{
			"dine_in":  true,
			"takeout":  true,
			"delivery": true,
		})
	}

	return render(have)
}

// handleDelivery decodes, processes and acks/nacks a single message.
func handleDelivery(
	ctx context.Context,
	logger *logger.Logger,
	processor *service.Processor,
	workerName, workerTypes string,
	d amqp091.Delivery,
) {
	// decode the message
	var msg contracts.OrderMessage
	if err := json.Unmarshal(d.Body, &msg); err != nil {
		logger.Error(ctx, "message_decode_failed", "Failed to decode OrderMessage", err)
		_ = d.Nack(false, false) // DLX for unrecoverable malformed JSON
		return
	}

	// process the message
	now := time.Now().UTC()
	err := processor.Process(ctx, workerName, workerTypes, msg, now)
	if err == nil {
		_ = d.Ack(false)
		return
	}

	// classify the error and decide on Ack/Nack
	switch {
	case errors.Is(err, service.ErrUnsupportedOrderType):
		logger.Debug(ctx, "unsupported_order_type",
			"Nack with requeue (unsupported type for this worker)",
			map[string]any{"order_number": msg.OrderNumber, "order_type": msg.OrderType})
		_ = d.Nack(false, true) // requeue to another specialized worker
	case service.IsRetryable(err):
		logger.Error(ctx, "processing_retryable",
			"Processing failed; requeuing for retry", err)
		_ = d.Nack(false, true) // transient -> requeue
	default:
		logger.Error(ctx, "processing_failed",
			"Processing failed; nacking to DLX", err)
		_ = d.Nack(false, false) // permanent -> DLX
	}
}
