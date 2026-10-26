package middleware

import (
	"log"
	"net/http"
	"time"

	"github.com/flowpay/flowpay-backend/internal/ratelimit"
	"github.com/gin-gonic/gin"
)

// Cada WhatsApp puede avisar queued, sent, delivered y read, y el ciclo de
// recordatorios los dispara juntos. 2000 por minuto y por IP deja pasar esa ráfaga.
const (
	twilioWebhookMax    = 2000
	twilioWebhookWindow = time.Minute
)

// TwilioWebhookLimit cuenta los webhooks de Twilio por IP. Pasado el tope responde 429.
// El mismo handler debe usarse en los dos webhooks para que compartan el conteo.
func TwilioWebhookLimit() gin.HandlerFunc {
	lim := ratelimit.New()
	return func(c *gin.Context) {
		ip := c.ClientIP()
		if ip == "" {
			ip = "unknown"
		}
		now := time.Now()
		if lim.Blocked(ip, twilioWebhookMax, twilioWebhookWindow, now) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "demasiadas solicitudes"})
			return
		}
		if _, tripped := lim.Hit(ip, twilioWebhookMax, twilioWebhookWindow, now); tripped {
			log.Printf("[FlowPay WhatsApp] tope de webhooks por IP alcanzado ip=%s", ip)
		}
		c.Next()
	}
}
