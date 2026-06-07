package helper

import (
	"crypto/rand"
)

func RandomNumber(length int) (string, error) {
	const numbers = "0123456789"

	buffer := make([]byte, length)
	_, err := rand.Read(buffer)
	if err != nil {
		return "", err
	}

	for i := 0; i < length; i++ {
		buffer[i] = numbers[int(buffer[i])%len(numbers)]
	}

	return string(buffer), nil
}
