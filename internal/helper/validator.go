package helper

import (
	"fmt"
	"regexp"
	"strings"
)

// ValidatePhoneNumber validates phone number format
// Accepts various international formats and commonly used patterns
func ValidatePhoneNumber(phone string) error {
	// Remove common whitespace and separators
	cleaned := strings.TrimSpace(phone)
	if cleaned == "" {
		return fmt.Errorf("phone number is empty")
	}

	// Extract only digits to validate
	digitsOnly := regexp.MustCompile(`[0-9]`).FindAllString(cleaned, -1)
	if len(digitsOnly) < 9 {
		return fmt.Errorf("phone number must have at least 9 digits")
	}
	if len(digitsOnly) > 15 {
		return fmt.Errorf("phone number must have at most 15 digits")
	}

	// Allow flexible format: digits, +, -, space, parentheses only
	// Examples: +1234567890, +1-234-567-8900, +1 (234) 567-8900, 1234567890
	phoneRegex := regexp.MustCompile(`^[\d+\-\s()]+$`)
	if !phoneRegex.MatchString(cleaned) {
		return fmt.Errorf("invalid phone number format: %s", phone)
	}

	return nil
}

// ValidateEmail validates email address format
// Uses a practical regex pattern that covers most email formats
func ValidateEmail(email string) error {
	trimmed := strings.TrimSpace(email)
	if trimmed == "" {
		return fmt.Errorf("email is empty")
	}

	// Practical email regex pattern (RFC 5322 simplified)
	emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
	if !emailRegex.MatchString(trimmed) {
		return fmt.Errorf("invalid email format: %s", email)
	}

	if len(trimmed) > 254 {
		return fmt.Errorf("email is too long (max 254 characters)")
	}

	return nil
}


