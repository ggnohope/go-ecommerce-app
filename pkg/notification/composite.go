package notification

type CompositeNotificationClient struct {
	SMSClient   NotificationClient
	EmailClient NotificationClient
}

func NewCompositeNotificationClient(smsClient, emailClient NotificationClient) *CompositeNotificationClient {
	return &CompositeNotificationClient{
		SMSClient:   smsClient,
		EmailClient: emailClient,
	}
}

func (c *CompositeNotificationClient) SendSMS(to string, message string) error {
	return c.SMSClient.SendSMS(to, message)
}

func (c *CompositeNotificationClient) SendEmail(to string, subject string, body string) error {
	return c.EmailClient.SendEmail(to, subject, body)
}

