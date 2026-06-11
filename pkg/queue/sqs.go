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
