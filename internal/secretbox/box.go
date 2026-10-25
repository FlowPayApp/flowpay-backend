// Package secretbox cifra secretos en reposo con AES-256-GCM.
// El formato es v1.<base64(nonce || ciphertext)>. El AAD ata el texto a una
// empresa y una columna, así un valor copiado a otra fila no descifra.
// FLOWPAY_PAYMENT_CREDENTIALS_KEY tiene que ser la misma en el servicio que
// más adelante cobre con estas credenciales.
package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"strings"
)

const versionPrefix = "v1."

// ErrKeyMaterial la clave de aplicación no alcanza para cifrar.
var ErrKeyMaterial = errors.New("la clave de cifrado debe tener al menos 32 caracteres")

// KeyFromMaterial deriva una clave de 32 bytes. No registra el material.
func KeyFromMaterial(material string) ([]byte, error) {
	material = strings.TrimSpace(material)
	if len(material) < 32 {
		return nil, ErrKeyMaterial
	}
	sum := sha256.Sum256([]byte(material))
	return sum[:], nil
}

// Seal cifra plain. Limpia plain antes de volver.
func Seal(key, plain, aad []byte) (string, error) {
	defer wipe(plain)
	if len(key) != 32 {
		return "", errors.New("clave de cifrado inválida")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	out := gcm.Seal(nonce, nonce, plain, aad)
	return versionPrefix + base64.RawStdEncoding.EncodeToString(out), nil
}

// Open descifra un valor producido por Seal. El AAD tiene que ser el mismo.
func Open(key []byte, sealed string, aad []byte) (string, error) {
	if len(key) != 32 {
		return "", errors.New("clave de cifrado inválida")
	}
	payload, ok := strings.CutPrefix(sealed, versionPrefix)
	if !ok || payload == "" {
		return "", errors.New("secreto ilegible")
	}
	raw, err := base64.RawStdEncoding.DecodeString(payload)
	if err != nil {
		return "", errors.New("secreto ilegible")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("secreto ilegible")
	}
	nonce, ciphertext := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return "", errors.New("secreto ilegible")
	}
	defer wipe(plain)
	return string(plain), nil
}

func wipe(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
