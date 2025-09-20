package notificationservice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"git.platform.alem.school/amibragim/wheres-my-pizza/internal/shared/contracts"
	"git.platform.alem.school/amibragim/wheres-my-pizza/internal/shared/logger"
	"git.platform.alem.school/amibragim/wheres-my-pizza/internal/shared/rabbitmq"

	amqp "github.com/rabbitmq/amqp091-go"
)

// ConsumeForever continuously (re)creates a channel and starts consuming from the durable `notifications_queue`.
func ConsumeForever(ctx context.Context, rmq *rabbitmq.Client, logger *logger.Logger) {
	const (
		queueName      = "notifications_queue"
		prefetch       = 50               // limit unacked messages this consumer can hold
		retryBaseDelay = time.Second      // backoff base
		retryMaxDelay  = 30 * time.Second // backoff cap
		consumerName   = ""               // let the server generate a unique consumer tag
		autoAck        = false
		exclusive      = false
		noLocal        = false
		noWait         = false
	)

	backoff := retryBaseDelay
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// acquire a fresh channel with QoS
		ch, err := rmq.NewConsumerChannel(prefetch)
		if err != nil {
			logger.Error(ctx, "rabbitmq_channel_open_failed", "Failed to open consumer channel", err)
			if !sleepWithContext(ctx, backoff) {
				return
			}
			backoff = nextBackoff(backoff, retryMaxDelay)
			continue
		}

		// reset backoff on successful channel creation
		backoff = retryBaseDelay

		// start consuming from the durable queue
		deliveries, err := ch.Consume(queueName, consumerName, autoAck, exclusive, noLocal, noWait, nil)
		if err != nil {
			_ = ch.Close()
			logger.Error(ctx, "rabbitmq_consume_failed", "Failed to start consuming notifications", err)
			if !sleepWithContext(ctx, backoff) {
				return
			}
			backoff = nextBackoff(backoff, retryMaxDelay)
			continue
		}

		// watch for channel close to trigger a re-open
		closed := ch.NotifyClose(make(chan *amqp.Error, 1))

		// consume loop
	consumption:
		for {
			select {
			case <-ctx.Done():
				// stop consuming new messages; channel will be closed by defer in caller
				_ = ch.Close()
				break consumption

			case amqpErr := <-closed:
				// channel closed (or connection closed underneath). Recreate channel.
				if amqpErr != nil {
					logger.Error(ctx, "rabbitmq_channel_closed", "Consumer channel closed", amqpErr)
				} else {
					logger.Error(ctx, "rabbitmq_channel_closed", "Consumer channel closed", errors.New("unknown channel close"))
				}
				break consumption

			case d, ok := <-deliveries:
				if !ok {
					// delivery channel closed (same as closed signal)
					logger.Error(ctx, "rabbitmq_deliveries_closed", "Deliveries channel closed", errors.New("deliveries channel closed"))
					break consumption
				}

				// handle one message
				handleDelivery(ctx, logger, d)
			}
		}

		// small delay before attempting to recreate channel (avoid hot loop)
		if !sleepWithContext(ctx, backoff) {
			return
		}
		backoff = nextBackoff(backoff, retryMaxDelay)
	}
}

// handleDelivery parses the status update JSON and prints/acknowledges.
func handleDelivery(ctx context.Context, logger *logger.Logger, d amqp.Delivery) {
	// parse JSON
	var update contracts.StatusUpdateMessage
	if err := json.Unmarshal(d.Body, &update); err != nil {
		logger.Error(ctx, "notification_decode_failed", "Failed to decode status update JSON", err)
		// for this subscriber, malformed JSON cannot be recovered by redelivery - ack to drop it
		_ = d.Ack(false)
		return
	}

	// log at debug level
	logger.Debug(ctx, "notification_received", "Received status update", map[string]any{
		"order_number": update.OrderNumber,
		"old_status":   update.OldStatus,
		"new_status":   update.NewStatus,
		"changed_by":   update.ChangedBy,
	})

	// print human-readable line to stdout
	fmt.Println(renderHuman(update))

	// ack on success
	if err := d.Ack(false); err != nil {
		logger.Error(ctx, "rabbitmq_ack_failed", "Failed to ack notification message", err)
	}
}

// renderHuman formats a human-readable line as per project description.
func renderHuman(update contracts.StatusUpdateMessage) string {
	if update.EstimatedCompletion != nil {
		return fmt.Sprintf(
			"Notification for order %s: Status changed from '%s' to '%s' by %s. Estimated time of completion: %s",
			update.OrderNumber, update.OldStatus, update.NewStatus, update.ChangedBy, update.EstimatedCompletion.UTC().Format(time.RFC3339),
		)
	}

	return fmt.Sprintf(
		"Notification for order %s: Status changed from '%s' to '%s' by %s.",
		update.OrderNumber, update.OldStatus, update.NewStatus, update.ChangedBy,
	)
}

// Helpers

// sleepWithContext sleeps for the given duration or returns early if ctx is done.
func sleepWithContext(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// nextBackoff calculates the next backoff duration with exponential growth capped at max.
func nextBackoff(curr, cap time.Duration) time.Duration {
	n := curr * 2
	if n > cap {
		return cap
	}
	return n
}
