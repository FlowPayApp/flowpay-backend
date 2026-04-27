package authjwt

import (
	"errors"
	"fmt"

	"github.com/golang-jwt/jwt/v5"
)

// AccessClaims debe coincidir con flowpay-sso/internal/authjwt.
type AccessClaims struct {
	UserID    int64  `json:"uid"`
	CompanyID int64  `json:"company_id"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	jwt.RegisteredClaims
}

func ParseAccessToken(secret []byte, raw string) (*AccessClaims, error) {
	if raw == "" {
		return nil, errors.New("token vacío")
	}
	tok, err := jwt.ParseWithClaims(raw, &AccessClaims{}, func(t *jwt.Token) (interface{}, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("algoritmo inesperado")
		}
		return secret, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := tok.Claims.(*AccessClaims)
	if !ok || !tok.Valid {
		return nil, errors.New("token inválido")
	}
	return claims, nil
}
