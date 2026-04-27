package twiliovalidate

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"net/url"
	"sort"
	"strings"
)

// RequestValidator valida cabecera X-Twilio-Signature (POST application/x-www-form-urlencoded).
// Ver https://www.twilio.com/docs/usage/security#validating-requests
type RequestValidator struct {
	AuthToken string
}

// Validate compara la firma de Twilio con el cuerpo recibido.
// fullURL debe ser la URL pública exacta configurada en Twilio (esquema + host + path, sin query opcional según consola).
func (v RequestValidator) Validate(signature string, fullURL string, form url.Values) bool {
	if strings.TrimSpace(v.AuthToken) == "" || strings.TrimSpace(signature) == "" {
		return false
	}
	keys := make([]string, 0, len(form))
	for k := range form {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString(fullURL)
	for _, k := range keys {
		b.WriteString(k)
		for _, val := range form[k] {
			b.WriteString(val)
		}
	}
	mac := hmac.New(sha1.New, []byte(v.AuthToken))
	_, _ = mac.Write([]byte(b.String()))
	expected := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}
