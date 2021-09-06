package queue

import (
	"context"
	"plexobject.com/formicary/internal/types"
	"sync"
)

// StubClientImpl structure
type StubClientImpl struct {
	SentMessagesByTopic map[string][]MessageEvent      // topic => list of events
	SubscribersByTopic  map[string]map[string]Callback // topic => [subscriber-id => callback]
	lock                sync.RWMutex
}

// NewStubClient stub implementation of queue
func NewStubClient(_ *types.CommonConfig) Client {
	return &StubClientImpl{
		SentMessagesByTopic: make(map[string][]MessageEvent),
		SubscribersByTopic:  make(map[string]map[string]Callback),
	}
}

// Subscribe a consumer
func (c *StubClientImpl) Subscribe(
	_ context.Context,
	topic string,
	id string,
	_ map[string]string,
	_ bool,
	cb Callback) (err error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	cbs := c.SubscribersByTopic[topic]
	if cbs == nil {
		cbs = make(map[string]Callback, 0)
	}
	cbs[id] = cb
	c.SubscribersByTopic[topic] = cbs
	return
}

// UnSubscribe a consumer
func (c *StubClientImpl) UnSubscribe(_ context.Context, topic string, id string) (err error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	cbs := c.SubscribersByTopic[topic]
	if cbs == nil {
		return
	}
	delete(cbs, id)
	c.SubscribersByTopic[topic] = cbs
	return
}

// Send sends a message
func (c *StubClientImpl) Send(
	ctx context.Context,
	topic string,
	props map[string]string,
	payload []byte,
	_ bool) (messageID []byte, err error) {
	return c.Publish(ctx, topic, props, payload, true)
}

// SendReceive - sends and receives a message
func (c *StubClientImpl) SendReceive(
	ctx context.Context,
	outTopic string,
	props map[string]string,
	payload []byte,
	inTopic string,
) (event *MessageEvent, err error) {
	_, _ = c.Publish(ctx, outTopic, props, payload, true)
	return &MessageEvent{
		Topic:      inTopic,
		Properties: props,
		Payload:    payload,
		Ack: func() {
		},
		Nack: func() {
		},
	}, nil
}

// Publish sends a message to topic
func (c *StubClientImpl) Publish(
	ctx context.Context,
	topic string,
	props map[string]string,
	payload []byte,
	_ bool) (messageID []byte, err error) {
	c.lock.Lock()
	msgs := c.SentMessagesByTopic[topic]
	if msgs == nil {
		msgs = make([]MessageEvent, 0)
	}
	event := MessageEvent{
		Topic:      topic,
		Properties: props,
		Payload:    payload,
		Ack: func() {
		},
		Nack: func() {
		},
	}
	msgs = append(msgs, event)
	c.SentMessagesByTopic[topic] = msgs
	cbs := c.SubscribersByTopic[topic]
	c.lock.Unlock()
	if cbs == nil {
		return
	}
	for _, cb := range cbs {
		_ = cb(ctx, &event)
	}

	return make([]byte, 0), nil
}

// Close closes queue
func (c *StubClientImpl) Close() {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.SentMessagesByTopic = make(map[string][]MessageEvent)
	c.SubscribersByTopic = make(map[string]map[string]Callback)
}
