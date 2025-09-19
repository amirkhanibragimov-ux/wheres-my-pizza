package rabbitmq

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"git.platform.alem.school/amibragim/wheres-my-pizza/internal/shared/config"
	"git.platform.alem.school/amibragim/wheres-my-pizza/internal/shared/logger"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Client is a resilient RabbitMQ connector with auto-reconnect and topology setup.
type Client struct {
	url    string
	logger *logger.Logger
	logCtx context.Context // carries context with request_id across reconnects

	mu      sync.RWMutex
	conn    *amqp.Connection
	pubChan *amqp.Channel

	closed    chan struct{}
	reconnect chan struct{}
}

// MQPublisher is a simple RabbitMQ publisher using the Client.
type MQPublisher struct {
	Client *Client
}

// Publish sends a message to the specified RabbitMQ exchange and routing key.
func (p *MQPublisher) Publish(exchange, routingKey string, body []byte, priority uint8) error {
	return p.Client.PublishMessage(exchange, routingKey, body, priority)
}

// ConnectRabbitMQ establishes connection and starts a background watcher that reconnects on failures.
func ConnectRabbitMQ(ctx context.Context, cfg *config.Config, log *logger.Logger) (*Client, error) {
	url := fmt.Sprintf("amqp://%s:%s@%s:%d/",
		cfg.RabbitMQ.User, cfg.RabbitMQ.Password, cfg.RabbitMQ.Host, cfg.RabbitMQ.Port)

	client := &Client{
		url:       url,
		logger:    log,
		logCtx:    context.WithoutCancel(ctx), // avoid ctx cancel on reconnects
		closed:    make(chan struct{}),
		reconnect: make(chan struct{}, 1),
	}

	// initial connect (single attempt; further retries happen in the watcher)
	if err := client.connectOnce(ctx); err != nil {
		return nil, err
	}

	// background watcher for reconnects
	go client.watch()

	return client, nil
}

// NewConsumerChannel returns a fresh channel with prefetch (QoS) applied.
func (client *Client) NewConsumerChannel(prefetch int) (*amqp.Channel, error) {
	client.mu.RLock()
	conn := client.conn
	client.mu.RUnlock()

	// quick fail if no connection
	if conn == nil || conn.IsClosed() {
		return nil, errors.New("rabbitmq: connection is not ready")
	}

	// open a new channel
	ch, err := conn.Channel()
	if err != nil {
		return nil, err
	}

	// set prefetch if requested
	if prefetch > 0 {
		if err := ch.Qos(prefetch, 0, false); err != nil {
			ch.Close()
			return nil, err
		}
	}

	return ch, nil
}

