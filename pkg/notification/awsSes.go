package notification

import (
	"fmt"
	"go-ecommerce-app/internal/helper"
	"log"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ses"
)

type AWSEmailNotification struct {
	Client    *ses.SES
	FromEmail string
}

func NewAWSEmailNotification(region, fromEmail string) (*AWSEmailNotification, error) {
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(region),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create AWS session: %w", err)
	}

	sesClient := ses.New(sess)

	return &AWSEmailNotification{
		Client:    sesClient,
		FromEmail: fromEmail,
	}, nil
}

func (a *AWSEmailNotification) SendSMS(to string, message string) error {
	return fmt.Errorf("SMS not supported with AWS Email notification")
}

func (a *AWSEmailNotification) SendEmail(to string, subject string, body string) error {
	// Validate email address before attempting to send
	if err := helper.ValidateEmail(to); err != nil {
		log.Printf("email validation error: to=%s err=%s", to, err.Error())
		return WrappedValidationError(err)
	}

	// Validate sender email
	if err := helper.ValidateEmail(a.FromEmail); err != nil {
		log.Printf("email validation error: from=%s err=%s", a.FromEmail, err.Error())
		return WrappedValidationError(err)
	}

	log.Printf("email attempt: to=%s from=%s subject=%s", to, a.FromEmail, subject)

	input := &ses.SendEmailInput{
		Source: aws.String(a.FromEmail),
		Destination: &ses.Destination{
			ToAddresses: []*string{aws.String(to)},
		},
		Message: &ses.Message{
			Subject: &ses.Content{
				Data: aws.String(subject),
			},
			Body: &ses.Body{
				Html: &ses.Content{
					Data: aws.String(body),
				},
			},
		},
	}

	_, err := a.Client.SendEmail(input)
	if err != nil {
		log.Printf("email send error: to=%s err=%s", to, err.Error())
		return WrappedDeliveryError(err)
	}

	log.Printf("email sent: to=%s", to)
	return nil
}

