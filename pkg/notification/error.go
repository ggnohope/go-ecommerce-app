package notification

import "fmt"

// ValidationError represents an error caused by invalid input (validation failure)
// Should be treated as client error (HTTP 400)
type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string {
	return e.Message
}

// IsValidationError checks if an error is a ValidationError
func IsValidationError(err error) bool {
	_, ok := err.(*ValidationError)
	return ok
}

// NewValidationError creates a new ValidationError
func NewValidationError(msg string) error {
	return &ValidationError{Message: msg}
}

// DeliveryError represents an error caused by delivery failure (SMS/Email send failed)
// Should be treated as server error (HTTP 500)
type DeliveryError struct {
	Message string
}

func (e *DeliveryError) Error() string {
	return e.Message
}

// IsDeliveryError checks if an error is a DeliveryError
func IsDeliveryError(err error) bool {
	_, ok := err.(*DeliveryError)
	return ok
}

// NewDeliveryError creates a new DeliveryError
func NewDeliveryError(msg string) error {
	return &DeliveryError{Message: msg}
}

// WrappedValidationError wraps a format error as ValidationError
func WrappedValidationError(err error) error {
	if err == nil {
		return nil
	}
	return NewValidationError(err.Error())
}

// WrappedDeliveryError wraps a send/publish error as DeliveryError
func WrappedDeliveryError(err error) error {
	if err == nil {
		return nil
	}
	return NewDeliveryError(fmt.Sprintf("Failed to deliver message: %s", err.Error()))
}

