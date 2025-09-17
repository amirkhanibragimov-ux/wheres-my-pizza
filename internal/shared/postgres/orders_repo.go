package postgres

import (
	"context"
	"time"

	"git.platform.alem.school/amibragim/wheres-my-pizza/internal/domain/orders"
	"git.platform.alem.school/amibragim/wheres-my-pizza/internal/ports"
	"github.com/jackc/pgx/v5"
)

// OrdersRepo implements persistence for orders using pgx and SQL.
type OrdersRepo struct{}

// NewOrdersRepo constructs a new OrdersRepo.
func NewOrdersRepo() ports.OrderRepository {
	return &OrdersRepo{}
}

// CreateOrder inserts the order header, its items, and an initial 'received' status log.
func (r *OrdersRepo) CreateOrder(ctx context.Context, order *orders.Order) error {
	tx, err := MustTxFromContext(ctx)
	if err != nil {
		return err
	}

	// insert new order into database
	// note: total_amount is NUMERIC(10,2) in DB; we send integer cents and divide by 100 in SQL
	var status string
	err = tx.QueryRow(ctx, `
		INSERT INTO orders (number, customer_name, type, table_number, delivery_address, total_amount, priority, status)
		VALUES ($1, $2, $3, $4, $5, $6::numeric/100, $7, 'received')
		RETURNING id, created_at, updated_at, status
		`,
		order.Number,
		order.CustomerName,
		order.Type,            // TEXT with check ('dine_in','takeout','delivery')
		order.TableNumber,     // NULL for non-dine_in
		order.DeliveryAddress, // NULL for non-delivery
		int64(order.TotalAmount),
		order.Priority,
	).Scan(&order.ID, &order.CreatedAt, &order.UpdatedAt, &status)
	if err != nil {
		return err
	}
	order.Status = orders.OrderStatus(status)

	// insert order items into database
	// note: price is NUMERIC(10,2) in DB; we send integer cents and divide by 100 in SQL
	for i := range order.Items {
		it := &order.Items[i]
		_, err = tx.Exec(ctx, `
			INSERT INTO order_items (order_id, name, quantity, price)
			VALUES ($1, $2, $3, $4::numeric/100)
		`,
			order.ID,
			it.Name,
			it.Quantity,
			int64(it.Price),
		)
		if err != nil {
			return err
		}
		it.OrderID = order.ID
	}

	// insert initial status log into database
	_, err = tx.Exec(ctx, `
		INSERT INTO order_status_log (order_id, status, changed_by, changed_at, notes)
		VALUES ($1, 'received', $2, now(), NULL)
	`,
		order.ID,
		"order-service",
	)
	return err
}

// GetByNumber retrieves an order by its unique number, including its items and current status.
func (r *OrdersRepo) GetByNumber(ctx context.Context, number string) (*orders.Order, error) {
	tx, err := MustTxFromContext(ctx)
	if err != nil {
		return nil, err
	}

	var order orders.Order
	var typeStr string
	var status string
	err = tx.QueryRow(ctx, `
		SELECT id, number, customer_name, type, table_number, delivery_address, total_amount::numeric*100, priority, status, created_at, updated_at
		FROM orders
		WHERE number = $1
	`, number).Scan(
		&order.ID, &order.Number, &order.CustomerName, &order.Type, &order.TableNumber, &order.DeliveryAddress,
		&order.TotalAmount, &order.Priority, &status, &order.CreatedAt, &order.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	order.Type = orders.OrderType(typeStr)
	order.Status = orders.OrderStatus(status)

	rows, err := tx.Query(ctx, `
		SELECT id, name, quantity, price::numeric*100
		FROM order_items
		WHERE order_id = $1
	`, order.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var item orders.OrderItem
		err = rows.Scan(&item.ID, &item.Name, &item.Quantity, &item.Price)
		if err != nil {
			return nil, err
		}
		order.Items = append(order.Items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return &order, nil
}

// UpdateStatusCAS atomically changes status from expected->next and writes a history row.
func (r *OrdersRepo) UpdateStatusCAS(
	ctx context.Context,
	number string,
	expected, next orders.OrderStatus,
	changedBy string,
	notes *string,
) (bool, error) {
	tx, err := MustTxFromContext(ctx)
	if err != nil {
		return false, err
	}

	// One round-trip:
	// 1) UPDATE orders ... WHERE number=$2 AND status=$3 RETURNING id     -> upd
	// 2) INSERT INTO order_status_log (...) SELECT id FROM upd            -> ins
	// 3) SELECT EXISTS(SELECT 1 FROM ins) tells us if the transition happened
	var ok bool
	err = tx.QueryRow(ctx, `
		WITH upd AS (
			UPDATE orders
			SET status = $1, updated_at = now()
			WHERE number = $2 AND status = $3
			RETURNING id
		),
		ins AS (
			INSERT INTO order_status_log (order_id, status, changed_by, changed_at, notes)
			SELECT id, $1, $4, now(), $5
			FROM upd
			RETURNING 1
		)
		SELECT EXISTS (SELECT 1 FROM ins);
	`, next, number, expected, changedBy, notes).Scan(&ok)
	if err != nil {
		return false, err
	}
	return ok, nil
}

// SetProcessedBy sets the worker who is processing the order.
func (r *OrdersRepo) SetProcessedBy(ctx context.Context, number string, worker string) error {
	tx, err := MustTxFromContext(ctx)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		UPDATE orders
		SET processed_by = $1, updated_at = now()
		WHERE number = $2
	`, worker, number)
	return err
}

// SetCompletedAt sets the completion timestamp for an order.
func (r *OrdersRepo) SetCompletedAt(ctx context.Context, orderNumber string, t time.Time) error {
	tx, err := MustTxFromContext(ctx)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		UPDATE orders
		SET completed_at = $1, updated_at = now()
		WHERE number = $2
	`, t, orderNumber)
	return err
}

// ListHistory retrieves the status change history for an order.
func (r *OrdersRepo) ListHistory(ctx context.Context, orderNumber string) ([]orders.StatusLog, error) {
	tx, err := MustTxFromContext(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := tx.Query(ctx, `
		SELECT status, changed_by, changed_at, notes
		FROM order_status_log
		WHERE order_id = (SELECT id FROM orders WHERE number = $1)
		ORDER BY changed_at ASC
	`, orderNumber)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var history []orders.StatusLog
	for rows.Next() {
		var log orders.StatusLog
		err = rows.Scan(&log.Status, &log.ChangedBy, &log.ChangedAt, &log.Notes)
		if err != nil {
			return nil, err
		}
		history = append(history, log)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return history, nil
}
