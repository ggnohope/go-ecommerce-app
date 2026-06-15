package worker

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/aws/aws-sdk-go/service/sqs"

	"go-ecommerce-app/pkg/queue"
)

// messageReceiver is the subset of *queue.SQSClient the consumer needs.
type messageReceiver interface {
	ReceiveMessages(maxMessages, waitSeconds int64) ([]*sqs.Message, error)
	DeleteMessage(receiptHandle string) error
}

// Consumer long-polls a queue and dispatches each message.
type Consumer struct {
	queue    messageReceiver
	dispatch func(queue.OrderEvent) error
}

func NewConsumer(q messageReceiver, dispatch func(queue.OrderEvent) error) *Consumer {
	return &Consumer{queue: q, dispatch: dispatch}
}

// Run polls until ctx is cancelled (graceful shutdown).
func (c *Consumer) Run(ctx context.Context) {
	slog.Info("worker: started, polling for messages")
	for {
		select {
		case <-ctx.Done():
			slog.Info("worker: shutdown signal received, stopping after current batch")
			return
		default:
			if err := c.processOnce(); err != nil {
				slog.Error("worker: receive failed, will retry", "err", err)
			}
		}
	}
}

// processOnce receives one batch and processes each message. A message is
// deleted ONLY after its handler succeeds; on handler error it is left in the
// queue so SQS redelivers it after the visibility timeout.
func (c *Consumer) processOnce() error {
	msgs, err := c.queue.ReceiveMessages(10, 20)
	if err != nil {
		return err
	}
	for _, m := range msgs {
		var event queue.OrderEvent
		if err := json.Unmarshal([]byte(*m.Body), &event); err != nil {
			// Poison message: it will never parse, so retrying forever is
			// pointless. Delete it (and log) instead of blocking the queue.
			slog.Error("worker: unparseable message body, discarding", "err", err, "body", *m.Body)
			c.deleteQuietly(*m.ReceiptHandle)
			continue
		}
		if err := c.dispatch(event); err != nil {
			slog.Error("worker: handler failed, leaving message for retry",
				"err", err, "type", event.EventType, "order", event.OrderID)
			continue // do NOT delete — let SQS redeliver
		}
		c.deleteQuietly(*m.ReceiptHandle)
	}
	return nil
}

func (c *Consumer) deleteQuietly(receiptHandle string) {
	if err := c.queue.DeleteMessage(receiptHandle); err != nil {
		slog.Error("worker: delete failed (message may be redelivered)", "err", err)
	}
}
