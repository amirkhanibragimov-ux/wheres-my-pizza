package orderservice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"git.platform.alem.school/amibragim/wheres-my-pizza/internal/domain/orders"
	"git.platform.alem.school/amibragim/wheres-my-pizza/internal/ports"
	"git.platform.alem.school/amibragim/wheres-my-pizza/internal/shared/contracts"
	"git.platform.alem.school/amibragim/wheres-my-pizza/internal/shared/logger"
)

type Publisher interface {
	Publish(exchange, routingKey string, body []byte, priority uint8) error
}

// Service implements ports.OrderService.
type Service struct {
	uow       ports.UnitOfWork
	repo      ports.OrderRepository
	logger    *logger.Logger
	publisher Publisher
}

// Ensure Service implements the interface at compile time.
var _ ports.OrderService = (*Service)(nil)

// New creates a new OrderService with the required dependencies.
func New(uow ports.UnitOfWork, repo ports.OrderRepository, logger *logger.Logger, publisher Publisher) *Service {
	return &Service{
		uow:       uow,
		repo:      repo,
		logger:    logger,
		publisher: publisher,
	}
}

// PlaceOrder validates input, builds a domain Order, and returns a summary.
func (service *Service) PlaceOrder(ctx context.Context, cmd ports.CreateOrderCommand) (ports.OrderPlaced, error) {
	// basic validation
	if len(cmd.Items) < 1 || len(cmd.Items) > 20 {
		return ports.OrderPlaced{}, errors.New("order must contain between 1 and 20 items")
	}

	cmd.CustomerName = strings.TrimSpace(cmd.CustomerName)
	if len(cmd.CustomerName) < 1 || len(cmd.CustomerName) > 100 {
		return ports.OrderPlaced{}, errors.New("customer_name must be 1-100 characters long")
	}

	re := regexp.MustCompile(`^[A-Za-z][A-Za-z '-]{0,99}$`)
	if !re.MatchString(cmd.CustomerName) {
		return ports.OrderPlaced{}, errors.New("customer_name must not contain special characters")
	}

	// validate type-specific fields
	switch cmd.Type {
	case orders.OrderTypeDineIn:
		if cmd.TableNumber == nil || *cmd.TableNumber < 1 || *cmd.TableNumber > 100 {
			return ports.OrderPlaced{}, errors.New("dine_in requires table_number in 1..100")
		}
		if cmd.DeliveryAddress != nil {
			return ports.OrderPlaced{}, errors.New("dine_in must not have delivery_address")
		}
	case orders.OrderTypeDelivery:
		if cmd.DeliveryAddress == nil || len(strings.TrimSpace(*cmd.DeliveryAddress)) < 10 {
			return ports.OrderPlaced{}, errors.New("delivery requires a non-empty delivery_address (>= 10 chars)")
		}
		if cmd.TableNumber != nil {
			return ports.OrderPlaced{}, errors.New("delivery must not have table_number")
		}
	case orders.OrderTypeTakeout:
		if cmd.TableNumber != nil || cmd.DeliveryAddress != nil {
			return ports.OrderPlaced{}, errors.New("takeout must not have table_number or delivery_address")
		}
	default:
		return ports.OrderPlaced{}, fmt.Errorf("unknown order type: %q", cmd.Type)
	}

	// validate items
	for i := range cmd.Items {
		cmd.Items[i].Name = strings.TrimSpace(cmd.Items[i].Name)
		if len(cmd.Items[i].Name) < 1 || len(cmd.Items[i].Name) > 50 {
			return ports.OrderPlaced{}, fmt.Errorf("item %d name must be between 1 and 50 characters", i+1)
		}
		if cmd.Items[i].Quantity < 1 || cmd.Items[i].Quantity > 10 {
			return ports.OrderPlaced{}, fmt.Errorf("item %d quantity must be between 1 and 10", i+1)
		}
		if cmd.Items[i].Price < 1 || cmd.Items[i].Price > 99999 {
			return ports.OrderPlaced{}, fmt.Errorf("item %d price must be between 0.01 and 999.99", i+1)
		}
	}

	// define variables to hold the order and the result
	var order orders.Order
	var placed ports.OrderPlaced

	// run within a transaction
	err := service.uow.WithinTx(ctx, func(txCtx context.Context) error {
		// build the aggregate
		order.CustomerName = cmd.CustomerName
		order.Type = cmd.Type
		order.TableNumber = cmd.TableNumber
		if cmd.DeliveryAddress != nil {
			addr := strings.TrimSpace(*cmd.DeliveryAddress)
			order.DeliveryAddress = &addr
		}

		// copy items
		order.Items = make([]orders.OrderItem, len(cmd.Items))
		for i, item := range cmd.Items {
			order.Items[i] = orders.OrderItem{
				Name:     item.Name,
				Quantity: item.Quantity,
				Price:    item.Price,
			}
		}

		// compute totals & priority using your domain helper.
		order.SetTotalAmount()
		order.SetPriorityFromTotalAmount()

		// get the next order number for today
		now := time.Now().UTC()
		seq, err := service.repo.NextOrderSeq(txCtx, now)
		if err != nil {
			service.logger.Error(ctx, "db_transaction_failed", "failed to get next order sequence", err)
			return err
		}
		order.Number = fmt.Sprintf("ORD_%s_%03d", now.Format("20060102"), seq)

		// add order to the database
		if err := service.repo.CreateOrder(txCtx, &order); err != nil {
			service.logger.Error(ctx, "db_transaction_failed", "failed to create order", err)
			return err
		}

		// construct and return the summary for response
		placed = ports.OrderPlaced{
			OrderNumber: order.Number,
			Status:      order.Status,
			TotalAmount: order.TotalAmount,
			Priority:    order.Priority,
		}
		return nil
	})
	if err != nil {
		return ports.OrderPlaced{}, err
	}

	// check if publisher is configured
	if service.publisher == nil {
		return ports.OrderPlaced{}, fmt.Errorf("no publisher configured")
	}

	mItems := make([]contracts.OrderItemMessage, len(order.Items))
	for i, it := range order.Items {
		mItems[i] = contracts.OrderItemMessage{
			Name:     it.Name,
			Quantity: it.Quantity,
			Price:    it.Price.ToFloat2(),
		}
	}

	msg := contracts.OrderMessage{
		OrderNumber:     order.Number,
		CustomerName:    order.CustomerName,
		OrderType:       string(order.Type),
		TableNumber:     order.TableNumber,
		DeliveryAddress: order.DeliveryAddress,
		Items:           mItems,
		TotalAmount:     order.TotalAmount.ToFloat2(),
		Priority:        order.Priority,
	}

	body, err := json.Marshal(msg)
	if err != nil {
		service.logger.Error(ctx, "rabbitmq_publish_failed", "failed to encode order message", err)
		return placed, nil // Do not fail the HTTP flow because DB commit already succeeded
	}

	routingKey := fmt.Sprintf("kitchen.%s.%d", string(cmd.Type), placed.Priority)
	if pubErr := service.publisher.Publish("orders_topic", routingKey, body, uint8(placed.Priority)); pubErr != nil {
		service.logger.Error(ctx, "rabbitmq_publish_failed", "failed to publish order message", pubErr)
	} else {
		service.logger.Debug(ctx, "order_published", "order published to RabbitMQ", map[string]any{
			"order_number": placed.OrderNumber,
			"routing_key":  routingKey,
			"priority":     placed.Priority,
		})
	}

	return placed, err
}
