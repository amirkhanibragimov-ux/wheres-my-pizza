package orderservice

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"git.platform.alem.school/amibragim/wheres-my-pizza/internal/domain/orders"
	"git.platform.alem.school/amibragim/wheres-my-pizza/internal/ports"
)

// Service implements ports.OrderService.
type Service struct {
	uow  ports.UnitOfWork
	repo ports.OrderRepository
}

// Ensure Service implements the interface at compile time.
var _ ports.OrderService = (*Service)(nil)

// New creates a new OrderService with the required dependencies.
func New(uow ports.UnitOfWork, repo ports.OrderRepository) *Service {
	return &Service{uow: uow, repo: repo}
}

// PlaceOrder validates input, builds a domain Order aggregate, assigns
// an order number, persists it transactionally, and returns a summary.
//
// Number generation note:
// We use a day-scoped "ORD_YYYYMMDD_NNN" format and retry on unique
// violations. Your DB has a per-day counter table (order_number_seq),
// which we can wire up later; for now, this retry strategy avoids SQL
// in the service layer and stays within the existing ports. (See TODO.)
func (s *Service) PlaceOrder(ctx context.Context, cmd ports.CreateOrderCommand) (ports.OrderPlaced, error) {
	// basic validation
	if len(cmd.Items) < 1 || len(cmd.Items) > 20 {
		return ports.OrderPlaced{}, errors.New("order must contain between 1 and 20 items")
	}

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
		if cmd.DeliveryAddress == nil || len(*cmd.DeliveryAddress) < 10 {
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
	for i, it := range cmd.Items {
		if len(it.Name) < 1 || len(it.Name) > 50 {
			return ports.OrderPlaced{}, fmt.Errorf("item %d name must be between 1 and 50 characters", i+1)
		}
		if it.Quantity < 1 || it.Quantity > 10 {
			return ports.OrderPlaced{}, fmt.Errorf("item %d quantity must be between 1 and 10", i+1)
		}
		if it.Price < 1 || it.Price > 99999 {
			return ports.OrderPlaced{}, fmt.Errorf("item %d price must be between 0.01 and 999.99", i+1)
		}
	}

	var placed ports.OrderPlaced
	err := s.uow.WithinTx(ctx, func(txCtx context.Context) error {
		now := time.Now().UTC()
		// build the aggregate
		var order orders.Order
		order.CustomerName = cmd.CustomerName
		order.Type = cmd.Type
		order.TableNumber = cmd.TableNumber
		order.DeliveryAddress = cmd.DeliveryAddress
		order.Items = make([]orders.OrderItem, len(cmd.Items))
		for i := range cmd.Items {
			in := cmd.Items[i]
			order.Items[i] = orders.OrderItem{
				Name:     in.Name,
				Quantity: in.Quantity,
				Price:    in.Price,
			}
		}

		// Compute totals & priority using your domain helper.
		order.SetTotalAmount()
		order.SetPriorityFromTotalAmount()

		// Assign an order number with retries on unique violation.
		const maxAttempts = 6
		for attempt := 0; attempt < maxAttempts; attempt++ {
			order.Number = makeOrderNumber(now, attempt)
			err := s.repo.CreateOrder(txCtx, &order)
			if err == nil {
				// Repo sets initial status to 'received' and returns it. :contentReference[oaicite:4]{index=4}
				placed = ports.OrderPlaced{
					OrderNumber: order.Number,
					Status:      order.Status,
					TotalAmount: order.TotalAmount,
					Priority:    order.Priority,
				}
				return nil
			}
			// On unique violation, try another suffix; otherwise bubble up.
			if !isUniqueViolation(err) {
				return err
			}
		}
		return errors.New("could not generate a unique order number after retries")
	})
	return placed, err
}

// makeOrderNumber builds "ORD_YYYYMMDD_NNN". The base (YYYYMMDD) is taken from 'now'.
// The suffix NNN is zero-padded and derived from the attempt counter to make
// retries deterministic within the same second/process.
func makeOrderNumber(now time.Time, attempt int) string {
	date := now.Format("20060102")
	suffix := attempt % 1000 // keep within 000..999
	return fmt.Sprintf("ORD_%s_%03d", date, suffix)
}

// isUniqueViolation returns true if err is a Postgres unique violation (SQLSTATE 23505).
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}
