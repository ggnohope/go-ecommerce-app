package helper

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
)

// HashToken returns the hex-encoded SHA-256 digest of a token.
// Store this in the DB; compare against it rather than the raw token.
func HashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

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
