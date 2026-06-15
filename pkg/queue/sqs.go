package queue

import (
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"
)

type EventType string

const (
	EventOrderPlaced  EventType = "ORDER_PLACED"
	EventOrderPaid    EventType = "ORDER_PAID"
	EventOrderShipped EventType = "ORDER_SHIPPED"
)

type OrderEvent struct {
	EventType EventType `json:"event_type"`
	OrderID   uint      `json:"order_id"`
	UserID    uint      `json:"user_id"`
	Amount    float64   `json:"amount"`
}

type SQSClient struct {
	client   *sqs.SQS
	queueURL string
}

func NewSQSClient(region, queueURL string) (*SQSClient, error) {
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(region),
	})
	if err != nil {
		return nil, fmt.Errorf("sqs: failed to create session: %w", err)
	}
	return &SQSClient{
		client:   sqs.New(sess),
		queueURL: queueURL,
	}, nil
}

func (q *SQSClient) PublishOrderEvent(event OrderEvent) error {
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("sqs: marshal error: %w", err)
	}
	_, err = q.client.SendMessage(&sqs.SendMessageInput{
		QueueUrl:    aws.String(q.queueURL),
		MessageBody: aws.String(string(body)),
		MessageAttributes: map[string]*sqs.MessageAttributeValue{
			"event_type": {
				DataType:    aws.String("String"),
				StringValue: aws.String(string(event.EventType)),
			},
		},
	})
	if err != nil {
		return fmt.Errorf("sqs: send failed: %w", err)
	}
	return nil
}

// ReceiveMessages long-polls the queue, returning up to maxMessages messages.
// waitSeconds enables long polling (0 = short polling). Message attributes are
// requested so consumers can read the event_type the producer attached.
func (q *SQSClient) ReceiveMessages(maxMessages, waitSeconds int64) ([]*sqs.Message, error) {
	out, err := q.client.ReceiveMessage(&sqs.ReceiveMessageInput{
		QueueUrl:              aws.String(q.queueURL),
		MaxNumberOfMessages:   aws.Int64(maxMessages),
		WaitTimeSeconds:       aws.Int64(waitSeconds),
		MessageAttributeNames: aws.StringSlice([]string{"All"}),
	})
	if err != nil {
		return nil, fmt.Errorf("sqs: receive failed: %w", err)
	}
	return out.Messages, nil
}

// DeleteMessage removes a message from the queue after successful processing.
// It uses the per-receive ReceiptHandle, NOT the message ID.
func (q *SQSClient) DeleteMessage(receiptHandle string) error {
	_, err := q.client.DeleteMessage(&sqs.DeleteMessageInput{
		QueueUrl:      aws.String(q.queueURL),
		ReceiptHandle: aws.String(receiptHandle),
	})
	if err != nil {
		return fmt.Errorf("sqs: delete failed: %w", err)
	}
	return nil
}
