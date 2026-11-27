package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

type Config struct {
	URL          string
	ExchangeName string
	ExchangeType string
}

type amqpChannel interface {
	IsClosed() bool
	Close() error
	PublishWithContext(ctx context.Context, exchange, key string, mandatory, immediate bool, msg amqp.Publishing) error
	QueueDeclare(name string, durable, autoDelete, exclusive, noWait bool, args amqp.Table) (amqp.Queue, error)
	QueueDeclarePassive(name string, durable, autoDelete, exclusive, noWait bool, args amqp.Table) (amqp.Queue, error)
	QueueBind(name, key, exchange string, noWait bool, args amqp.Table) error
	ExchangeDeclare(name, kind string, durable, autoDelete, internal, noWait bool, args amqp.Table) error
	ExchangeDeclarePassive(name, kind string, durable, autoDelete, internal, noWait bool, args amqp.Table) error
}

type RabbitMQAdapter struct {
	conn             *amqp.Connection
	channel          amqpChannel
	openChannel      func() (amqpChannel, error)
	pubMu            sync.Mutex
	consumersMu      sync.Mutex
	consumerChannels []*amqp.Channel
	cfg              Config
}

func isAMQPCode(err error, code int) bool {
	var amqpErr *amqp.Error
	if errors.As(err, &amqpErr) {
		return amqpErr.Code == code
	}
	return false
}

func (r *RabbitMQAdapter) newChannel() (amqpChannel, error) {
	if r.openChannel != nil {
		return r.openChannel()
	}
	if r.conn == nil || r.conn.IsClosed() {
		return nil, fmt.Errorf("rabbitmq connection is closed")
	}
	return r.conn.Channel()
}

func (r *RabbitMQAdapter) ensurePublishChannelLocked() error {
	if r.channel != nil && !r.channel.IsClosed() {
		return nil
	}
	ch, err := r.newChannel()
	if err != nil {
		return fmt.Errorf("failed to reopen publish channel: %w", err)
	}
	r.channel = ch
	return nil
}

func (r *RabbitMQAdapter) probeQueueExists(name string) (bool, error) {
	probeCh, err := r.newChannel()
	if err != nil {
		return false, fmt.Errorf("failed to open probe channel for queue %s: %w", name, err)
	}
	defer func() {
		if !probeCh.IsClosed() {
			_ = probeCh.Close()
		}
	}()

	_, err = probeCh.QueueDeclarePassive(name, true, false, false, false, nil)
	if err == nil {
		return true, nil
	}
	if isAMQPCode(err, amqp.NotFound) {
		return false, nil
	}
	return false, fmt.Errorf("failed to inspect queue %s: %w", name, err)
}

func (r *RabbitMQAdapter) probeExchangeExists(name, kind string) (bool, error) {
	probeCh, err := r.newChannel()
	if err != nil {
		return false, fmt.Errorf("failed to open probe channel for exchange %s: %w", name, err)
	}
	defer func() {
		if !probeCh.IsClosed() {
			_ = probeCh.Close()
		}
	}()

	err = probeCh.ExchangeDeclarePassive(name, kind, true, false, false, false, nil)
	if err == nil {
		return true, nil
	}
	if isAMQPCode(err, amqp.NotFound) {
		return false, nil
	}
	return false, fmt.Errorf("failed to inspect exchange %s: %w", name, err)
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
		_ = conn.Close()
		return nil, fmt.Errorf("failed to open channel: %w", err)
	}

	adapter := &RabbitMQAdapter{
		conn:    conn,
		channel: ch,
		cfg:     cfg,
	}

	if cfg.ExchangeName != "" {
		if err := adapter.SetupExchange(cfg.ExchangeName, cfg.ExchangeType); err != nil {
			adapter.Close()
			return nil, err
		}
	}

	return adapter, nil
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

	exists, err := r.probeQueueExists(name)
	if err != nil {
		return err
	}

	if !exists {
		if err := r.ensurePublishChannelLocked(); err != nil {
			return err
		}

		_, err = r.channel.QueueDeclare(name, true, false, false, false, args)
		if err != nil {
			if isAMQPCode(err, amqp.PreconditionFailed) {
				// 406 closes the AMQP channel; reset it and reuse the queue if it already exists.
				r.channel = nil
				existsNow, probeErr := r.probeQueueExists(name)
				if probeErr != nil || !existsNow {
					return err
				}
				log.Printf("rabbitmq: queue %q already exists with different arguments; reusing existing queue", name)
			} else {
				return err
			}
		}
	}

	if routingKey == "" || r.cfg.ExchangeName == "" {
		return nil
	}

	if err := r.ensurePublishChannelLocked(); err != nil {
		return err
	}

	return r.channel.QueueBind(name, routingKey, r.cfg.ExchangeName, false, nil)
}

func (r *RabbitMQAdapter) SetupExchange(name string, kind string) error {
	r.pubMu.Lock()
	defer r.pubMu.Unlock()

	exists, err := r.probeExchangeExists(name, kind)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	if err := r.ensurePublishChannelLocked(); err != nil {
		return err
	}

	err = r.channel.ExchangeDeclare(name, kind, true, false, false, false, nil)
	if err != nil {
		if isAMQPCode(err, amqp.PreconditionFailed) {
			r.channel = nil
			existsNow, probeErr := r.probeExchangeExists(name, kind)
			if probeErr != nil || !existsNow {
				return err
			}
			log.Printf("rabbitmq: exchange %q already exists with different settings; reusing existing exchange", name)
			return nil
		}
		return err
	}

	return nil
}

func (r *RabbitMQAdapter) Consume(queueName string) (<-chan amqp.Delivery, error) {
	return r.ConsumeWithPrefetch(queueName, 10)
}

func (r *RabbitMQAdapter) ConsumeWithPrefetch(queueName string, prefetchCount int) (<-chan amqp.Delivery, error) {
	if r.conn == nil || r.conn.IsClosed() {
		return nil, fmt.Errorf("rabbitmq connection is closed")
	}
	if prefetchCount <= 0 {
		prefetchCount = 1
	}

	ch, err := r.conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("failed to open consumer channel for %s: %w", queueName, err)
	}

	if err := ch.Qos(
		prefetchCount,
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

	ch, err := r.newChannel()
	if err != nil {
		return fmt.Errorf("failed to reopen channel: %w", err)
	}

	r.channel = ch
	return nil
}
