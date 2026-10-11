package repository

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
)

// EnsureClientPaymentToken devuelve el enlace estable de la sucursal (cliente).
// Reutiliza uno vigente; si no hay, crea uno nuevo.
func (db *DB) EnsureClientPaymentToken(ctx context.Context, companyID, clientID int64) (string, error) {
	if companyID <= 0 || clientID <= 0 {
		return "", errors.New("sucursal inválida")
	}
	var token string
	err := db.db.QueryRowContext(ctx, `
SELECT token FROM payment_tokens
WHERE company_id = $1 AND client_id = $2 AND status::text <> 'revoked'
ORDER BY id DESC
LIMIT 1
`, companyID, clientID).Scan(&token)
	if err == nil && token != "" {
		return token, nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token = base64.RawURLEncoding.EncodeToString(raw)
	err = db.db.QueryRowContext(ctx, `
INSERT INTO payment_tokens (company_id, client_id, token, status)
VALUES ($1, $2, $3, 'issued')
RETURNING token
`, companyID, clientID, token).Scan(&token)
	if err != nil {
		return "", err
	}
	return token, nil
}
