package queue

import (
	"context"
	"errors"
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"
)

type fakeAMQPChannel struct {
	closed bool

	onQueueDeclarePassive    func(name string) (amqp.Queue, error)
	onQueueDeclare           func(name string, args amqp.Table) (amqp.Queue, error)
	onQueueBind              func(name, key, exchange string) error
	onExchangeDeclarePassive func(name, kind string) error
	onExchangeDeclare        func(name, kind string) error

	queueDeclareCalls    int
	queueBindCalls       int
	exchangeDeclareCalls int
}

func (f *fakeAMQPChannel) IsClosed() bool { return f.closed }
func (f *fakeAMQPChannel) Close() error {
	f.closed = true
	return nil
}
func (f *fakeAMQPChannel) PublishWithContext(ctx context.Context, exchange, key string, mandatory, immediate bool, msg amqp.Publishing) error {
	return nil
}
func (f *fakeAMQPChannel) QueueDeclare(name string, durable, autoDelete, exclusive, noWait bool, args amqp.Table) (amqp.Queue, error) {
	f.queueDeclareCalls++
	if f.onQueueDeclare != nil {
		q, err := f.onQueueDeclare(name, args)
		if isAMQPCode(err, amqp.PreconditionFailed) {
			f.closed = true
		}
		return q, err
	}
	return amqp.Queue{Name: name}, nil
}
func (f *fakeAMQPChannel) QueueDeclarePassive(name string, durable, autoDelete, exclusive, noWait bool, args amqp.Table) (amqp.Queue, error) {
	if f.onQueueDeclarePassive != nil {
		q, err := f.onQueueDeclarePassive(name)
		if err != nil {
			f.closed = true
		}
		return q, err
	}
	return amqp.Queue{Name: name}, nil
}
func (f *fakeAMQPChannel) QueueBind(name, key, exchange string, noWait bool, args amqp.Table) error {
	f.queueBindCalls++
	if f.onQueueBind != nil {
		return f.onQueueBind(name, key, exchange)
	}
	return nil
}
func (f *fakeAMQPChannel) ExchangeDeclare(name, kind string, durable, autoDelete, internal, noWait bool, args amqp.Table) error {
	f.exchangeDeclareCalls++
	if f.onExchangeDeclare != nil {
		err := f.onExchangeDeclare(name, kind)
		if isAMQPCode(err, amqp.PreconditionFailed) {
			f.closed = true
		}
		return err
	}
	return nil
}
func (f *fakeAMQPChannel) ExchangeDeclarePassive(name, kind string, durable, autoDelete, internal, noWait bool, args amqp.Table) error {
	if f.onExchangeDeclarePassive != nil {
		err := f.onExchangeDeclarePassive(name, kind)
		if err != nil {
			f.closed = true
		}
		return err
	}
	return nil
}

func TestSetupQueue_ReusesExistingQueueWithoutRedeclare(t *testing.T) {
	mainCh := &fakeAMQPChannel{}
	probeCh := &fakeAMQPChannel{
		onQueueDeclarePassive: func(name string) (amqp.Queue, error) {
			return amqp.Queue{Name: name}, nil
		},
	}

	adapter := &RabbitMQAdapter{
		channel: mainCh,
		cfg:     Config{ExchangeName: "whatsapp_events", ExchangeType: "topic"},
		openChannel: func() (amqpChannel, error) {
			return probeCh, nil
		},
	}

	err := adapter.SetupQueue("whatsapp_fast_lane", "msg.send.priority", amqp.Table{"x-dead-letter-exchange": "whatsapp_dlx"})
	if err != nil {
		t.Fatalf("expected no error reusing existing queue, got %v", err)
	}
	if mainCh.queueDeclareCalls != 0 {
		t.Fatalf("expected QueueDeclare not to be called when queue already exists, got %d calls", mainCh.queueDeclareCalls)
	}
	if mainCh.queueBindCalls != 1 {
		t.Fatalf("expected QueueBind to be called once, got %d", mainCh.queueBindCalls)
	}
	if !probeCh.closed {
		t.Fatal("expected probe channel to be closed after passive check")
	}
}

func TestSetupQueue_CreatesWhenNotFound(t *testing.T) {
	mainCh := &fakeAMQPChannel{}
	probeCh := &fakeAMQPChannel{
		onQueueDeclarePassive: func(name string) (amqp.Queue, error) {
			return amqp.Queue{}, &amqp.Error{Code: amqp.NotFound, Reason: "NOT_FOUND"}
		},
	}

	adapter := &RabbitMQAdapter{
		channel: mainCh,
		cfg:     Config{ExchangeName: "whatsapp_events", ExchangeType: "topic"},
		openChannel: func() (amqpChannel, error) {
			return probeCh, nil
		},
	}

	err := adapter.SetupQueue("whatsapp_fast_lane", "msg.send.priority", nil)
	if err != nil {
		t.Fatalf("expected no error creating missing queue, got %v", err)
	}
	if mainCh.queueDeclareCalls != 1 {
		t.Fatalf("expected QueueDeclare to be called once, got %d", mainCh.queueDeclareCalls)
	}
	if mainCh.queueBindCalls != 1 {
		t.Fatalf("expected QueueBind to be called once, got %d", mainCh.queueBindCalls)
	}
}

