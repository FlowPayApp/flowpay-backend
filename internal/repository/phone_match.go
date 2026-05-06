package repository

import (
	"regexp"
	"strings"
)

var digitsOnlyMatch = regexp.MustCompile(`\D+`)

// phonesLikelyMatch compara dos teléfonos en formatos distintos (Twilio whatsapp:+, planilla, etc.).
func phonesLikelyMatch(a, b string) bool {
	da := digitsOnlyMatch.ReplaceAllString(strings.TrimSpace(a), "")
	db := digitsOnlyMatch.ReplaceAllString(strings.TrimSpace(b), "")
	if len(da) < 8 || len(db) < 8 {
		return false
	}
	strip56 := func(d string) string {
		if strings.HasPrefix(d, "56") && len(d) >= 11 {
			return d[2:]
		}
		return d
	}
	da, db = strip56(da), strip56(db)
	tail := func(d string) string {
		if len(d) > 9 {
			return d[len(d)-9:]
		}
		return d
	}
	return tail(da) == tail(db)
}
