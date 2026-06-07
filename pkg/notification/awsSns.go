package notification

import (
	"fmt"
	"go-ecommerce-app/internal/helper"
	"log"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sns"
)

type AWSSNSNotification struct {
	Client *sns.SNS
}

func NewAWSSNSNotification(region string) (*AWSSNSNotification, error) {
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(region),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create AWS session: %w", err)
	}

	snsClient := sns.New(sess)

	return &AWSSNSNotification{
		Client: snsClient,
	}, nil
}

func (a *AWSSNSNotification) SendSMS(to string, message string) error {
	// Validate phone number before attempting to send
	if err := helper.ValidatePhoneNumber(to); err != nil {
		log.Printf("sms validation error: phone=%s err=%s", to, err.Error())
		return WrappedValidationError(err)
	}

	log.Printf("sms attempt: phone=%s message_len=%d", to, len(message))

	input := &sns.PublishInput{
		Message:     aws.String(message),
		PhoneNumber: aws.String(to),
	}

	_, err := a.Client.Publish(input)
	if err != nil {
		log.Printf("sms publish error: phone=%s err=%s", to, err.Error())
		return WrappedDeliveryError(err)
	}

	log.Printf("sms published: phone=%s", to)
	return nil
}

func (a *AWSSNSNotification) SendEmail(to string, subject string, body string) error {
	return fmt.Errorf("email not supported with AWS SNS notification, use Email client instead")
}
