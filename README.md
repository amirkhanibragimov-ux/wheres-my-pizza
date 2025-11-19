# wheres-my-pizza 🍕

**wheres-my-pizza** is a distributed restaurant order‑management system written in Go.  
It models a real kitchen workflow through four independent services communicating over RabbitMQ, with PostgreSQL as the single source of truth.  
The architecture emphasizes message‑driven design, clean layering (domain → ports → adapters), and structured JSON logging.


## Features

### Order Service (HTTP API)
- Accepts and validates new orders.
- Computes total amount and assigns priority.
- Persists orders, items, and an audit trail.
- Publishes order messages into RabbitMQ.

### Kitchen Worker
- Consumes order messages from RabbitMQ.
- Supports worker specialization (e.g., only `delivery`).
- Performs cooking workflow: `received → cooking → ready`.
- Updates worker statistics and writes status changes to DB.
- Publishes status‑update notifications.

### Tracking Service (HTTP API)
- Read‑only service for:
  - Current order status
  - Order history
  - Worker summary

### Notification Subscriber
- Listens to fanout notifications.
- Prints readable events for each order status update.

### Structured Logging
- All logs are JSON with fields:
  `timestamp`, `service`, `action`, `message`, `request_id`, `error`, `details`.


## Project Structure

```text
cmd/
  orderservice/           
  kitchenworker/          
  trackingservice/        
  notificationservice/    

config/
  config.yaml             

internal/
  app/
    orderservice/
    kitchenworker/
    trackingservice/
    notificationservice/
  cli/
  domain/
    orders/
    workers/
  ports/
  shared/
    config/
    contracts/
    logger/
    postgres/
    rabbitmq/

migrations/
  init.sql                

Makefile                  
LICENSE                   
README.md                 
```


## Getting Started

### Build the binary

```bash
go build -o restaurant-system .
```

### Run the services

```bash
./restaurant-system --mode=order-service --port=3000
./restaurant-system --mode=kitchen-worker --worker-name="chef_anna"
./restaurant-system --mode=tracking-service --port=3002
./restaurant-system --mode=notification-subscriber
```

Or use Makefile shortcuts:

```bash
make orderservice
make kitchenworker1
make trackingservice
make notificationservice
```


## API Overview

### Create Order — `POST /orders`

```json
{
  "customer_name": "John Doe",
  "order_type": "delivery",
  "delivery_address": "742 Evergreen St",
  "items": [
    { "name": "Margherita", "quantity": 2, "price": 12.50 }
  ]
}
```

### Tracking (simplified)

- `GET /orders/{order_number}` — current status  
- `GET /orders/{order_number}/history` — audit log  
- `GET /workers` — worker statuses  


## Database

The schema is defined in `migrations/init.sql` and includes:

- `orders`
- `order_items`
- `order_status_log`
- `workers`


## License

MIT License — see `LICENSE`.
