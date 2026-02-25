package httpserver

import (
	"crypto/rand"
	"encoding/base64"
)

const passwordAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789"

func generateRandomToken(byteSize int) (string, error) {
	b := make([]byte, byteSize)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(b), nil
}

func generateRandomPassword(length int) (string, error) {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	password := make([]byte, length)
	for idx, raw := range b {
		password[idx] = passwordAlphabet[int(raw)%len(passwordAlphabet)]
	}

	return string(password), nil
}
