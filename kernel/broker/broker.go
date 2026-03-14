package broker

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/AnantSingh1510/agentd/kernel/types"
)

var (
	ErrNotConnected = errors.New("broker not connected")
	ErrEmptySubject = errors.New("subject cannot be empty")
	ErrNilHandler   = errors.New("handler cannot be nil")
)

type Handler func(event *types.Event)

type Broker struct {
	nc   *nats.Conn
	subs []*nats.Subscription
}

func New(url string) (*Broker, error) {
	nc, err := nats.Connect(url,
		nats.Timeout(5*time.Second),
		nats.MaxReconnects(5),
		nats.ReconnectWait(1*time.Second),
	)
	if err != nil {
		return nil, err
	}
	return &Broker{nc: nc, subs: make([]*nats.Subscription, 0)}, nil
}

func (b *Broker) Publish(subject string, payload []byte, from string) error {
	if b.nc == nil || !b.nc.IsConnected() {
		return ErrNotConnected
	}
	if subject == "" {
		return ErrEmptySubject
	}

	event := &types.Event{
		ID:      uuid.NewString(),
		Subject: subject,
		Payload: payload,
		From:    from,
	}

	data, err := marshalEvent(event)
	if err != nil {
		return err
	}

	return b.nc.Publish(subject, data)
}

func (b *Broker) Subscribe(subject string, handler Handler) error {
	if b.nc == nil || !b.nc.IsConnected() {
		return ErrNotConnected
	}
	if subject == "" {
		return ErrEmptySubject
	}
	if handler == nil {
		return ErrNilHandler
	}

	sub, err := b.nc.Subscribe(subject, func(msg *nats.Msg) {
		event, err := unmarshalEvent(msg.Data)
		if err != nil {
			return
		}
		handler(event)
	})
	if err != nil {
		return err
	}

	b.subs = append(b.subs, sub)
	return nil
}

func (b *Broker) Drain() error {
	if b.nc == nil {
		return nil
	}
	return b.nc.Drain()
}

func (b *Broker) Close() {
	if b.nc != nil {
		b.nc.Close()
	}
}
