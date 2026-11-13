package queue

import (
	"chatnip/internal/kit/queue"
	"context"
	"fmt"
	amqp "github.com/rabbitmq/amqp091-go"
)

type WhatsAppEventProducer struct {
	adapter *queue.RabbitMQAdapter
}

func NewWhatsAppProducer(adapter *queue.RabbitMQAdapter) *WhatsAppEventProducer {

	dlqArgs := amqp.Table{
		"x-dead-letter-exchange":    "whatsapp_dlx",
		"x-dead-letter-routing-key": "msg.failed",
	}

	if err := adapter.SetupExchange("whatsapp_dlx", "direct"); err != nil {
		panic(err)
	}

	if err := adapter.SetupExchange("whatsapp_events", "topic"); err != nil {
		panic(fmt.Errorf("failed to setup exchange whatsapp_events: %w", err))
	}

	if err := adapter.SetupQueue("whatsapp_fast_lane", "msg.send.priority", dlqArgs); err != nil {
		panic(fmt.Errorf("failed to setup queue whatsapp_fast_lane: %w", err))
	}

	if err := adapter.SetupQueue("whatsapp_standard_lane", "msg.send.normal", dlqArgs); err != nil {
		panic(fmt.Errorf("failed to setup queue whatsapp_standard_lane: %w", err))
	}

	if err := adapter.SetupQueue("whatsapp_incoming_messages", "msg.receive", nil); err != nil {
		panic(fmt.Errorf("failed to setup queue whatsapp_incoming_messages: %w", err))
	}

	if err := adapter.SetupQueue("whatsapp_group_create", "group.create", dlqArgs); err != nil {
		panic(fmt.Errorf("failed to setup queue whatsapp_group_create: %w", err))
	}

	return &WhatsAppEventProducer{adapter: adapter}
}

func (p *WhatsAppEventProducer) Publish(ctx context.Context, routingKey string, payload interface{}) error {
	return p.adapter.Publish(ctx, routingKey, payload)
}
