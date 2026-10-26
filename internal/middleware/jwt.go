package middleware

import (
	"net/http"
	"strings"

	"github.com/flowpay/flowpay-backend/internal/authjwt"
	"github.com/gin-gonic/gin"
)

// BearerJWT exige Authorization: Bearer cuando secret no está vacío.
// revoked consulta la copia en memoria de revoked_access_tokens.
// Un token sin jti sigue válido hasta su propio vencimiento.
func BearerJWT(secret string, revoked func(string) bool) gin.HandlerFunc {
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
		if err != nil || (revoked != nil && revoked(claims.ID)) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "token inválido o expirado"})
			return
		}
		c.Set("company_id", claims.CompanyID)
		c.Set("user_id", claims.UserID)
		c.Set("role", claims.Role)
		c.Next()
	}
}
