package notification

type NotificationClient interface {
	SendSMS(to string, message string) error
	SendEmail(to string, subject string, body string) error
}
