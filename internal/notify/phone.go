package notify

import (
	"fmt"
	"regexp"
	"strings"
)

var digitsOnly = regexp.MustCompile(`\D+`)

// NormalizeWhatsAppForTwilio devuelve formato Twilio: whatsapp:+56912345678
func NormalizeWhatsAppForTwilio(raw string) string {
	s := strings.TrimSpace(raw)
	if strings.HasPrefix(strings.ToLower(s), "whatsapp:") {
		return s
	}
	d := digitsOnly.ReplaceAllString(s, "")
	if d == "" {
		return ""
	}
	if strings.HasPrefix(d, "56") {
		return "whatsapp:+" + d
	}
	if strings.HasPrefix(d, "9") && len(d) >= 9 {
		return "whatsapp:+56" + d
	}
	return fmt.Sprintf("whatsapp:+%s", d)
}

func normalizeSMSPhone(raw string) string {
	s := strings.TrimSpace(raw)
	d := digitsOnly.ReplaceAllString(s, "")
	if d == "" {
		return ""
	}
	if strings.HasPrefix(d, "56") {
		return "+" + d
	}
	if strings.HasPrefix(d, "9") && len(d) >= 9 {
		return "+56" + d
	}
	return fmt.Sprintf("+%s", d)
}
