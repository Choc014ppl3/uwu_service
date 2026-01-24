package client

import (
	"context"
	"encoding/json"

	"cloud.google.com/go/pubsub"
)

// PubSubClient wraps the Google Cloud Pub/Sub client.
type PubSubClient struct {
	client       *pubsub.Client
	topic        *pubsub.Topic
	subscription *pubsub.Subscription
}

// NewPubSubClient creates a new Pub/Sub client.
func NewPubSubClient(ctx context.Context, projectID, topicID string) (*PubSubClient, error) {
	client, err := pubsub.NewClient(ctx, projectID)
	if err != nil {
		return nil, err
	}

	topic := client.Topic(topicID)

	return &PubSubClient{
		client: client,
		topic:  topic,
	}, nil
}

// WithSubscription sets the subscription to use for receiving messages.
func (c *PubSubClient) WithSubscription(subscriptionID string) *PubSubClient {
	c.subscription = c.client.Subscription(subscriptionID)
	return c
}

// Close closes the client.
func (c *PubSubClient) Close() {
	if c.topic != nil {
		c.topic.Stop()
	}
	if c.client != nil {
		c.client.Close()
	}
}

// Publish publishes a message to the topic.
func (c *PubSubClient) Publish(ctx context.Context, data interface{}) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	result := c.topic.Publish(ctx, &pubsub.Message{
		Data: jsonData,
	})

	// Wait for the result
	_, err = result.Get(ctx)
	return err
}

// PublishWithAttributes publishes a message with attributes.
func (c *PubSubClient) PublishWithAttributes(ctx context.Context, data interface{}, attrs map[string]string) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	result := c.topic.Publish(ctx, &pubsub.Message{
		Data:       jsonData,
		Attributes: attrs,
	})

	_, err = result.Get(ctx)
	return err
}

// PublishAsync publishes a message asynchronously without waiting for the result.
func (c *PubSubClient) PublishAsync(ctx context.Context, data interface{}) {
	jsonData, _ := json.Marshal(data)
	c.topic.Publish(ctx, &pubsub.Message{
		Data: jsonData,
	})
}

// MessageHandler is a function that handles received messages.
type MessageHandler func(ctx context.Context, msg *pubsub.Message) error

// Subscribe starts receiving messages from the subscription.
func (c *PubSubClient) Subscribe(ctx context.Context, handler MessageHandler) error {
	if c.subscription == nil {
		return nil
	}

	return c.subscription.Receive(ctx, func(ctx context.Context, msg *pubsub.Message) {
		if err := handler(ctx, msg); err != nil {
			msg.Nack()
			return
		}
		msg.Ack()
	})
}

// SubscribeJSON starts receiving messages and unmarshals them to the provided type.
func (c *PubSubClient) SubscribeJSON(ctx context.Context, handler func(ctx context.Context, data json.RawMessage, attrs map[string]string) error) error {
	return c.Subscribe(ctx, func(ctx context.Context, msg *pubsub.Message) error {
		return handler(ctx, msg.Data, msg.Attributes)
	})
}
