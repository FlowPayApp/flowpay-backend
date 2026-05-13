package paymenttoken

import (
	"crypto/rand"
	"encoding/base64"
)

const opaqueBytes = 32

// NewOpaque genera un valor de token impredecible (256 bits), codificado URL-safe sin padding.
func NewOpaque() (string, error) {
	b := make([]byte, opaqueBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
