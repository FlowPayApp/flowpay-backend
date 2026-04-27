package middleware

import (
	"net/http"
	"strings"

	"github.com/flowpay/flowpay-backend/internal/authjwt"
	"github.com/gin-gonic/gin"
)

// BearerJWT exige Authorization: Bearer cuando secret no está vacío.
func BearerJWT(secret string) gin.HandlerFunc {
	if strings.TrimSpace(secret) == "" {
		return func(c *gin.Context) { c.Next() }
	}
	sec := []byte(secret)
	return func(c *gin.Context) {
		h := c.GetHeader("Authorization")
		if h == "" || !strings.HasPrefix(h, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "se requiere Bearer token"})
			return
		}
		raw := strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
		claims, err := authjwt.ParseAccessToken(sec, raw)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "token inválido o expirado"})
			return
		}
		c.Set("company_id", claims.CompanyID)
		c.Set("user_id", claims.UserID)
		c.Set("role", claims.Role)
		c.Next()
	}
}