func TestSetupQueue_RecoversFromPreconditionFailedRace(t *testing.T) {
	firstMainCh := &fakeAMQPChannel{
		onQueueDeclare: func(name string, args amqp.Table) (amqp.Queue, error) {
			return amqp.Queue{}, &amqp.Error{Code: amqp.PreconditionFailed, Reason: "PRECONDITION_FAILED"}
		},
	}
	recoveredMainCh := &fakeAMQPChannel{}

	probeCalls := 0
	adapter := &RabbitMQAdapter{
		channel: firstMainCh,
		cfg:     Config{ExchangeName: "whatsapp_events", ExchangeType: "topic"},
		openChannel: func() (amqpChannel, error) {
			probeCalls++
			switch probeCalls {
			case 1:
				// Initial passive check: queue does not exist yet
				return &fakeAMQPChannel{
					onQueueDeclarePassive: func(name string) (amqp.Queue, error) {
						return amqp.Queue{}, &amqp.Error{Code: amqp.NotFound, Reason: "NOT_FOUND"}
					},
				}, nil
			case 2:
				// Verification passive check after 406: queue now exists
				return &fakeAMQPChannel{
					onQueueDeclarePassive: func(name string) (amqp.Queue, error) {
						return amqp.Queue{Name: name}, nil
					},
				}, nil
			default:
				// Reopened publish channel for QueueBind
				return recoveredMainCh, nil
			}
		},
	}

	err := adapter.SetupQueue("whatsapp_fast_lane", "msg.send.priority", nil)
	if err != nil {
		t.Fatalf("expected 406 recovery to succeed, got %v", err)
	}
	if recoveredMainCh.queueBindCalls != 1 {
		t.Fatalf("expected QueueBind on recovered channel, got %d", recoveredMainCh.queueBindCalls)
	}
}

func TestSetupQueue_PropagatesUnexpectedProbeError(t *testing.T) {
	mainCh := &fakeAMQPChannel{}
	adapter := &RabbitMQAdapter{
		channel: mainCh,
		cfg:     Config{ExchangeName: "whatsapp_events", ExchangeType: "topic"},
		openChannel: func() (amqpChannel, error) {
			return &fakeAMQPChannel{
				onQueueDeclarePassive: func(name string) (amqp.Queue, error) {
					return amqp.Queue{}, &amqp.Error{Code: amqp.AccessRefused, Reason: "ACCESS_REFUSED"}
				},
			}, nil
		},
	}

	err := adapter.SetupQueue("whatsapp_fast_lane", "msg.send.priority", nil)
	if err == nil {
		t.Fatal("expected error for ACCESS_REFUSED, got nil")
	}
	var amqpErr *amqp.Error
	if !errors.As(err, &amqpErr) || amqpErr.Code != amqp.AccessRefused {
		t.Fatalf("expected ACCESS_REFUSED amqp error, got %v", err)
	}
}

func TestSetupExchange_ReusesExistingExchange(t *testing.T) {
	mainCh := &fakeAMQPChannel{}
	probeCh := &fakeAMQPChannel{
		onExchangeDeclarePassive: func(name, kind string) error {
			return nil
		},
	}

	adapter := &RabbitMQAdapter{
		channel: mainCh,
		openChannel: func() (amqpChannel, error) {
			return probeCh, nil
		},
	}

	if err := adapter.SetupExchange("whatsapp_events", "topic"); err != nil {
		t.Fatalf("expected no error reusing exchange, got %v", err)
	}
	if mainCh.exchangeDeclareCalls != 0 {
		t.Fatalf("expected ExchangeDeclare not to be called when exchange exists, got %d", mainCh.exchangeDeclareCalls)
	}
}

func TestSetupExchange_CreatesWhenNotFound(t *testing.T) {
	mainCh := &fakeAMQPChannel{}
	adapter := &RabbitMQAdapter{
		channel: mainCh,
		openChannel: func() (amqpChannel, error) {
			return &fakeAMQPChannel{
				onExchangeDeclarePassive: func(name, kind string) error {
					return &amqp.Error{Code: amqp.NotFound, Reason: "NOT_FOUND"}
				},
			}, nil
		},
	}

	if err := adapter.SetupExchange("whatsapp_events", "topic"); err != nil {
		t.Fatalf("expected no error creating missing exchange, got %v", err)
	}
	if mainCh.exchangeDeclareCalls != 1 {
		t.Fatalf("expected ExchangeDeclare to be called once, got %d", mainCh.exchangeDeclareCalls)
	}
}
