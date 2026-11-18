package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

type Config struct {
	URL          string
	ExchangeName string
	ExchangeType string
}

type RabbitMQAdapter struct {
	conn             *amqp.Connection
	channel          *amqp.Channel
	pubMu            sync.Mutex
	consumersMu      sync.Mutex
	consumerChannels []*amqp.Channel
	cfg              Config
}

func (r *RabbitMQAdapter) ensurePublishChannelLocked() error {
	if r.channel != nil && !r.channel.IsClosed() {
		return nil
	}
	if r.conn == nil || r.conn.IsClosed() {
		return fmt.Errorf("rabbitmq connection is closed")
	}
	ch, err := r.conn.Channel()
	if err != nil {
		return fmt.Errorf("failed to reopen publish channel: %w", err)
	}
	r.channel = ch
	return nil
}

func (r *RabbitMQAdapter) PublishWithHeaders(ctx context.Context, exchange, routingKey string, body []byte, headers amqp.Table) error {
	r.pubMu.Lock()
	defer r.pubMu.Unlock()

	if err := r.ensurePublishChannelLocked(); err != nil {
		return err
	}

	return r.channel.PublishWithContext(ctx,
		exchange,
		routingKey,
		false,
		false,
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Timestamp:    time.Now(),
			Headers:      headers,
			Body:         body,
		})
}

func (r *RabbitMQAdapter) NotifyClose() chan *amqp.Error {
	receiver := make(chan *amqp.Error, 1)
	return r.conn.NotifyClose(receiver)
}

func NewRabbitMQAdapter(cfg Config) (*RabbitMQAdapter, error) {
	var conn *amqp.Connection
	var err error
	maxRetries := 5
	for i := 1; i <= maxRetries; i++ {
		conn, err = amqp.Dial(cfg.URL)
		if err == nil {
			break
		}
		if i == maxRetries {
			return nil, fmt.Errorf("failed to connect to rabbitmq after %d attempts: %w", maxRetries, err)
		}
		time.Sleep(2 * time.Second)
	}

	ch, err := conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("failed to open channel: %w", err)
	}

	if cfg.ExchangeName != "" {
		err = ch.ExchangeDeclare(cfg.ExchangeName, cfg.ExchangeType, true, false, false, false, nil)
		if err != nil {
			return nil, err
		}
	}

	return &RabbitMQAdapter{
		conn:    conn,
		channel: ch,
		cfg:     cfg,
	}, nil
}

func (r *RabbitMQAdapter) Publish(ctx context.Context, routingKey string, payload interface{}) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	return r.PublishWithHeaders(ctx, r.cfg.ExchangeName, routingKey, body, nil)
}

func (r *RabbitMQAdapter) SetupQueue(name string, routingKey string, args amqp.Table) error {
	r.pubMu.Lock()
	defer r.pubMu.Unlock()

	if err := r.ensurePublishChannelLocked(); err != nil {
		return err
	}

	_, err := r.channel.QueueDeclare(name, true, false, false, false, args)
	if err != nil {
		return err
	}

	return r.channel.QueueBind(name, routingKey, r.cfg.ExchangeName, false, nil)
}

func (r *RabbitMQAdapter) SetupExchange(name string, kind string) error {
	r.pubMu.Lock()
	defer r.pubMu.Unlock()

	if err := r.ensurePublishChannelLocked(); err != nil {
		return err
	}

	return r.channel.ExchangeDeclare(name, kind, true, false, false, false, nil)
}

func (r *RabbitMQAdapter) Consume(queueName string) (<-chan amqp.Delivery, error) {
	if r.conn == nil || r.conn.IsClosed() {
		return nil, fmt.Errorf("rabbitmq connection is closed")
	}

	ch, err := r.conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("failed to open consumer channel for %s: %w", queueName, err)
	}

	if err := ch.Qos(
		10,
		0,
		false,
	); err != nil {
		_ = ch.Close()
		return nil, fmt.Errorf("failed to set qos for %s: %w", queueName, err)
	}

	msgs, err := ch.Consume(
		queueName,
		"",
		false,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		_ = ch.Close()
		return nil, err
	}

	r.consumersMu.Lock()
	r.consumerChannels = append(r.consumerChannels, ch)
	r.consumersMu.Unlock()

	return msgs, nil
}

func (r *RabbitMQAdapter) Close() {
	r.consumersMu.Lock()
	for _, ch := range r.consumerChannels {
		if ch != nil && !ch.IsClosed() {
			_ = ch.Close()
		}
	}
	r.consumerChannels = nil
	r.consumersMu.Unlock()

	r.pubMu.Lock()
	if r.channel != nil && !r.channel.IsClosed() {
		_ = r.channel.Close()
	}
	r.pubMu.Unlock()

	if r.conn != nil && !r.conn.IsClosed() {
		_ = r.conn.Close()
	}
}

func (r *RabbitMQAdapter) ReconnectChannel() error {
	r.pubMu.Lock()
	defer r.pubMu.Unlock()

	if r.channel != nil && !r.channel.IsClosed() {
		_ = r.channel.Close()
	}

	ch, err := r.conn.Channel()
	if err != nil {
		return fmt.Errorf("failed to reopen channel: %w", err)
	}

	r.channel = ch
	return nil
}
