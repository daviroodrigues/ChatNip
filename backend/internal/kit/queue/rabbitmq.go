package queue

import (
	"context"
	"encoding/json"
	"fmt"
	amqp "github.com/rabbitmq/amqp091-go"
	"time"
)

type Config struct {
	URL          string
	ExchangeName string
	ExchangeType string
}

type RabbitMQAdapter struct {
	conn    *amqp.Connection
	channel *amqp.Channel
	cfg     Config
}

func (r *RabbitMQAdapter) PublishWithHeaders(ctx context.Context, exchange, routingKey string, body []byte, headers amqp.Table) error {
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
	receiver := make(chan *amqp.Error)
	return r.channel.NotifyClose(receiver)
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

	return r.channel.PublishWithContext(ctx,
		r.cfg.ExchangeName,
		routingKey,
		false,
		false,
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Timestamp:    time.Now(),
			Body:         body,
		})
}

func (r *RabbitMQAdapter) SetupQueue(name string, routingKey string, args amqp.Table) error {
	_, err := r.channel.QueueDeclare(name, true, false, false, false, args)
	if err != nil {
		return err
	}

	return r.channel.QueueBind(name, routingKey, r.cfg.ExchangeName, false, nil)
}

func (r *RabbitMQAdapter) SetupExchange(name string, kind string) error {
	return r.channel.ExchangeDeclare(name, kind, true, false, false, false, nil)
}

func (r *RabbitMQAdapter) Consume(queueName string) (<-chan amqp.Delivery, error) {
	if err := r.channel.Qos(
		10,
		0,
		false,
	); err != nil {
		return nil, fmt.Errorf("failed to set qos: %w", err)
	}

	return r.channel.Consume(
		queueName,
		"",
		false,
		false,
		false,
		false,
		nil,
	)
}

func (r *RabbitMQAdapter) Close() {
	if r.channel != nil {
		r.channel.Close()
	}
	if r.conn != nil {
		r.conn.Close()
	}
}

func (r *RabbitMQAdapter) ReconnectChannel() error {
	if r.channel != nil {
		r.channel.Close()
	}

	ch, err := r.conn.Channel()
	if err != nil {
		return fmt.Errorf("failed to reopen channel: %w", err)
	}

	r.channel = ch
	return nil
}