// PublishMessage publishes JSON messages with persistence and AMQP priority.
func (client *Client) PublishMessage(exchange, routingKey string, body []byte, priority uint8) error {
	client.mu.RLock()
	ch := client.pubChan
	conn := client.conn
	client.mu.RUnlock()

	// quick fail if no channel
	if conn == nil || conn.IsClosed() {
		return errors.New("rabbitmq: connection is not open")
	}
	if ch == nil || ch.IsClosed() {
		return errors.New("rabbitmq: publish channel is not open")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return ch.PublishWithContext(ctx,
		exchange, routingKey, false, false,
		amqp.Publishing{
			DeliveryMode: amqp.Persistent, // durable per spec
			Priority:     priority,        // 1,5,10 supported by queue x-max-priority
			ContentType:  "application/json",
			Body:         body,
		})
}

// Ping checks connectivity by dialing TCP to the RabbitMQ.
func (client *Client) Ping(timeout time.Duration) error {
	// grab conn under lock
	client.mu.RLock()
	conn := client.conn
	url := client.url
	client.mu.RUnlock()

	// quick fail if no connection
	if conn == nil || conn.IsClosed() {
		return errors.New("rabbitmq: no connection")
	}

	// parse URL to extract host:port for TCP dial
	u, err := amqp.ParseURI(url)
	if err != nil {
		return fmt.Errorf("rabbitmq: bad url: %w", err)
	}
	addr := net.JoinHostPort(u.Host, fmt.Sprintf("%d", u.Port))

	// dial TCP to the RabbitMQ host:port to verify connectivity
	d := net.Dialer{Timeout: timeout}
	c, err := d.Dial("tcp", addr)
	if err != nil {
		return err
	}

	_ = c.Close()
	return nil
}

// Close gracefully stops the watcher and closes AMQP resources.
func (client *Client) Close() {
	select {
	case <-client.closed:
		// already closed
	default:
		close(client.closed)
	}

	client.mu.Lock()
	if client.pubChan != nil {
		_ = client.pubChan.Close()
		client.pubChan = nil
	}
	if client.conn != nil {
		_ = client.conn.Close()
		client.conn = nil
	}
	client.mu.Unlock()
}

// --- internals ---

// connectOnce tries to connect and set up topology once.
func (client *Client) connectOnce(ctx context.Context) error {
	start := time.Now().UTC()

	// use DialConfig to set heartbeat and TCP dial timeout
	conn, err := amqp.DialConfig(client.url, amqp.Config{
		Heartbeat: 10 * time.Second,
		Locale:    "en_US",
		Dial:      amqp.DefaultDial(10 * time.Second),
	})
	if err != nil {
		return err
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return err
	}

	// declare/ensure topology idempotently
	if err := declareTopology(ch); err != nil {
		ch.Close()
		conn.Close()
		return err
	}

	client.mu.Lock()
	client.conn = conn
	if client.pubChan != nil {
		_ = client.pubChan.Close()
	}
	client.pubChan = ch
	client.mu.Unlock()

	// watch for connection/channel closures and trigger reconnect
	go func() {
		// Either the connection or the publisher channel closing should trigger reconnect
		connClosed := conn.NotifyClose(make(chan *amqp.Error, 1))
		chClosed := ch.NotifyClose(make(chan *amqp.Error, 1))
		select {
		case <-client.closed:
			return
		case <-connClosed:
		case <-chClosed:
		}

		// Try to enqueue a reconnect signal
		select {
		case client.reconnect <- struct{}{}:
		default:
			// already enqueued; no-op
		}
	}()

	client.logger.Info(ctx, "rabbitmq_connected",
		"Connected to RabbitMQ; exchanges: orders_topic, notifications_fanout",
		map[string]any{"duration_ms": time.Since(start).Milliseconds()})

	return nil
}

// watch runs in background and attempts reconnects with exponential backoff.
func (client *Client) watch() {
	// reconnect loop with exponential backoff
	backoff := time.Second
	for {
		select {
		case <-client.closed:
			return
		case <-client.reconnect:
			// attempt reconnect until success or Close()
			for {
				select {
				case <-client.closed:
					return
				default:
				}

				ctx, cancel := context.WithTimeout(client.logCtx, 30*time.Second)
				err := client.connectOnce(ctx)
				cancel()

				if err == nil {
					// reset backoff on success
					backoff = time.Second
					client.logger.Info(client.logCtx, "rabbitmq_reconnected", "Reconnected to RabbitMQ and re-ensured topology", nil)
					break
				}

				// log retry attempt and sleep with backoff
				client.logger.Error(client.logCtx, "retry_attempted", fmt.Sprintf("RabbitMQ reconnect failed: %v", err), err)

				// cap the backoff
				time.Sleep(backoff)
				if backoff < 30*time.Second {
					backoff *= 2
					if backoff > 30*time.Second {
						backoff = 30 * time.Second
					}
				}
			}
		}
	}
}

// declareTopology declares exchanges, queues, and bindings.
func declareTopology(ch *amqp.Channel) error {
	// exchanges
	if err := ch.ExchangeDeclare("orders_topic", "topic", true, false, false, false, nil); err != nil {
		return err
	}
	if err := ch.ExchangeDeclare("notifications_fanout", "fanout", true, false, false, false, nil); err != nil {
		return err
	}

	// main kitchen queue: durable, supports priority, dead-letters to DLX
	_, err := ch.QueueDeclare(
		"kitchen_queue", true, false, false, false, amqp.Table{
			"x-dead-letter-exchange": "orders_topic_dlx",
			"x-max-priority":         int32(10), // enable AMQP priority (1..10)
		},
	)
	if err != nil {
		return err
	}
	if err := ch.QueueBind("kitchen_queue", "kitchen.*.*", "orders_topic", false, nil); err != nil {
		return err
	}

	// DLX + DLQ
	if err := ch.ExchangeDeclare("orders_topic_dlx", "topic", true, false, false, false, nil); err != nil {
		return err
	}
	if _, err := ch.QueueDeclare("kitchen_dlx_queue", true, false, false, false, nil); err != nil {
		return err
	}
	if err := ch.QueueBind("kitchen_dlx_queue", "#", "orders_topic_dlx", false, nil); err != nil {
		return err
	}

	// notifications queue bound to fanout
	if _, err := ch.QueueDeclare("notifications_queue", true, false, false, false, nil); err != nil {
		return err
	}
	if err := ch.QueueBind("notifications_queue", "", "notifications_fanout", false, nil); err != nil {
		return err
	}

	return nil
}
